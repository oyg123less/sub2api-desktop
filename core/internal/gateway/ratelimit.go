package gateway

import (
	"net/http"
	"strconv"
	"time"

	"sub2api-desktop/core/internal/store"
)

// parseCodexUsageHeaders extracts the Codex rate-limit window headers
// (x-codex-primary-* = 7-day window, x-codex-secondary-* = 5-hour window)
// from an upstream response. Returns nil when no usage data is present.
func parseCodexUsageHeaders(h http.Header) *store.CodexUsage {
	u := &store.CodexUsage{}
	has := false

	parseFloat := func(key string) *float64 {
		if v := h.Get(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return &f
			}
		}
		return nil
	}
	parseInt := func(key string) *int {
		if v := h.Get(key); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return &i
			}
		}
		return nil
	}

	if v := parseFloat("x-codex-primary-used-percent"); v != nil {
		u.PrimaryUsedPercent = v
		has = true
	}
	if v := parseInt("x-codex-primary-reset-after-seconds"); v != nil {
		u.PrimaryResetAfterSeconds = v
		has = true
	}
	if v := parseInt("x-codex-primary-window-minutes"); v != nil {
		u.PrimaryWindowMinutes = v
		has = true
	}
	if v := parseFloat("x-codex-secondary-used-percent"); v != nil {
		u.SecondaryUsedPercent = v
		has = true
	}
	if v := parseInt("x-codex-secondary-reset-after-seconds"); v != nil {
		u.SecondaryResetAfterSeconds = v
		has = true
	}
	if v := parseInt("x-codex-secondary-window-minutes"); v != nil {
		u.SecondaryWindowMinutes = v
		has = true
	}

	if !has {
		return nil
	}
	u.UpdatedAt = time.Now()
	return u
}

// captureCodexUsage persists the latest usage snapshot for the account and
// returns it (nil when the response carried no usage headers).
func (e *Engine) captureCodexUsage(acc *store.Account, h http.Header) *store.CodexUsage {
	if acc == nil || acc.AccountType == store.AccountTypeAPIKey {
		return nil
	}
	u := parseCodexUsageHeaders(h)
	if u != nil {
		_ = e.store.SetAccountCodexUsage(acc.ID, u)
	}
	return u
}

// rateLimitRetryAfter derives how long an account should stay rate-limited,
// using the earliest positive Codex window reset, then Retry-After, and
// falling back to 10 minutes.
func rateLimitRetryAfter(h http.Header, usage *store.CodexUsage) time.Duration {
	resetSeconds := 0
	if usage != nil {
		for _, candidate := range []*int{usage.PrimaryResetAfterSeconds, usage.SecondaryResetAfterSeconds} {
			if candidate != nil && *candidate > 0 && (resetSeconds == 0 || *candidate < resetSeconds) {
				resetSeconds = *candidate
			}
		}
	}
	if resetSeconds > 0 {
		return time.Duration(resetSeconds) * time.Second
	}
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return 10 * time.Minute
}
