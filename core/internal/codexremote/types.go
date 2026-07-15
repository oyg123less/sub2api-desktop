package codexremote

import (
	"context"
	"fmt"
	"net"
	"time"

	"sub2api-desktop/core/internal/store"
)

const (
	StatusConnected      = "connected"
	StatusDown           = "down"
	StatusDisabled       = "disabled"
	StatusNotInjected    = "not_injected"
	StatusInjectedDirect = "injected_direct"

	ModeTunnel = "tunnel"
	ModeDirect = "direct"
)

type Error struct {
	Code        string
	Fingerprint string
	cause       error
}

func (e *Error) Error() string {
	switch e.Code {
	case "auth_failed":
		return "SSH authentication failed"
	case "host_key_unknown":
		return "SSH host key confirmation is required"
	case "host_key_mismatch":
		return "SSH host key does not match the trusted key"
	case "unsupported_os":
		return "only Linux and macOS remote servers are supported"
	case "invalid_target":
		return "SSH target is invalid"
	case "target_not_found":
		return "remote target was not found"
	case "tunnel_failed":
		return "SSH reverse tunnel could not be established"
	case "tunnel_not_applicable":
		return "SSH tunnel controls do not apply to direct targets"
	case "remote_command_failed":
		return "remote Codex configuration command failed"
	default:
		return "SSH connection failed"
	}
}

func (e *Error) Unwrap() error { return e.cause }

func codedError(code string, cause error) error { return &Error{Code: code, cause: cause} }

type ProbeRequest struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type Probe struct {
	OS                 string `json:"os"`
	Home               string `json:"home"`
	CodexDir           string `json:"codex_dir"`
	HostKeyFingerprint string `json:"host_key_fingerprint"`
	Known              bool   `json:"known"`
}

type InjectRequest struct {
	ID            int64  `json:"id,omitempty"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	User          string `json:"user"`
	Password      string `json:"password"`
	Model         string `json:"model"`
	RemotePort    int    `json:"remote_port"`
	Save          bool   `json:"save"`
	AcceptHostKey bool   `json:"accept_host_key"`
	Mode          string `json:"mode"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	Config        string `json:"-"`
	Auth          string `json:"-"`
}

type TargetStatus struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	User          string    `json:"user"`
	RemotePort    int       `json:"remote_port"`
	Model         string    `json:"model"`
	Mode          string    `json:"mode"`
	BaseURL       string    `json:"base_url,omitempty"`
	Saved         bool      `json:"saved"`
	Injected      bool      `json:"injected"`
	TunnelEnabled bool      `json:"tunnel_enabled"`
	TunnelStatus  string    `json:"tunnel_status"`
	LastError     string    `json:"last_error,omitempty"`
	ConfigPreview string    `json:"config_preview"`
	AuthPreview   string    `json:"auth_preview"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type targetStore interface {
	CreateCodexRemoteTarget(*store.CodexRemoteTarget) (*store.CodexRemoteTarget, error)
	UpdateCodexRemoteTarget(*store.CodexRemoteTarget) (*store.CodexRemoteTarget, error)
	GetCodexRemoteTarget(int64) (*store.CodexRemoteTarget, error)
	ListCodexRemoteTargets() ([]*store.CodexRemoteTarget, error)
	DeleteCodexRemoteTarget(int64) error
}

type remoteConnection interface {
	Run(context.Context, string, []byte) ([]byte, error)
	Listen(string, string) (net.Listener, error)
	SendRequest(string, bool, []byte) (bool, []byte, error)
	Close() error
}

type dialResult struct {
	connection  remoteConnection
	fingerprint string
	known       bool
}

type dialFunc func(context.Context, ProbeRequest, bool, bool) (*dialResult, error)

type remoteOperations interface {
	Probe(context.Context) (Probe, error)
	Inject(context.Context, string, string, string) error
	Restore(context.Context, string) error
}

type remoteFactory func(remoteConnection) remoteOperations

func address(host string, port int) string { return net.JoinHostPort(host, fmt.Sprintf("%d", port)) }
