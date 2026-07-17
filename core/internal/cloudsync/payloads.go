package cloudsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"sub2api-desktop/core/internal/store"
)

type vaultEnvelope struct {
	UpdatedAt time.Time       `json:"updated_at"`
	Data      json.RawMessage `json:"data"`
}

type accountPayload struct {
	AccountType      store.AccountType   `json:"account_type"`
	BaseURL          string              `json:"base_url"`
	APIKey           string              `json:"api_key"`
	Email            string              `json:"email"`
	ChatGPTAccountID string              `json:"chatgpt_account_id"`
	PlanType         string              `json:"plan_type"`
	AccessToken      string              `json:"access_token"`
	RefreshToken     string              `json:"refresh_token"`
	IDToken          string              `json:"id_token"`
	ExpiresAt        time.Time           `json:"expires_at"`
	Status           store.AccountStatus `json:"status"`
	StatusReason     string              `json:"status_reason"`
	MaxConcurrency   int                 `json:"max_concurrency,omitempty"`
	QueueCapacity    *int                `json:"queue_capacity,omitempty"`
	ProxyUID         string              `json:"proxy_uid"`
	CreatedAt        time.Time           `json:"created_at"`
}

type proxyPayload struct {
	Name      string          `json:"name"`
	Type      store.ProxyType `json:"type"`
	Host      string          `json:"host"`
	Port      int             `json:"port"`
	Username  string          `json:"username"`
	Password  string          `json:"password"`
	CreatedAt time.Time       `json:"created_at"`
}

type codexRemotePayload struct {
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	User          string    `json:"user"`
	Password      string    `json:"password"`
	RemotePort    int       `json:"remote_port"`
	Model         string    `json:"model"`
	Mode          string    `json:"mode"`
	BaseURL       string    `json:"base_url"`
	APIKey        string    `json:"api_key"`
	TunnelEnabled bool      `json:"tunnel_enabled"`
	Injected      bool      `json:"injected"`
	CreatedAt     time.Time `json:"created_at"`
}

type settingsPayload struct {
	InjectInstr      bool   `json:"inject_instructions"`
	DefaultModel     string `json:"default_model"`
	UserAgent        string `json:"user_agent"`
	Originator       string `json:"originator"`
	Language         string `json:"language"`
	AutoStartServer  bool   `json:"auto_start_server"`
	TLSFingerprint   bool   `json:"tls_fingerprint"`
	CodexModel       string `json:"codex_model"`
	AccountStrategy  string `json:"account_strategy"`
	LogRetentionDays int    `json:"log_retention_days"`
	MaxLogRows       int    `json:"max_log_rows"`
	AutoRecovery     bool   `json:"auto_recovery"`
	CompatProfile    string `json:"compatibility_profile"`
}

func encryptEnvelope(vaultKey []byte, updatedAt time.Time, payload any) (string, error) {
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	envelope, err := json.Marshal(vaultEnvelope{UpdatedAt: updatedAt.UTC(), Data: data})
	if err != nil {
		return "", err
	}
	return encryptVaultItem(vaultKey, envelope)
}

func decryptEnvelope(vaultKey []byte, ciphertext string) (vaultEnvelope, error) {
	plaintext, err := decryptVaultItem(vaultKey, ciphertext)
	if err != nil {
		return vaultEnvelope{}, err
	}
	defer wipe(plaintext)
	var envelope vaultEnvelope
	if err := json.Unmarshal(plaintext, &envelope); err != nil {
		return vaultEnvelope{}, errors.New("invalid cloud vault envelope")
	}
	if envelope.UpdatedAt.IsZero() || len(envelope.Data) == 0 || len(envelope.Data) > 1024*1024 {
		return vaultEnvelope{}, errors.New("invalid cloud vault envelope")
	}
	return envelope, nil
}

func validateAccountPayload(payload accountPayload) error {
	switch payload.AccountType {
	case store.AccountTypeOAuth:
		if strings.TrimSpace(payload.AccessToken) == "" && strings.TrimSpace(payload.RefreshToken) == "" {
			return errors.New("OAuth account has no credential")
		}
	case store.AccountTypeAPIKey:
		if strings.TrimSpace(payload.APIKey) == "" {
			return errors.New("API-key account has no credential")
		}
		parsed, err := url.Parse(payload.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return errors.New("invalid API-key account base URL")
		}
	default:
		return errors.New("invalid account type")
	}
	switch payload.Status {
	case store.AccountActive, store.AccountRefreshFailed, store.AccountRateLimited, store.AccountDisabled, store.AccountPending:
	default:
		return errors.New("invalid account status")
	}
	if len(payload.Email) > 320 || len(payload.ChatGPTAccountID) > 512 || len(payload.StatusReason) > 2048 {
		return errors.New("account metadata is too large")
	}
	maxConcurrency, queueCapacity := accountPayloadLimits(payload)
	if err := store.ValidateAccountLimits(maxConcurrency, queueCapacity); err != nil {
		return err
	}
	return nil
}

func accountPayloadLimits(payload accountPayload) (int, int) {
	maxConcurrency := payload.MaxConcurrency
	if maxConcurrency == 0 {
		maxConcurrency = store.DefaultAccountMaxConcurrency
	}
	queueCapacity := store.DefaultAccountQueueCapacity
	if payload.QueueCapacity != nil {
		queueCapacity = *payload.QueueCapacity
	}
	return maxConcurrency, queueCapacity
}

func validateProxyPayload(payload proxyPayload) error {
	switch payload.Type {
	case store.ProxyHTTP, store.ProxyHTTPS, store.ProxySOCKS5:
	default:
		return errors.New("invalid proxy type")
	}
	if strings.TrimSpace(payload.Host) == "" || payload.Port < 1 || payload.Port > 65535 || len(payload.Name) > 256 || len(payload.Username) > 512 {
		return errors.New("invalid proxy")
	}
	return nil
}

func validateCodexRemotePayload(payload codexRemotePayload) error {
	if strings.TrimSpace(payload.Host) == "" || strings.TrimSpace(payload.User) == "" || payload.Port < 1 || payload.Port > 65535 {
		return errors.New("invalid Codex remote target")
	}
	if payload.Mode != "tunnel" && payload.Mode != "direct" {
		return errors.New("invalid Codex remote mode")
	}
	if payload.Mode == "tunnel" && (payload.RemotePort < 1 || payload.RemotePort > 65535) {
		return errors.New("invalid Codex remote port")
	}
	if payload.Mode == "direct" {
		parsed, err := url.Parse(payload.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || payload.APIKey == "" {
			return errors.New("invalid direct Codex target")
		}
	}
	return nil
}

func decodePayload[T any](envelope vaultEnvelope) (T, error) {
	var payload T
	if err := json.Unmarshal(envelope.Data, &payload); err != nil {
		return payload, fmt.Errorf("decode cloud payload: %w", err)
	}
	return payload, nil
}
