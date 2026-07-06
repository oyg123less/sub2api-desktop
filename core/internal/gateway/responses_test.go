package gateway_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

func TestResponsesPassthrough(t *testing.T) {
	var captured http.Header
	upstream := httptest.NewServer(mockUpstreamSSE(t, &captured))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)

	mgr := account.NewManager(st)
	engine := gateway.New(st, mgr, func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	reqBody := `{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	engine.Responses(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	out := w.Body.String()
	if !strings.Contains(out, "response.created") || !strings.Contains(out, "response.completed") {
		t.Errorf("expected verbatim responses SSE, got: %s", out)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("missing content in stream: %s", out)
	}
	if captured.Get("originator") != "codex_cli_rs" {
		t.Errorf("expected originator codex_cli_rs, got %q", captured.Get("originator"))
	}
	if captured.Get("chatgpt-account-id") != "acc-123" {
		t.Errorf("expected chatgpt-account-id header")
	}
}
