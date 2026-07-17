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
