package gateway

import (
	"net/http"
	"testing"
	"time"

	"sub2api-desktop/core/internal/store"
)

func TestClassifyUpstreamHTTPError(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		message       string
		wantOutcome   forwardOutcome
		wantReason    string
		wantErrorKind string
		wantRetryable bool
	}{
		{
			name: "429 is a transient rate limit", status: http.StatusTooManyRequests,
			message: `{"error":{"message":"rate limited"}}`, wantOutcome: outcomeRateLimited,
			wantReason: statusReasonTransientRateLimit, wantErrorKind: "account_rate_limited",
		},
		{
			name: "403 quota body is quota exhaustion", status: http.StatusForbidden,
			message: `{"error":{"code":"insufficient_user_quota"}}`, wantOutcome: outcomeRateLimited,
			wantReason: statusReasonQuotaExhausted, wantErrorKind: "account_quota_exhausted",
		},
		{
			name: "403 invalid API key is authentication", status: http.StatusForbidden,
			message: `{"error":{"code":"invalid_api_key"}}`, wantOutcome: outcomeAuthFailed,
			wantErrorKind: "account_unauthorized",
		},
		{
			name: "unknown 403 is retryable upstream failure", status: http.StatusForbidden,
			message: `{"error":{"message":"policy restriction"}}`, wantOutcome: outcomeUpstreamError,
			wantErrorKind: "upstream_forbidden", wantRetryable: true,
		},
	}

	account := &store.Account{AccountType: store.AccountTypeAPIKey}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyUpstreamHTTPError(tt.status, http.Header{}, tt.message, account, nil)
			if got.outcome != tt.wantOutcome || got.statusReason != tt.wantReason || got.errorKind != tt.wantErrorKind || got.retryable != tt.wantRetryable {
				t.Fatalf("classification = %+v", got)
			}
		})
	}
}

func TestAPIKeyRateLimitWithoutHeadersUsesShortRetry(t *testing.T) {
	account := &store.Account{AccountType: store.AccountTypeAPIKey}
	got := classifyUpstreamHTTPError(http.StatusTooManyRequests, http.Header{}, "rate limited", account, nil)
	if got.retryAfter != 30*time.Second {
		t.Fatalf("retry after = %s, want 30s", got.retryAfter)
	}
}

func TestQuotaFailureWinsOverAuthenticationMarkers(t *testing.T) {
	message := `{"error":{"code":"insufficient_user_quota","message":"forbidden"}}`
	got := classifyUpstreamHTTPError(http.StatusForbidden, http.Header{}, message, &store.Account{}, nil)
	if got.outcome != outcomeRateLimited || got.statusReason != statusReasonQuotaExhausted {
		t.Fatalf("quota response was classified as an auth failure: %+v", got)
	}
}
