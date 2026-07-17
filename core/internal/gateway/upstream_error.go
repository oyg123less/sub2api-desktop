package gateway

import (
	"net/http"
	"strings"

	"sub2api-desktop/core/internal/store"
)

const (
	statusReasonTransientRateLimit = "transient_rate_limit"
	statusReasonQuotaExhausted     = "quota_exhausted"
)

func classifyUpstreamHTTPError(status int, header http.Header, message string, acc *store.Account, usage *store.CodexUsage) forwardResult {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "upstream returned an error"
	}
	lower := strings.ToLower(message)

	if status == http.StatusTooManyRequests || (status == http.StatusForbidden && isQuotaFailure(lower)) {
		reason := statusReasonTransientRateLimit
		kind := "account_rate_limited"
		if isQuotaFailure(lower) {
			reason = statusReasonQuotaExhausted
			kind = "account_quota_exhausted"
		}
		event := "http_429"
		if status == http.StatusForbidden {
			event = "http_403_quota"
		}
		return forwardResult{
			outcome: outcomeRateLimited, status: status, errMsg: message,
			retryAfter: rateLimitRetryAfterForAccount(acc, header, usage), statusReason: reason,
			errorKind: kind, terminalEvent: event,
		}
	}

	if status == http.StatusUnauthorized || (status == http.StatusForbidden && isAuthenticationFailure(lower)) {
		return forwardResult{
			outcome: outcomeAuthFailed, status: status, errMsg: "authentication failed: " + message,
			errorKind: "account_unauthorized", terminalEvent: "http_auth_error",
		}
	}

	retryable := status >= http.StatusInternalServerError || status == http.StatusForbidden
	kind := "upstream_http_error"
	if status == http.StatusForbidden {
		kind = "upstream_forbidden"
	}
	return forwardResult{
		outcome: outcomeUpstreamError, status: status, errMsg: message, retryable: retryable,
		errorKind: kind, terminalEvent: "http_error",
	}
}

func isQuotaFailure(message string) bool {
	for _, marker := range []string{
		"insufficient_quota", "insufficient_user_quota", "usage_limit_reached", "quota_exceeded",
		"quota exhausted", "额度不足", "余额不足", "剩余额度",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isAuthenticationFailure(message string) bool {
	for _, marker := range []string{
		"invalid_api_key", "invalid api key", "invalid token", "token expired", "authentication",
		"unauthorized", "access denied", "forbidden", "credential",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
