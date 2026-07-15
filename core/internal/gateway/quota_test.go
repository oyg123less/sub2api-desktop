package gateway_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

func TestRefreshUsageSnapshotsPersistsQuotaThroughAccountProxy(t *testing.T) {
	var usageCalls atomic.Int32
	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		usageCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer quota-access" {
			t.Error("quota request did not use the account access token")
		}
		if r.Header.Get("chatgpt-account-id") != "acct_quota" {
			t.Error("quota request did not include chatgpt-account-id")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"rate_limit":{"primary_window":{"used_percent":12.5,"limit_window_seconds":18000,"reset_after_seconds":1200},"secondary_window":{"used_percent":34.5,"limit_window_seconds":604800,"reset_after_seconds":3600}}}`)
	}))
	defer usageServer.Close()
	t.Setenv("SUB2API_USAGE_URL", usageServer.URL)

	var proxyCalls atomic.Int32
	directTransport := &http.Transport{}
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		outgoing := r.Clone(r.Context())
		outgoing.RequestURI = ""
		response, err := directTransport.RoundTrip(outgoing)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	defer proxyServer.Close()
	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyPort, err := strconv.Atoi(proxyURL.Port())
	if err != nil {
		t.Fatal(err)
	}

	st := newTestStore(t)
	proxy, err := st.CreateProxy(&store.Proxy{Name: "quota proxy", Type: store.ProxyHTTP, Host: proxyURL.Hostname(), Port: proxyPort})
	if err != nil {
		t.Fatal(err)
	}
	oauthAccount, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeOAuth, Email: "quota@example.com", ChatGPTAccountID: "acct_quota",
		AccessToken: "quota-access", ExpiresAt: time.Now().Add(time.Hour), Status: store.AccountActive, ProxyID: &proxy.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeAPIKey, BaseURL: "https://api.example.com/v1/responses", APIKey: "sk-skip", Status: store.AccountActive,
	}); err != nil {
		t.Fatal(err)
	}
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { settings, _ := st.LoadSettings(); return settings }, nil)

	engine.RefreshUsageSnapshots(t.Context())

	updated, err := st.GetAccount(oauthAccount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CodexUsage == nil || updated.CodexUsage.PrimaryUsedPercent == nil || *updated.CodexUsage.PrimaryUsedPercent != 12.5 {
		t.Fatal("primary quota window was not persisted")
	}
	if updated.CodexUsage.PrimaryWindowMinutes == nil || *updated.CodexUsage.PrimaryWindowMinutes != 300 {
		t.Fatal("primary quota window duration was not converted to minutes")
	}
	if updated.CodexUsage.SecondaryUsedPercent == nil || *updated.CodexUsage.SecondaryUsedPercent != 34.5 {
		t.Fatal("secondary quota window was not persisted")
	}
	if updated.CodexUsage.SecondaryWindowMinutes == nil || *updated.CodexUsage.SecondaryWindowMinutes != 10080 {
		t.Fatal("secondary quota window duration was not converted to minutes")
	}
	if proxyCalls.Load() != 1 || usageCalls.Load() != 1 {
		t.Fatalf("proxy calls = %d, usage calls = %d; want 1 each", proxyCalls.Load(), usageCalls.Load())
	}
}

func TestRefreshUsageSnapshotsFailurePreservesAccountStatus(t *testing.T) {
	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusInternalServerError)
	}))
	defer usageServer.Close()
	t.Setenv("SUB2API_USAGE_URL", usageServer.URL)

	st := newTestStore(t)
	created, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeOAuth, ChatGPTAccountID: "acct_failure", AccessToken: "quota-access",
		ExpiresAt: time.Now().Add(time.Hour), Status: store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { settings, _ := st.LoadSettings(); return settings }, nil)

	engine.RefreshUsageSnapshots(t.Context())

	updated, err := st.GetAccount(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != store.AccountActive || updated.CodexUsage != nil {
		t.Fatalf("failed quota query changed account state: status=%s usage_present=%t", updated.Status, updated.CodexUsage != nil)
	}
}
