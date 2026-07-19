package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sub2api-desktop/core/internal/account"
	appcrypto "sub2api-desktop/core/internal/crypto"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

func newAccountsControlTest(t *testing.T) (*Control, *store.Account) {
	t.Helper()
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "data.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	created, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeAPIKey,
		BaseURL:     "https://api.example.test/v1",
		APIKey:      "sk-fixture",
		Email:       "fixture account",
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { return store.Settings{AccountStrategy: gateway.StrategyQuotaAware} }, nil)
	return &Control{store: st, engine: engine}, created
}

func TestSetAccountLimitsValidatesAndReturnsRuntime(t *testing.T) {
	control, created := newAccountsControlTest(t)
	request := httptest.NewRequest(http.MethodPut, "/control/accounts/1/limits", strings.NewReader(`{"max_concurrency":7,"queue_capacity":42}`))
	request.SetPathValue("id", "1")
	response := httptest.NewRecorder()

	control.setAccountLimits(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Account store.Account `json:"account"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Account.ID != created.ID || result.Account.MaxConcurrency != 7 || result.Account.QueueCapacity != 42 || result.Account.InFlight != 0 || result.Account.Waiting != 0 {
		t.Fatalf("unexpected account response: %+v", result.Account)
	}

	invalid := httptest.NewRequest(http.MethodPut, "/control/accounts/1/limits", strings.NewReader(`{"max_concurrency":101,"queue_capacity":42}`))
	invalid.SetPathValue("id", "1")
	invalidResponse := httptest.NewRecorder()
	control.setAccountLimits(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest || !strings.Contains(invalidResponse.Body.String(), "invalid_account_limits") {
		t.Fatalf("invalid status=%d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}
}

func TestListAccountRuntimeRecoversExpiredRateLimit(t *testing.T) {
	control, created := newAccountsControlTest(t)
	if err := control.store.SetRateLimited(created.ID, time.Now().Add(-time.Minute), "transient_rate_limit"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/control/accounts/runtime", nil)
	response := httptest.NewRecorder()

	control.listAccountRuntime(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Accounts []store.AccountRuntimeState `json:"accounts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 1 {
		t.Fatalf("runtime states = %+v", result.Accounts)
	}
	state := result.Accounts[0]
	if state.ID != created.ID || state.Status != store.AccountActive || state.StatusReason != "" || state.RateLimitedUntil != nil || state.InFlight != 0 || state.Waiting != 0 {
		t.Fatalf("expired rate limit was not recovered: %+v", state)
	}
}

func TestListAccountsIncludesManagedCloudShare(t *testing.T) {
	control, local := newAccountsControlTest(t)
	if err := control.store.SaveCloudSession(store.CloudSession{
		UserID: 11, Email: "recipient@example.test", Role: "user", SaltKDF: "kdf", SaltAuth: "auth",
		WrappedVaultKey: "wrapped", VaultKey: "vault-key", RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	if err := control.store.SaveCloudReceivedKey(store.CloudReceivedKey{
		UserID: 11, GrantPublicID: "sgr_control", KeyVersion: 2, KeyPrefix: "sk-amber-current",
		BaseURL: "https://cloud.example.test/v1", GuestKey: "sk-amber-current-secret",
	}); err != nil {
		t.Fatal(err)
	}
	if err := control.store.SaveCloudReceivedAccountLink(store.CloudReceivedAccountLink{
		UserID: 11, GrantPublicID: "sgr_control", OwnerName: "Share owner", GroupName: "Team share",
		RemoteStatus: "active", Enabled: true, RPMLimit: 20, ConcurrencyLimit: 2,
	}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/control/accounts", nil)
	response := httptest.NewRecorder()

	control.listAccounts(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Accounts []store.Account `json:"accounts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 2 {
		t.Fatalf("accounts = %#v", result.Accounts)
	}
	var managed *store.Account
	for index := range result.Accounts {
		if result.Accounts[index].Source == "cloud_share" {
			managed = &result.Accounts[index]
		}
	}
	if managed == nil || managed.ID >= 0 || managed.CloudGrantID != "sgr_control" ||
		managed.CloudOwnerName != "Share owner" || managed.CloudGroupName != "Team share" ||
		managed.MaxConcurrency != 2 || managed.Status != store.AccountActive || local.ID <= 0 {
		t.Fatalf("managed cloud account = %#v", managed)
	}
}

func TestManagedCloudAccountRejectsLocalDeleteAndLimits(t *testing.T) {
	control, _ := newAccountsControlTest(t)
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/control/accounts/-1", nil)
	deleteRequest.SetPathValue("id", "-1")
	deleteResponse := httptest.NewRecorder()
	control.deleteAccount(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusConflict || !strings.Contains(deleteResponse.Body.String(), "cloud_share_managed") {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}

	limitsRequest := httptest.NewRequest(http.MethodPut, "/control/accounts/-1/limits", strings.NewReader(`{"max_concurrency":2,"queue_capacity":10}`))
	limitsRequest.SetPathValue("id", "-1")
	limitsResponse := httptest.NewRecorder()
	control.setAccountLimits(limitsResponse, limitsRequest)
	if limitsResponse.Code != http.StatusConflict || !strings.Contains(limitsResponse.Body.String(), "cloud_share_managed") {
		t.Fatalf("limits status=%d body=%s", limitsResponse.Code, limitsResponse.Body.String())
	}
}

func TestStatusCountsManagedCloudShares(t *testing.T) {
	control, _ := newAccountsControlTest(t)
	control.settings = &settingsPatchAccess{value: store.DefaultSettings()}
	control.server = &restartServerStub{running: true}
	if err := control.store.SaveCloudSession(store.CloudSession{
		UserID: 12, Email: "recipient@example.test", Role: "user", SaltKDF: "kdf", SaltAuth: "auth",
		WrappedVaultKey: "wrapped", VaultKey: "vault-key", RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	if err := control.store.SaveCloudReceivedKey(store.CloudReceivedKey{
		UserID: 12, GrantPublicID: "sgr_status", KeyVersion: 1, KeyPrefix: "sk-amber-status",
		BaseURL: "https://cloud.example.test/v1", GuestKey: "sk-amber-status-secret",
	}); err != nil {
		t.Fatal(err)
	}
	if err := control.store.SaveCloudReceivedAccountLink(store.CloudReceivedAccountLink{
		UserID: 12, GrantPublicID: "sgr_status", OwnerName: "Owner", GroupName: "Shared",
		RemoteStatus: "active", Enabled: true, RPMLimit: 20, ConcurrencyLimit: 2,
	}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	control.status(response, httptest.NewRequest(http.MethodGet, "/control/status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		AccountCount int `json:"account_count"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AccountCount != 2 {
		t.Fatalf("account_count=%d, want local + managed cloud share", payload.AccountCount)
	}
}
