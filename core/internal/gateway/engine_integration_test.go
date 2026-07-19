package gateway_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/crypto"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	cipher, err := crypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "db.sqlite"), cipher)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// mockUpstreamSSE returns a handler emitting a minimal Codex Responses SSE stream.
func mockUpstreamSSE(t *testing.T, capture *http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			*capture = r.Header.Clone()
		}
		// Verify instructions were injected + store=false.
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if instr, _ := req["instructions"].(string); strings.TrimSpace(instr) == "" {
			t.Errorf("expected injected instructions, got empty")
		}
		if storeVal, ok := req["store"].(bool); !ok || storeVal {
			t.Errorf("expected store=false, got %v", req["store"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		lines := []string{
			`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4"}}`,
			`data: {"type":"response.output_text.delta","delta":"Hello"}`,
			`data: {"type":"response.output_text.delta","delta":", world"}`,
			`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":2}}}}`,
		}
		for _, l := range lines {
			_, _ = io.WriteString(w, l+"\n\n")
			if fl != nil {
				fl.Flush()
			}
		}
	}
}

func seedAccount(t *testing.T, st *store.Store) {
	t.Helper()
	_, err := st.CreateAccount(&store.Account{
		Email:            "test@example.com",
		ChatGPTAccountID: "acc-123",
		AccessToken:      "test-access-token",
		RefreshToken:     "",
		ExpiresAt:        time.Now().Add(time.Hour),
		Status:           store.AccountActive,
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
}

func TestChatCompletionsStreaming(t *testing.T) {
	var captured http.Header
	upstream := httptest.NewServer(mockUpstreamSSE(t, &captured))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false // upstream is plain HTTP
	_ = st.SaveSettings(cfg)

	mgr := account.NewManager(st)
	engine := gateway.New(st, mgr, func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	reqBody := `{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	out := w.Body.String()
	if !strings.Contains(out, "Hello") || !strings.Contains(out, "world") {
		t.Errorf("missing content in stream: %s", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Errorf("missing [DONE] terminator")
	}
	// Anti-ban headers.
	if ua := captured.Get("User-Agent"); !strings.Contains(ua, "codex_cli_rs") {
		t.Errorf("expected codex UA, got %q", ua)
	}
	if captured.Get("originator") != "codex_cli_rs" {
		t.Errorf("expected originator codex_cli_rs, got %q", captured.Get("originator"))
	}
	if captured.Get("chatgpt-account-id") != "acc-123" {
		t.Errorf("expected chatgpt-account-id header")
	}
	if captured.Get("Authorization") != "Bearer test-access-token" {
		t.Errorf("expected bearer token header, got %q", captured.Get("Authorization"))
	}
}

func TestChatCompletionsNonStreaming(t *testing.T) {
	upstream := httptest.NewServer(mockUpstreamSSE(t, nil))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)

	mgr := account.NewManager(st)
	engine := gateway.New(st, mgr, func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	reqBody := `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "Hello, world" {
		t.Errorf("unexpected content: %+v", resp.Choices)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected total tokens 15, got %d", resp.Usage.TotalTokens)
	}
}

func TestChatCompletionsOmittedModelReturnsResolvedModel(t *testing.T) {
	tests := []struct {
		name   string
		stream bool
	}{
		{name: "streaming", stream: true},
		{name: "non-streaming", stream: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(mockUpstreamSSE(t, nil))
			defer upstream.Close()
			t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

			st := newTestStore(t)
			seedAccount(t, st)
			cfg, _ := st.LoadSettings()
			cfg.DefaultModel = "gpt-5"
			cfg.TLSFingerprint = false
			_ = st.SaveSettings(cfg)
			engine := gateway.New(st, account.NewManager(st), func() store.Settings { settings, _ := st.LoadSettings(); return settings }, nil)
			body := `{"messages":[{"role":"user","content":"hi"}]}`
			if tt.stream {
				body = `{"stream":true,"messages":[{"role":"user","content":"hi"}]}`
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			response := httptest.NewRecorder()

			engine.ChatCompletions(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), `"model":"gpt-5.4"`) || strings.Contains(response.Body.String(), `"model":""`) {
				t.Fatalf("response did not contain resolved model: %s", response.Body.String())
			}
		})
	}
}

func TestNoAccountReturns503(t *testing.T) {
	st := newTestStore(t)
	mgr := account.NewManager(st)
	engine := gateway.New(st, mgr, func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestAPIKeyAccountUsesBaseURLWithoutOAuthRefresh(t *testing.T) {
	tests := []struct {
		name string
		call func(*gateway.Engine, *store.Store) (int, string)
	}{
		{
			name: "chat completions",
			call: func(engine *gateway.Engine, _ *store.Store) (int, string) {
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
				w := httptest.NewRecorder()
				engine.ChatCompletions(w, r)
				return w.Code, w.Body.String()
			},
		},
		{
			name: "responses",
			call: func(engine *gateway.Engine, _ *store.Store) (int, string) {
				r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","stream":true,"input":"hi"}`))
				w := httptest.NewRecorder()
				engine.Responses(w, r)
				return w.Code, w.Body.String()
			},
		},
		{
			name: "connectivity test",
			call: func(engine *gateway.Engine, st *store.Store) (int, string) {
				accounts, err := st.ListAccounts()
				if err != nil || len(accounts) != 1 {
					return http.StatusInternalServerError, "failed to load API-key test account"
				}
				result := engine.TestAccount(t.Context(), accounts[0], "gpt-5.4", "hi")
				if !result.OK {
					return result.Status, result.Error
				}
				return result.Status, result.Sample
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured http.Header
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				captured = r.Header.Clone()
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"id":"resp_api","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`+"\n\n")
			}))
			defer upstream.Close()
			t.Setenv("SUB2API_UPSTREAM_URL", "http://127.0.0.1:1/should-not-be-used")

			st := newTestStore(t)
			_, err := st.CreateAccount(&store.Account{
				AccountType:      store.AccountTypeAPIKey,
				BaseURL:          upstream.URL,
				APIKey:           "sk-api-key",
				ChatGPTAccountID: "must-not-be-forwarded",
				RefreshToken:     "refresh_must_not_be_used_1234567890",
				ExpiresAt:        time.Now().Add(-time.Hour),
				Status:           store.AccountActive,
			})
			if err != nil {
				t.Fatal(err)
			}
			engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)
			status, body := tt.call(engine, st)
			if status != http.StatusOK {
				t.Fatalf("status = %d body=%s", status, body)
			}
			if calls != 1 {
				t.Fatalf("upstream calls = %d, want 1", calls)
			}
			if captured.Get("Authorization") != "Bearer sk-api-key" {
				t.Fatal("API-key authorization header was not forwarded")
			}
			if captured.Get("chatgpt-account-id") != "" {
				t.Fatalf("unexpected chatgpt-account-id = %q", captured.Get("chatgpt-account-id"))
			}
		})
	}
}

func TestManagedCloudShareTestUsesCurrentKeyAndDisablesRevokedAccess(t *testing.T) {
	revoked := false
	var authorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		if revoked {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"code":"share_access_revoked","message":"The shared access was revoked."}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"id":"resp_share","status":"completed","model":"gpt-5.6","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`+"\n\n")
	}))
	defer upstream.Close()

	st := newTestStore(t)
	if err := st.SaveCloudSession(store.CloudSession{
		UserID: 15, Email: "recipient@example.test", Role: "user", SaltKDF: "kdf", SaltAuth: "auth",
		WrappedVaultKey: "wrapped", VaultKey: "vault-key", RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedKey(store.CloudReceivedKey{
		UserID: 15, GrantPublicID: "sgr_engine", KeyVersion: 1, KeyPrefix: "sk-amber-old",
		BaseURL: upstream.URL + "/v1", GuestKey: "sk-amber-old-secret",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedAccountLink(store.CloudReceivedAccountLink{
		UserID: 15, GrantPublicID: "sgr_engine", OwnerName: "Owner", GroupName: "Shared",
		RemoteStatus: "active", Enabled: true, RPMLimit: 20, ConcurrencyLimit: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedKey(store.CloudReceivedKey{
		UserID: 15, GrantPublicID: "sgr_engine", KeyVersion: 2, KeyPrefix: "sk-amber-current",
		BaseURL: upstream.URL + "/v1", GuestKey: "sk-amber-current-secret",
	}); err != nil {
		t.Fatal(err)
	}
	accounts, err := st.ListActiveCloudReceivedAccounts()
	if err != nil || len(accounts) != 1 {
		t.Fatalf("managed accounts = %#v, err = %v", accounts, err)
	}
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { settings, _ := st.LoadSettings(); return settings }, nil)

	result := engine.TestAccount(t.Context(), accounts[0], "gpt-5.6", "hi")
	if !result.OK || result.AccountStatus != string(store.AccountActive) || authorization != "Bearer sk-amber-current-secret" {
		t.Fatalf("current managed share test = %#v, authorization = %q", result, authorization)
	}
	current, err := st.GetCloudReceivedAccount(accounts[0].ID)
	if err != nil || current.Status != store.AccountActive || current.LastSuccessAt == nil {
		t.Fatalf("successful managed share health = %#v, err = %v", current, err)
	}

	revoked = true
	result = engine.TestAccount(t.Context(), current, "gpt-5.6", "hi")
	if result.OK || result.ErrorKind != "share_access_revoked" || result.AccountStatus != string(store.AccountDisabled) {
		t.Fatalf("revoked managed share test = %#v", result)
	}
	disabled, err := st.GetCloudReceivedAccount(current.ID)
	if err != nil || disabled.Status != store.AccountDisabled || disabled.StatusReason != "share_access_revoked" {
		t.Fatalf("revoked managed share state = %#v, err = %v", disabled, err)
	}
}

func TestChatStreamingFailureEmitsErrorWithoutDone(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"type":"response.created","response":{"id":"r"}}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"partial"}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"type":"response.failed","response":{"status":"failed","error":{"message":"boom"}}}`+"\n\n")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)
	out := w.Body.String()
	if !strings.Contains(out, `"code":"upstream_failed_event"`) {
		t.Fatalf("missing error event: %s", out)
	}
	if strings.Contains(out, "data: [DONE]") {
		t.Fatalf("failed stream must not contain DONE: %s", out)
	}
	logs, _ := st.RecentLogs(1)
	if len(logs) != 1 || logs[0].StatusCode != http.StatusBadGateway || logs[0].TerminalEvent != "response.failed" {
		t.Fatalf("unexpected log: %+v", logs)
	}
}

func TestChatIncompleteMaxTokensEndsWithLength(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"type":"response.created","response":{"id":"r"}}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"partial"}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`+"\n\n")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	engine.ChatCompletions(w, r)
	if !strings.Contains(w.Body.String(), `"finish_reason":"length"`) || !strings.Contains(w.Body.String(), "data: [DONE]") {
		t.Fatalf("unexpected incomplete response: %s", w.Body.String())
	}
}

func TestUnsupportedParametersAreIgnored(t *testing.T) {
	tests := []struct {
		path    string
		body    string
		ignored string
	}{
		{path: "/v1/chat/completions", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stop":["END"]}`, ignored: "stop"},
		{path: "/v1/chat/completions", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"n":2}`, ignored: "n"},
		{path: "/v1/responses", body: `{"model":"gpt-5.4","input":"hi","logprobs":true}`, ignored: "logprobs"},
	}
	for _, tt := range tests {
		var forwarded map[string]any
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&forwarded); err != nil {
				t.Errorf("decode upstream request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"id":"r","status":"completed","output":[]}}`+"\n\n")
		}))
		t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)
		st := newTestStore(t)
		seedAccount(t, st)
		engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)
		r := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
		w := httptest.NewRecorder()
		if tt.path == "/v1/responses" {
			engine.Responses(w, r)
		} else {
			engine.ChatCompletions(w, r)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("path=%s status=%d body=%s", tt.path, w.Code, w.Body.String())
		}
		if _, exists := forwarded[tt.ignored]; exists {
			t.Fatalf("path=%s forwarded ignored parameter %q: %#v", tt.path, tt.ignored, forwarded)
		}
		upstream.Close()
	}
}

func TestAccountProbeRejectsMissingTerminal(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"type":"response.created","response":{"id":"r"}}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"partial"}`+"\n\n")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)
	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)
	manager := account.NewManager(st)
	engine := gateway.New(st, manager, func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)
	accountValue, _ := st.GetAccount(1)
	result := engine.TestAccount(t.Context(), accountValue, "gpt-5.4", "hi")
	if result.OK || result.Status != http.StatusBadGateway || !strings.Contains(result.Error, "terminal") {
		t.Fatalf("unexpected probe result: %+v", result)
	}
}
