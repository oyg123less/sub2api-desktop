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
			`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":5}}}`,
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

func TestUnsupportedParametersReturnStableCode(t *testing.T) {
	tests := []struct {
		path string
		body string
	}{
		{path: "/v1/chat/completions", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stop":["END"]}`},
		{path: "/v1/chat/completions", body: `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"n":2}`},
		{path: "/v1/responses", body: `{"model":"gpt-5.4","input":"hi","logprobs":true}`},
	}
	for _, tt := range tests {
		st := newTestStore(t)
		engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)
		r := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
		w := httptest.NewRecorder()
		if tt.path == "/v1/responses" {
			engine.Responses(w, r)
		} else {
			engine.ChatCompletions(w, r)
		}
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"unsupported_parameter"`) {
			t.Fatalf("path=%s status=%d body=%s", tt.path, w.Code, w.Body.String())
		}
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
