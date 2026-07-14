package account

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

// LoginFlow holds the state of an in-progress OAuth PKCE login.
type LoginFlow struct {
	State        string
	CodeVerifier string
	RedirectURI  string
	AuthURL      string
	ProxyID      *int64
}

// NewLoginFlow prepares a fresh PKCE login flow and its authorization URL.
func NewLoginFlow(redirectURI string, proxyID *int64) (*LoginFlow, error) {
	state, err := openai.GenerateState()
	if err != nil {
		return nil, err
	}
	verifier, err := openai.GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}
	challenge := openai.GenerateCodeChallenge(verifier)
	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
	}
	return &LoginFlow{
		State:        state,
		CodeVerifier: verifier,
		RedirectURI:  redirectURI,
		AuthURL:      openai.BuildAuthorizationURL(state, challenge, redirectURI),
		ProxyID:      proxyID,
	}, nil
}

// Exchange trades an authorization code for tokens and persists a new (or
// updated) account. If an account with the same ChatGPT account id exists, its
// tokens are updated instead of creating a duplicate.
func (m *Manager) Exchange(ctx context.Context, client *http.Client, flow *LoginFlow, code string) (*store.Account, error) {
	tokReq := openai.BuildTokenRequest(code, flow.CodeVerifier, flow.RedirectURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openai.TokenURL, strings.NewReader(tokReq.ToFormData()))
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
		return nil, fmt.Errorf("code exchange failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var tok openai.TokenResponse
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	var info *openai.UserInfo
	if tok.IDToken != "" {
		if claims, err := openai.DecodeIDToken(tok.IDToken); err == nil {
			info = claims.GetUserInfo()
		}
	}

	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	if tok.ExpiresIn == 0 {
		expiresAt = time.Now().Add(1 * time.Hour)
	}

	email, cid, plan := "", "", ""
	if info != nil {
		email = info.Email
		cid = info.ChatGPTAccountID
		plan = info.PlanType
	}

	// Update existing account if it matches.
	if cid != "" {
		if existing, err := m.store.GetAccountByChatGPTID(cid); err == nil {
			newRefresh := tok.RefreshToken
			if newRefresh == "" {
				newRefresh = existing.RefreshToken
			}
			if err := m.store.UpdateTokens(existing.ID, tok.AccessToken, newRefresh, tok.IDToken, expiresAt); err != nil {
				return nil, err
			}
			if flow.ProxyID != nil {
				_ = m.store.SetAccountProxy(existing.ID, flow.ProxyID)
			}
			return m.store.GetAccount(existing.ID)
		}
	}

	acc := &store.Account{
		Email:            email,
		ChatGPTAccountID: cid,
		PlanType:         plan,
		AccessToken:      tok.AccessToken,
		RefreshToken:     tok.RefreshToken,
		IDToken:          tok.IDToken,
		ExpiresAt:        expiresAt,
		Status:           store.AccountActive,
		ProxyID:          flow.ProxyID,
	}
	return m.store.CreateAccount(acc)
}
