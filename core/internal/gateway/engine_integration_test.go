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
