package store

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	appcrypto "sub2api-desktop/core/internal/crypto"
)

func openCloudTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(dir, "data.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCloudMigrationTracksDirtyRowsAndTombstones(t *testing.T) {
	st := openCloudTestStore(t)
	account, err := st.CreateAccount(&Account{
		AccountType: AccountTypeOAuth, Email: "sync@example.test", AccessToken: "access", RefreshToken: "refresh", Status: AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuid.MatchString(account.ClientUID) || !account.SyncDirty || account.SyncVersion != 0 {
		t.Fatalf("unexpected sync metadata: uid=%q dirty=%v version=%d", account.ClientUID, account.SyncDirty, account.SyncVersion)
	}
	if err := st.MarkCloudItemSynced(CloudKindAccount, account.ClientUID, 3); err != nil {
		t.Fatal(err)
	}
	if err := st.SetAccountLimits(account.ID, 8, 64); err != nil {
		t.Fatal(err)
	}
	limitsUpdated, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !limitsUpdated.SyncDirty || limitsUpdated.MaxConcurrency != 8 || limitsUpdated.QueueCapacity != 64 {
		t.Fatalf("limit update did not mark account dirty: %#v", limitsUpdated)
	}
	if err := st.MarkCloudItemSynced(CloudKindAccount, account.ClientUID, 3); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateTokens(account.ID, "new-access", "new-refresh", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.SyncDirty || updated.SyncVersion != 3 {
		t.Fatalf("credential update did not mark dirty: %#v", updated)
	}
	if err := st.DeleteAccount(account.ID); err != nil {
		t.Fatal(err)
	}
	tombstones, err := st.CloudTombstones()
	if err != nil || len(tombstones) != 1 {
		t.Fatalf("tombstones=%#v err=%v", tombstones, err)
	}
	if tombstones[0].ClientUID != account.ClientUID || tombstones[0].SyncVersion != 3 {
		t.Fatalf("unexpected tombstone: %#v", tombstones[0])
	}
}

func TestCloudSessionSecretsUseInstallationCipher(t *testing.T) {
	st := openCloudTestStore(t)
	session := CloudSession{
		UserID: 7, Email: "cloud@example.test", Role: "user", SaltKDF: "salt-kdf", SaltAuth: "salt-auth",
		WrappedVaultKey: "v1.wrapped", VaultKey: "vault-key-plaintext", RefreshToken: "refresh-token-plaintext",
	}
	if err := st.SaveCloudSession(session); err != nil {
		t.Fatal(err)
	}
	var vaultCipher, refreshCipher string
	if err := st.db.QueryRow(`SELECT vault_key_cipher,refresh_token_cipher FROM cloud_session WHERE id=1`).Scan(&vaultCipher, &refreshCipher); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(vaultCipher, session.VaultKey) || strings.Contains(refreshCipher, session.RefreshToken) {
		t.Fatal("cloud session stored plaintext secrets")
	}
	loaded, err := st.LoadCloudSession()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VaultKey != session.VaultKey || loaded.RefreshToken != session.RefreshToken {
		t.Fatalf("loaded session differs: %#v", loaded)
	}
}

func TestSettingsBecomeDirtyOutsideRemoteApply(t *testing.T) {
	st := openCloudTestStore(t)
	state, err := st.CloudSettingsState()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkCloudItemSynced(CloudKindSettings, state.ClientUID, 2); err != nil {
		t.Fatal(err)
	}
	settings, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Language = "en-US"
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	state, err = st.CloudSettingsState()
	if err != nil || !state.SyncDirty || state.SyncVersion != 2 {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestCloudConflictIncludesLocalDisplayNameWithoutPersistingIt(t *testing.T) {
	st := openCloudTestStore(t)
	proxy, err := st.CreateProxy(&Proxy{Name: "Office gateway", Type: ProxyHTTP, Host: "127.0.0.1", Port: 8080})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddCloudConflict(CloudKindProxy, proxy.ClientUID, "local_won", "Local update was newer than the remote update."); err != nil {
		t.Fatal(err)
	}
	conflicts, err := st.ListCloudConflicts(10)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts=%#v err=%v", conflicts, err)
	}
	if conflicts[0].DisplayName != proxy.Name {
		t.Fatalf("display name = %q, want %q", conflicts[0].DisplayName, proxy.Name)
	}
	var storedDetails string
	if err := st.db.QueryRow("SELECT details FROM cloud_sync_conflicts WHERE id=?", conflicts[0].ID).Scan(&storedDetails); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedDetails, proxy.Name) {
		t.Fatal("display name was persisted in conflict details")
	}
}

func TestCloudPendingRegistrationUsesInstallationCipher(t *testing.T) {
	st := openCloudTestStore(t)
	payload := []byte(`{"email":"pending@example.test","auth_hash":"private-auth","vault_key":"private-vault"}`)
	if err := st.SaveCloudPendingRegistration(payload); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := st.db.QueryRow(`SELECT payload_cipher FROM cloud_pending_registration WHERE id=1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "pending@example.test") || strings.Contains(stored, "private-auth") || strings.Contains(stored, "private-vault") {
		t.Fatal("pending registration material was stored in plaintext")
	}
	loaded, err := st.LoadCloudPendingRegistration()
	if err != nil || string(loaded) != string(payload) {
		t.Fatalf("loaded=%q err=%v", loaded, err)
	}
	if err := st.DeleteCloudPendingRegistration(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LoadCloudPendingRegistration(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load after delete error = %v", err)
	}
}
