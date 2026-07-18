package gateway_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

func TestRelayAccountForcesCloudAuthorizedAccountAndMarksUpstreamStart(t *testing.T) {
	var captured http.Header
	upstream := httptest.NewServer(mockUpstreamSSE(t, &captured))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	for _, candidate := range []*store.Account{
		{Email: "other@example.test", ChatGPTAccountID: "acct-other", AccessToken: "token-other", ExpiresAt: time.Now().Add(time.Hour), Status: store.AccountActive},
		{Email: "shared@example.test", ChatGPTAccountID: "acct-shared", AccessToken: "token-shared", ExpiresAt: time.Now().Add(time.Hour), Status: store.AccountActive},
	} {
		if _, err := st.CreateAccount(candidate); err != nil {
			t.Fatal(err)
		}
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	var targetUID string
	for _, candidate := range accounts {
		if candidate.ChatGPTAccountID == "acct-shared" {
			targetUID = candidate.ClientUID
		}
	}
	if targetUID == "" {
		t.Fatal("target account UID was not created")
	}
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	engine := gateway.New(st, account.NewManager(st), func() store.Settings {
		settings, _ := st.LoadSettings()
		return settings
	}, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","stream":true,"input":"hi"}`))
	response := httptest.NewRecorder()
	started := false
	engine.RelayAccount(response, request, targetUID, func() { started = true })

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if captured.Get("chatgpt-account-id") != "acct-shared" || captured.Get("Authorization") != "Bearer token-shared" {
		t.Fatalf("relay used the wrong account: account=%q auth=%q", captured.Get("chatgpt-account-id"), captured.Get("Authorization"))
	}
	if !started {
		t.Fatal("upstream_started callback was not fired before the relay completed")
	}
}

func TestRelayAccountRejectsUnknownAccountWithoutStartingUpstream(t *testing.T) {
	st := newTestStore(t)
	seedAccount(t, st)
	engine := gateway.New(st, account.NewManager(st), func() store.Settings {
		settings, _ := st.LoadSettings()
		return settings
	}, nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","input":"hi"}`))
	response := httptest.NewRecorder()
	started := false
	engine.RelayAccount(response, request, "018f1f46-7a19-7cc2-88cb-f577e51d3999", func() { started = true })
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if started {
		t.Fatal("unknown relay account reached the upstream-start boundary")
	}
}
