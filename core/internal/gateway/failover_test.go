package gateway_test

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

// TestFailoverOn429 verifies that a 429 from the first account triggers a switch
// to the next account, which succeeds.
func TestFailoverOn429(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		for _, l := range []string{
			`data: {"type":"response.created","response":{"id":"r","model":"gpt-5.4"}}`,
			`data: {"type":"response.output_text.delta","delta":"ok"}`,
			`data: {"type":"response.completed","response":{"id":"r","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`,
		} {
			_, _ = w.Write([]byte(l + "\n\n"))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	for i := 0; i < 2; i++ {
		if _, err := st.CreateAccount(&store.Account{
			Email:            "acc",
			ChatGPTAccountID: "cid-" + string(rune('a'+i)),
			AccessToken:      "tok",
			ExpiresAt:        time.Now().Add(time.Hour),
			Status:           store.AccountActive,
		}); err != nil {
			t.Fatal(err)
		}
	}
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)

	mgr := account.NewManager(st)
	engine := gateway.New(st, mgr, func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after failover, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("expected content from second account, got %s", w.Body.String())
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("expected 2 upstream calls (429 then success), got %d", calls)
	}
	// First account should be marked rate-limited.
	accs, _ := st.ListAccounts()
	var rateLimited int
	for _, a := range accs {
		if a.Status == store.AccountRateLimited {
			rateLimited++
		}
	}
	if rateLimited != 1 {
		t.Errorf("expected 1 rate-limited account, got %d", rateLimited)
	}
}

func TestFailoverOnUpstreamServerError(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		call func(*gateway.Engine, http.ResponseWriter, *http.Request)
	}{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`,
			call: (*gateway.Engine).ChatCompletions,
		},
		{
			name: "responses",
			path: "/v1/responses",
			body: `{"model":"gpt-5.4","stream":true,"input":"hi"}`,
			call: (*gateway.Engine).Responses,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if atomic.AddInt32(&calls, 1) == 1 {
					http.Error(w, "temporary upstream failure", http.StatusBadGateway)
					return
				}
				writeSuccessfulSSE(w, "recovered")
			}))
			defer upstream.Close()
			t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

			st := newTestStore(t)
			seedFailoverAccounts(t, st)
			engine := newFailoverEngine(t, st)

			r := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			tt.call(engine, w, r)

			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "recovered") {
				t.Fatalf("expected successful failover, status=%d body=%s", w.Code, w.Body.String())
			}
			if got := atomic.LoadInt32(&calls); got != 2 {
				t.Fatalf("upstream calls = %d, want 2", got)
			}
		})
	}
}

func TestFailoverOnProxyConnectionRefusedPreservesAccountStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSuccessfulSSE(w, "via second account")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	st := newTestStore(t)
	accounts := seedFailoverAccounts(t, st)
	proxy, err := st.CreateProxy(&store.Proxy{Name: "refused", Type: store.ProxyHTTP, Host: "127.0.0.1", Port: port})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetAccountProxy(accounts[0].ID, &proxy.ID); err != nil {
		t.Fatal(err)
	}
	engine := newFailoverEngine(t, st)

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "via second account") {
		t.Fatalf("expected successful proxy failover, status=%d body=%s", w.Code, w.Body.String())
	}
	first, err := st.GetAccount(accounts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != store.AccountActive {
		t.Fatalf("first account status = %q, want active", first.Status)
	}
	logs, err := st.RecentLogs(10)
	if err != nil {
		t.Fatal(err)
	}
	var foundNetworkError bool
	for _, entry := range logs {
		if entry.AccountID != nil && *entry.AccountID == first.ID && entry.ErrorKind == "upstream_network_error" {
			foundNetworkError = true
			break
		}
	}
	if !foundNetworkError {
		t.Fatalf("missing upstream network error log: %+v", logs)
	}
}

func TestMissingBoundProxyFailsClosedAndUsesNextAccount(t *testing.T) {
	var calls int32
	var accountID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		accountID = r.Header.Get("chatgpt-account-id")
		writeSuccessfulSSE(w, "via unbound account")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	accounts := seedFailoverAccounts(t, st)
	missingProxyID := int64(999999)
	if err := st.SetAccountProxy(accounts[0].ID, &missingProxyID); err != nil {
		t.Fatal(err)
	}
	engine := newFailoverEngine(t, st)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	response := httptest.NewRecorder()

	engine.ChatCompletions(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "via unbound account") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want only the unbound account call", got)
	}
	if accountID != accounts[1].ChatGPTAccountID {
		t.Fatalf("upstream account = %q, want %q", accountID, accounts[1].ChatGPTAccountID)
	}
	logs, err := st.RecentLogs(10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range logs {
		if entry.AccountID != nil && *entry.AccountID == accounts[0].ID && entry.ErrorKind == "proxy_unavailable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing proxy_unavailable log: %+v", logs)
	}
}

func TestNativeResponsesMissingBoundProxyFailsClosed(t *testing.T) {
	var calls int32
	var accountID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		accountID = r.Header.Get("chatgpt-account-id")
		writeSuccessfulSSE(w, "native response")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	accounts := seedFailoverAccounts(t, st)
	missingProxyID := int64(999999)
	if err := st.SetAccountProxy(accounts[0].ID, &missingProxyID); err != nil {
		t.Fatal(err)
	}
	engine := newFailoverEngine(t, st)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","stream":true,"input":"hi"}`))
	response := httptest.NewRecorder()

	engine.Responses(response, request)

	got := atomic.LoadInt32(&calls)
	if response.Code != http.StatusOK || got != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, got, response.Body.String())
	}
	if accountID != accounts[1].ChatGPTAccountID {
		t.Fatalf("upstream account = %q, want %q", accountID, accounts[1].ChatGPTAccountID)
	}
}

func TestAccountCheckMissingBoundProxyDoesNotConnectDirectly(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeSuccessfulSSE(w, "unexpected direct request")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	accounts := seedFailoverAccounts(t, st)
	missingProxyID := int64(999999)
	if err := st.SetAccountProxy(accounts[0].ID, &missingProxyID); err != nil {
		t.Fatal(err)
	}
	boundAccount, err := st.GetAccount(accounts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	engine := newFailoverEngine(t, st)
	result := engine.TestAccount(t.Context(), boundAccount, "gpt-5.4", "hi")

	if result.OK || result.Status != http.StatusBadGateway || result.Error != "bound proxy is unavailable" {
		t.Fatalf("result = %+v", result)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("upstream calls = %d, want 0", got)
	}
}

func TestClientErrorDoesNotFailOver(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedFailoverAccounts(t, st)
	engine := newFailoverEngine(t, st)
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestFailoverOnEarlyStreamingFailure(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, `data: {"type":"response.failed","response":{"status":"failed","error":{"message":"first account failed"}}}`+"\n\n")
			return
		}
		writeSuccessfulSSE(w, "second account succeeded")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedFailoverAccounts(t, st)
	engine := newFailoverEngine(t, st)
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "second account succeeded") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected complete stream from second account: %s", body)
	}
	if strings.Contains(body, "first account failed") || strings.Contains(body, `"code":"upstream_failed_event"`) {
		t.Fatalf("first account failure leaked into client stream: %s", body)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
}

func seedFailoverAccounts(t *testing.T, st *store.Store) []*store.Account {
	t.Helper()
	accounts := make([]*store.Account, 0, 2)
	for i := 0; i < 2; i++ {
		created, err := st.CreateAccount(&store.Account{
			Email:            "account-" + strconv.Itoa(i) + "@example.com",
			ChatGPTAccountID: "cid-" + strconv.Itoa(i),
			AccessToken:      "token-" + strconv.Itoa(i),
			ExpiresAt:        time.Now().Add(time.Hour),
			Status:           store.AccountActive,
		})
		if err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, created)
	}
	return accounts
}

func newFailoverEngine(t *testing.T, st *store.Store) *gateway.Engine {
	t.Helper()
	cfg, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TLSFingerprint = false
	cfg.CompatProfile = "standard"
	cfg.AccountStrategy = gateway.StrategyFailover
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	return gateway.New(st, account.NewManager(st), func() store.Settings {
		settings, _ := st.LoadSettings()
		return settings
	}, nil)
}

func writeSuccessfulSSE(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, `data: {"type":"response.created","response":{"id":"r","model":"gpt-5.4"}}`+"\n\n")
	_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","delta":`+strconv.Quote(content)+`}`+"\n\n")
	_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"id":"r","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`+"\n\n")
}
