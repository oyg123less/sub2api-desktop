// Package account manages OAuth token lifecycle: exchange, refresh (with
// per-account single-flight), and persistence.
package account

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

// Manager coordinates token refresh across concurrent requests.
type Manager struct {
	store *store.Store

	mu    sync.Mutex
	locks map[int64]*sync.Mutex
}

// NewManager creates an account manager.
func NewManager(s *store.Store) *Manager {
	return &Manager{store: s, locks: make(map[int64]*sync.Mutex)}
}

func (m *Manager) lockFor(id int64) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.locks[id]
	if !ok {
		l = &sync.Mutex{}
		m.locks[id] = l
	}
	return l
}

// refreshSkew refreshes tokens this long before actual expiry.
const refreshSkew = 5 * time.Minute

// ValidAccessToken returns a non-expired access token for the account,
// refreshing it if necessary. It reloads the account from the store after
// refreshing so callers see the latest tokens.
func (m *Manager) ValidAccessToken(ctx context.Context, client *http.Client, acc *store.Account) (string, error) {
	if !acc.ExpiresAt.IsZero() && time.Now().Before(acc.ExpiresAt.Add(-refreshSkew)) {
		return acc.AccessToken, nil
	}
	if acc.RefreshToken == "" {
		return acc.AccessToken, nil // nothing to refresh with; use as-is
	}

	l := m.lockFor(acc.ID)
	l.Lock()
	defer l.Unlock()

	// Re-read after acquiring the lock: another goroutine may have refreshed.
	fresh, err := m.store.GetAccount(acc.ID)
	if err != nil {
		return "", err
	}
	if !fresh.ExpiresAt.IsZero() && time.Now().Before(fresh.ExpiresAt.Add(-refreshSkew)) {
		*acc = *fresh
		return fresh.AccessToken, nil
	}

	tok, err := m.doRefresh(ctx, client, fresh.RefreshToken)
	if err != nil {
		_ = m.store.SetAccountStatus(acc.ID, store.AccountRefreshFailed, err.Error())
		return "", err
	}

	newRefresh := tok.RefreshToken
	if newRefresh == "" {
		newRefresh = fresh.RefreshToken
	}
	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	if tok.ExpiresIn == 0 {
		expiresAt = time.Now().Add(1 * time.Hour)
	}
	if err := m.store.UpdateTokens(acc.ID, tok.AccessToken, newRefresh, tok.IDToken, expiresAt); err != nil {
		return "", err
	}
	updated, err := m.store.GetAccount(acc.ID)
	if err != nil {
		return "", err
	}
	*acc = *updated
	return updated.AccessToken, nil
}

// Refresh forces a token refresh for an account (used by the "re-login not
// needed, just refresh" path).
func (m *Manager) Refresh(ctx context.Context, client *http.Client, id int64) error {
	acc, err := m.store.GetAccount(id)
	if err != nil {
		return err
	}
	if acc.RefreshToken == "" {
		return fmt.Errorf("account has no refresh token; re-login required")
	}
	l := m.lockFor(id)
	l.Lock()
	defer l.Unlock()

	tok, err := m.doRefresh(ctx, client, acc.RefreshToken)
	if err != nil {
		_ = m.store.SetAccountStatus(id, store.AccountRefreshFailed, err.Error())
		return err
	}
	newRefresh := tok.RefreshToken
	if newRefresh == "" {
		newRefresh = acc.RefreshToken
	}
	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return m.store.UpdateTokens(id, tok.AccessToken, newRefresh, tok.IDToken, expiresAt)
}

func (m *Manager) doRefresh(ctx context.Context, client *http.Client, refreshToken string) (*openai.TokenResponse, error) {
	body := openai.BuildRefreshTokenRequest(refreshToken).ToFormData()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openai.TokenURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token refresh failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var tok openai.TokenResponse
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token refresh returned empty access_token")
	}
	return &tok, nil
}
