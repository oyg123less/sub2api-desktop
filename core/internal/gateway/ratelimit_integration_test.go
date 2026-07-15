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

func TestRateLimitedUntilUsesCodexResetHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		want    time.Duration
	}{
		{
			name: "earliest primary and secondary reset",
			headers: http.Header{
				"X-Codex-Primary-Reset-After-Seconds":   []string{"120"},
				"X-Codex-Secondary-Reset-After-Seconds": []string{"900"},
			},
			want: 120 * time.Second,
		},
		{name: "default without reset headers", headers: make(http.Header), want: 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for key, values := range tt.headers {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				http.Error(w, "rate limited", http.StatusTooManyRequests)
			}))
			defer upstream.Close()
			t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

			st := newTestStore(t)
			created, err := st.CreateAccount(&store.Account{
				AccountType: store.AccountTypeOAuth, ChatGPTAccountID: "acct_reset", AccessToken: "access",
				ExpiresAt: time.Now().Add(time.Hour), Status: store.AccountActive,
			})
			if err != nil {
				t.Fatal(err)
			}
			engine := gateway.New(st, account.NewManager(st), func() store.Settings { settings, _ := st.LoadSettings(); return settings }, nil)
			before := time.Now()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}]}`))
			response := httptest.NewRecorder()

			engine.ChatCompletions(response, request)

			updated, err := st.GetAccount(created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Status != store.AccountRateLimited || updated.RateLimitedUntil == nil {
				t.Fatalf("account was not rate limited: status=%s", updated.Status)
			}
			got := updated.RateLimitedUntil.Sub(before)
			if got < tt.want-2*time.Second || got > tt.want+2*time.Second {
				t.Fatalf("rate_limited_until delta = %s, want %s", got, tt.want)
			}
		})
	}
}
