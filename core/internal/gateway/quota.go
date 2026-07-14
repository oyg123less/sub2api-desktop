package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"sub2api-desktop/core/internal/store"
)

const (
	defaultUsageURL   = "https://chatgpt.com/backend-api/wham/usage"
	usagePollInterval = 15 * time.Minute
)

type quotaWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAfterSeconds  int64   `json:"reset_after_seconds"`
}

type quotaPayload struct {
	RateLimit *struct {
		PrimaryWindow   *quotaWindow `json:"primary_window"`
		SecondaryWindow *quotaWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

func usageURL() string {
	if value := strings.TrimSpace(os.Getenv("SUB2API_USAGE_URL")); value != "" {
		return value
	}
	return defaultUsageURL
}

// MaintainUsageSnapshots refreshes OAuth quota data immediately and every 15
// minutes until the sidecar context is canceled.
func (e *Engine) MaintainUsageSnapshots(ctx context.Context) {
	e.RefreshUsageSnapshots(ctx)
	ticker := time.NewTicker(usagePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.RefreshUsageSnapshots(ctx)
		}
	}
}

// RefreshUsageSnapshots performs one active quota polling pass. Individual
// account failures are isolated and never change account status here.
func (e *Engine) RefreshUsageSnapshots(ctx context.Context) {
	accounts, err := e.store.ListAccounts()
	if err != nil {
		e.logger.Warn("active quota account lookup failed", "error", err)
		return
	}
	for _, acc := range accounts {
		if ctx.Err() != nil {
			return
		}
		if acc.AccountType != store.AccountTypeOAuth || acc.Status != store.AccountActive || strings.TrimSpace(acc.ChatGPTAccountID) == "" {
			continue
		}
		if err := e.refreshAccountUsage(ctx, acc); err != nil {
			e.logger.Warn("active quota query failed", "account_id", acc.ID, "error", err)
		}
	}
}

func (e *Engine) refreshAccountUsage(ctx context.Context, acc *store.Account) error {
	var proxy *store.Proxy
	if acc.ProxyID != nil {
		var err error
		proxy, err = e.store.GetProxy(*acc.ProxyID)
		if err != nil {
			return fmt.Errorf("load account proxy: %w", err)
		}
	}
	cfg := e.settings()
	client, err := newHTTPClient(proxy, cfg.CompatProfile, 20*time.Second)
	if err != nil {
		return fmt.Errorf("build quota client: %w", err)
	}
	authClient, err := newHTTPClient(proxy, "standard", 20*time.Second)
	if err != nil {
		return fmt.Errorf("build quota auth client: %w", err)
	}
	token, err := e.accounts.ValidAccessToken(ctx, authClient, acc)
	if err != nil {
		return fmt.Errorf("acquire access token: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("chatgpt-account-id", acc.ChatGPTAccountID)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("OpenAI-Beta", "codex-1")
	request.Header.Set("originator", "Codex Desktop")
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = store.DefaultUserAgent
	}
	request.Header.Set("User-Agent", userAgent)

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request usage: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("usage endpoint returned HTTP %d", response.StatusCode)
	}
	usage, err := decodeQuotaUsage(response)
	if err != nil {
		return err
	}
	if err := e.store.SetAccountCodexUsage(acc.ID, usage); err != nil {
		return fmt.Errorf("persist usage snapshot: %w", err)
	}
	return nil
}

func decodeQuotaUsage(response *http.Response) (*store.CodexUsage, error) {
	var payload quotaPayload
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode usage response: %w", err)
	}
	if payload.RateLimit == nil || (payload.RateLimit.PrimaryWindow == nil && payload.RateLimit.SecondaryWindow == nil) {
		return nil, errors.New("usage response contains no rate-limit windows")
	}
	usage := &store.CodexUsage{UpdatedAt: time.Now()}
	if window := payload.RateLimit.PrimaryWindow; window != nil {
		used, reset, minutes := window.UsedPercent, int(window.ResetAfterSeconds), int(window.LimitWindowSeconds/60)
		usage.PrimaryUsedPercent = &used
		usage.PrimaryResetAfterSeconds = &reset
		usage.PrimaryWindowMinutes = &minutes
	}
	if window := payload.RateLimit.SecondaryWindow; window != nil {
		used, reset, minutes := window.UsedPercent, int(window.ResetAfterSeconds), int(window.LimitWindowSeconds/60)
		usage.SecondaryUsedPercent = &used
		usage.SecondaryResetAfterSeconds = &reset
		usage.SecondaryWindowMinutes = &minutes
	}
	return usage, nil
}
