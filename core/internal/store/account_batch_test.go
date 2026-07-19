package store

import (
	"errors"
	"testing"
)

func createBatchTestAccount(t *testing.T, st *Store, name, key string) *Account {
	t.Helper()
	account, err := st.CreateAccount(&Account{
		AccountType: AccountTypeAPIKey,
		BaseURL:     "https://api.example.test/" + name,
		APIKey:      key,
		Email:       name + "@example.test",
		Status:      AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func TestDeleteAccountsDeduplicatesAndCreatesTombstones(t *testing.T) {
	st := openCloudTestStore(t)
	first := createBatchTestAccount(t, st, "first", "sk-first")
	second := createBatchTestAccount(t, st, "second", "sk-second")
	result, err := st.DeleteAccounts([]int64{second.ID, first.ID, second.ID, 99999})
	if err != nil {
		t.Fatal(err)
	}
	if result.Requested != 3 || len(result.Deleted) != 2 || len(result.Missing) != 1 || len(result.Failed) != 0 {
		t.Fatalf("unexpected batch result: %#v", result)
	}
	if result.Deleted == nil || result.Missing == nil || result.Failed == nil {
		t.Fatalf("batch result collections must encode as arrays: %#v", result)
	}
	if _, err := st.GetAccount(first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("first account still exists: %v", err)
	}
	tombstones, err := st.CloudTombstones()
	if err != nil {
		t.Fatal(err)
	}
	if len(tombstones) != 2 {
		t.Fatalf("tombstones=%d, want 2", len(tombstones))
	}
}

func TestSetAllAccountsProxyAndSummary(t *testing.T) {
	st := openCloudTestStore(t)
	emptySummary, err := st.AccountProxySummary()
	if err != nil || emptySummary.Bindings == nil {
		t.Fatalf("empty summary must encode bindings as an array: %#v err=%v", emptySummary, err)
	}
	first := createBatchTestAccount(t, st, "first", "sk-first")
	second := createBatchTestAccount(t, st, "second", "sk-second")
	proxy, err := st.CreateProxy(&Proxy{Name: "relay", Type: ProxySOCKS5, Host: "127.0.0.1", Port: 1080})
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []*Account{first, second} {
		if err := st.MarkCloudItemSynced(CloudKindAccount, account.ClientUID, 1); err != nil {
			t.Fatalf("mark account %d synced: %v", account.ID, err)
		}
	}
	result, err := st.SetAllAccountsProxy(&proxy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 2 || result.Updated != 2 || result.Unchanged != 0 {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	repeat, err := st.SetAllAccountsProxy(&proxy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Updated != 0 || repeat.Unchanged != 2 {
		t.Fatalf("unexpected repeat result: %#v", repeat)
	}
	summary, err := st.AccountProxySummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 2 || summary.Bound != 2 || summary.Mixed || summary.UniformProxyID == nil || *summary.UniformProxyID != proxy.ID {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	for _, id := range []int64{first.ID, second.ID} {
		account, err := st.GetAccount(id)
		if err != nil || account.ProxyID == nil || *account.ProxyID != proxy.ID || !account.SyncDirty {
			t.Fatalf("account %d was not bound and marked dirty: %#v err=%v", id, account, err)
		}
	}
	cleared, err := st.SetAllAccountsProxy(nil)
	if err != nil || cleared.Updated != 2 {
		t.Fatalf("clear result=%#v err=%v", cleared, err)
	}
	summary, err = st.AccountProxySummary()
	if err != nil || summary.Unbound != 2 || summary.Bound != 0 || summary.Mixed || summary.Bindings == nil {
		t.Fatalf("unexpected cleared summary: %#v err=%v", summary, err)
	}
}

func TestSetAllAccountsProxyIncludesManagedCloudShares(t *testing.T) {
	st := openCloudTestStore(t)
	createBatchTestAccount(t, st, "local", "sk-local")
	proxy, err := st.CreateProxy(&Proxy{Name: "relay", Type: ProxySOCKS5, Host: "127.0.0.1", Port: 1080})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudSession(CloudSession{
		UserID: 9, Email: "recipient@example.test", Role: "user", SaltKDF: "kdf", SaltAuth: "auth",
		WrappedVaultKey: "wrapped", VaultKey: "vault-key", RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedKey(CloudReceivedKey{
		UserID: 9, GrantPublicID: "sgr_proxy", KeyVersion: 1, KeyPrefix: "sk-amber-proxy",
		BaseURL: "https://cloud.example.test/v1", GuestKey: "sk-amber-proxy-secret",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedAccountLink(CloudReceivedAccountLink{
		UserID: 9, GrantPublicID: "sgr_proxy", OwnerName: "Owner", GroupName: "Shared",
		RemoteStatus: "active", Enabled: true, RPMLimit: 30, ConcurrencyLimit: 2,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := st.SetAllAccountsProxy(&proxy.ID)
	if err != nil || result.Matched != 2 || result.Updated != 2 || result.Unchanged != 0 {
		t.Fatalf("global proxy result = %#v, err = %v", result, err)
	}
	managed, err := st.GetCloudReceivedAccountByGrant(9, "sgr_proxy")
	if err != nil || managed.ProxyID == nil || *managed.ProxyID != proxy.ID {
		t.Fatalf("managed cloud proxy was not updated: %#v, err = %v", managed, err)
	}
	summary, err := st.AccountProxySummary()
	if err != nil || summary.Total != 2 || summary.Bound != 2 || summary.Mixed ||
		summary.UniformProxyID == nil || *summary.UniformProxyID != proxy.ID {
		t.Fatalf("proxy summary omitted managed cloud account: %#v, err = %v", summary, err)
	}
}

func TestCloudConnectionSettingsAreDeviceLocalAndValidated(t *testing.T) {
	st := openCloudTestStore(t)
	initial, err := st.CloudConnectionSettings()
	if err != nil || initial.Mode != CloudConnectionSystem || initial.ProxyID != nil {
		t.Fatalf("initial=%#v err=%v", initial, err)
	}
	proxy, err := st.CreateProxy(&Proxy{Name: "cloud", Type: ProxyHTTP, Host: "127.0.0.1", Port: 8080})
	if err != nil {
		t.Fatal(err)
	}
	settingsBefore, err := st.CloudSettingsState()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudConnectionSettings(CloudConnectionSettings{Mode: CloudConnectionProxy, ProxyID: &proxy.ID}); err != nil {
		t.Fatal(err)
	}
	stored, err := st.CloudConnectionSettings()
	if err != nil || stored.ProxyID == nil || *stored.ProxyID != proxy.ID {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	if err := st.SaveCloudConnectionSettings(CloudConnectionSettings{Mode: CloudConnectionDirect, ProxyID: &proxy.ID}); err == nil {
		t.Fatal("direct mode accepted proxy_id")
	}
	settingsState, err := st.CloudSettingsState()
	if err != nil {
		t.Fatal(err)
	}
	if settingsState.SyncDirty != settingsBefore.SyncDirty || settingsState.SyncVersion != settingsBefore.SyncVersion || settingsState.UpdatedAt != settingsBefore.UpdatedAt {
		t.Fatal("device-local cloud connection settings changed cloud settings sync metadata")
	}
}
