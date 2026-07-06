package account

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

// ImportEntry is a single account record accepted by batch import. Only the
// token fields are required; identity fields (email / chatgpt_account_id /
// plan_type) are best-effort derived from id_token when present and otherwise
// taken from the entry.
type ImportEntry struct {
	Email            string `json:"email"`
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	PlanType         string `json:"plan_type"`
	ExpiresAt        string `json:"expires_at"`
}

// ImportResult summarizes the outcome of a batch import.
type ImportResult struct {
	Imported int      `json:"imported"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// Import upserts a batch of accounts from raw token data (e.g. exported from
// another tool). Accounts are matched by chatgpt_account_id; a match updates
// the existing tokens instead of creating a duplicate. Entries lacking both an
// access token and a refresh token are skipped.
func (m *Manager) Import(entries []ImportEntry) ImportResult {
	var res ImportResult
	for i, e := range entries {
		e.AccessToken = strings.TrimSpace(e.AccessToken)
		e.RefreshToken = strings.TrimSpace(e.RefreshToken)
		e.IDToken = strings.TrimSpace(e.IDToken)
		if e.AccessToken == "" && e.RefreshToken == "" {
			res.Skipped++
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 条: 缺少 access_token 和 refresh_token", i+1))
			continue
		}

		email, cid, plan := e.Email, e.ChatGPTAccountID, e.PlanType
		if e.IDToken != "" {
			if claims, err := openai.DecodeIDToken(e.IDToken); err == nil {
				info := claims.GetUserInfo()
				if info.Email != "" {
					email = info.Email
				}
				if info.ChatGPTAccountID != "" {
					cid = info.ChatGPTAccountID
				}
				if info.PlanType != "" {
					plan = info.PlanType
				}
			}
		}

		expiresAt := parseExpiry(e.ExpiresAt)

		if cid != "" {
			if existing, err := m.store.GetAccountByChatGPTID(cid); err == nil {
				access := e.AccessToken
				refresh := e.RefreshToken
				if refresh == "" {
					refresh = existing.RefreshToken
				}
				if err := m.store.UpdateTokens(existing.ID, access, refresh, e.IDToken, expiresAt); err != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("第 %d 条: 更新失败: %v", i+1, err))
					res.Skipped++
					continue
				}
				res.Updated++
				continue
			}
		}

		acc := &store.Account{
			Email:            email,
			ChatGPTAccountID: cid,
			PlanType:         plan,
			AccessToken:      e.AccessToken,
			RefreshToken:     e.RefreshToken,
			IDToken:          e.IDToken,
			ExpiresAt:        expiresAt,
			Status:           store.AccountActive,
		}
		if _, err := m.store.CreateAccount(acc); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 条: 创建失败: %v", i+1, err))
			res.Skipped++
			continue
		}
		res.Imported++
	}
	return res
}

// parseExpiry accepts an RFC3339 timestamp or a unix-seconds string. A zero
// value is returned when empty/unparseable so the token is refreshed on first
// use (when a refresh token is present).
func parseExpiry(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		if n > 1e12 { // unix milliseconds
			return time.UnixMilli(n)
		}
		return time.Unix(n, 0)
	}
	return time.Time{}
}
