package gateway_test

import (
	"net/http"
	"net/http/httptest"
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
