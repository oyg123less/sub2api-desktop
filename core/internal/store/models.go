package store

import "time"

// AccountStatus enumerates the lifecycle states of an OpenAI account.
type AccountStatus string

const (
	AccountActive        AccountStatus = "active"
	AccountRefreshFailed AccountStatus = "refresh_failed"
	AccountRateLimited   AccountStatus = "rate_limited"
	AccountDisabled      AccountStatus = "disabled"
)

// CodexUsage captures the Codex rate-limit windows reported by upstream
// x-codex-* response headers (primary = 7-day window, secondary = 5-hour
// window). Pointers distinguish "not reported" from zero.
type CodexUsage struct {
	PrimaryUsedPercent         *float64  `json:"primary_used_percent,omitempty"`
	PrimaryResetAfterSeconds   *int      `json:"primary_reset_after_seconds,omitempty"`
	PrimaryWindowMinutes       *int      `json:"primary_window_minutes,omitempty"`
	SecondaryUsedPercent       *float64  `json:"secondary_used_percent,omitempty"`
	SecondaryResetAfterSeconds *int      `json:"secondary_reset_after_seconds,omitempty"`
	SecondaryWindowMinutes     *int      `json:"secondary_window_minutes,omitempty"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// Account is a single ChatGPT OAuth account. Token fields are stored encrypted
// at rest and decrypted only in memory.
type Account struct {
	ID               int64         `json:"id"`
	Email            string        `json:"email"`
	ChatGPTAccountID string        `json:"chatgpt_account_id"`
	PlanType         string        `json:"plan_type"`
	AccessToken      string        `json:"-"`
	RefreshToken     string        `json:"-"`
	IDToken          string        `json:"-"`
	ExpiresAt        time.Time     `json:"expires_at"`
	Status           AccountStatus `json:"status"`
	StatusReason     string        `json:"status_reason,omitempty"`
	RateLimitedUntil *time.Time    `json:"rate_limited_until,omitempty"`
	ProxyID          *int64        `json:"proxy_id,omitempty"`
	LastUsedAt       *time.Time    `json:"last_used_at,omitempty"`
	CodexUsage       *CodexUsage   `json:"codex_usage,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

// ProxyType enumerates supported proxy protocols.
type ProxyType string

const (
	ProxyHTTP   ProxyType = "http"
	ProxyHTTPS  ProxyType = "https"
	ProxySOCKS5 ProxyType = "socks5"
)

// Proxy is an outbound proxy that can be bound to accounts. Credentials are
// stored encrypted at rest.
type Proxy struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      ProxyType `json:"type"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username,omitempty"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// RequestLog records one proxied request for statistics and diagnostics.
type RequestLog struct {
	ID               int64     `json:"id"`
	AccountID        *int64    `json:"account_id,omitempty"`
	AccountEmail     string    `json:"account_email,omitempty"`
	Model            string    `json:"model"`
	StatusCode       int       `json:"status_code"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	LatencyMS        int64     `json:"latency_ms"`
	Stream           bool      `json:"stream"`
	Error            string    `json:"error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Settings holds user-configurable application settings.
type Settings struct {
	ListenPort      int    `json:"listen_port"`
	AllowLAN        bool   `json:"allow_lan"`
	LocalAPIKey     string `json:"local_api_key"`
	InjectInstr     bool   `json:"inject_instructions"`
	DefaultModel    string `json:"default_model"`
	UserAgent       string `json:"user_agent"`
	Originator      string `json:"originator"`
	Language        string `json:"language"`
	AutoStartServer bool   `json:"auto_start_server"`
	TLSFingerprint  bool   `json:"tls_fingerprint"`
	// CodexModel is the model written into ~/.codex/config.toml when applying
	// the Codex CLI integration.
	CodexModel string `json:"codex_model"`
}
