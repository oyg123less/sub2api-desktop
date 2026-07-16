// Package control implements the loopback control API used by the desktop
// shell/frontend to manage accounts, proxies, settings, statistics and the
// local API server lifecycle. It listens only on 127.0.0.1 and is protected by
// a random per-session token.
package control

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/cloudsync"
	"sub2api-desktop/core/internal/codexremote"
	"sub2api-desktop/core/internal/diagnostics"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/redact"
	"sub2api-desktop/core/internal/store"
)

// ServerController lets the control API report and toggle the local API server.
type ServerController interface {
	Running() bool
	Port() int
	Start() error
	Stop() error
	Restart() error
}

// SettingsAccess abstracts reading and persisting settings, so the control API
// can update live configuration owned by main.
type SettingsAccess interface {
	Get() store.Settings
	Save(store.Settings) error
}

type CodexRemoteController interface {
	Probe(context.Context, codexremote.ProbeRequest) (codexremote.Probe, error)
	Inject(context.Context, codexremote.InjectRequest) (codexremote.TargetStatus, error)
	Targets() ([]codexremote.TargetStatus, error)
	SetTunnel(context.Context, int64, bool) (codexremote.TargetStatus, error)
	Restore(context.Context, int64) (codexremote.TargetStatus, error)
	Delete(int64) error
}

type CloudController interface {
	Status() cloudsync.Status
	Register(context.Context, cloudsync.RegisterInput) error
	VerifyEmail(context.Context, string, string) error
	ResendVerification(context.Context, string) error
	CancelRegistration()
	Login(context.Context, string, string) error
	Logout(context.Context) error
	Sync(context.Context) error
	ChangePassword(context.Context, string, string) error
	AdminOverview(context.Context, string) (cloudsync.AdminOverview, error)
	AdminSetUserBanned(context.Context, string, int64, bool) error
	AdminLogoutUser(context.Context, string, int64) error
	AdminDeleteUser(context.Context, string, int64) error
	AdminUpdateSettings(context.Context, string, map[string]bool) error
	AdminSetShareRevoked(context.Context, string, int64, bool) error
	ListShares(context.Context) ([]cloudsync.Share, error)
	CreateShare(context.Context, cloudsync.CreateShareInput) (cloudsync.CreatedShare, error)
	UpdateShare(context.Context, int64, map[string]any) (cloudsync.Share, error)
	ShareUsage(context.Context, int64) ([]cloudsync.ShareUsage, error)
}

// Control holds control API dependencies.
type Control struct {
	store       *store.Store
	mgr         *account.Manager
	oauth       *oauthCoordinator
	settings    SettingsAccess
	server      ServerController
	engine      *gateway.Engine
	diagnostics *diagnostics.Service
	updates     *updateChecker
	remoteCodex CodexRemoteController
	cloud       CloudController
	token       string
	version     string
}

func (c *Control) SetCloudController(controller CloudController) {
	c.cloud = controller
}

// New builds the control API.
func New(s *store.Store, mgr *account.Manager, settings SettingsAccess, server ServerController, engine *gateway.Engine, diagnosticService *diagnostics.Service, remoteCodex CodexRemoteController, token, version string) *Control {
	return &Control{
		store:       s,
		mgr:         mgr,
		oauth:       newOAuthCoordinator(mgr, s, settings.Get),
		settings:    settings,
		server:      server,
		engine:      engine,
		diagnostics: diagnosticService,
		updates:     newUpdateChecker(s, settings, version),
		remoteCodex: remoteCodex,
		token:       token,
		version:     version,
	}
}

// Mount registers control routes.
func (c *Control) Mount(mux *http.ServeMux) {
	h := c.authWrap
	mux.HandleFunc("GET /control/status", h(c.status))
	mux.HandleFunc("GET /control/update", h(c.latestRelease))
	mux.HandleFunc("POST /control/server/start", h(c.serverStart))
	mux.HandleFunc("POST /control/server/stop", h(c.serverStop))

	mux.HandleFunc("GET /control/settings", h(c.getSettings))
	mux.HandleFunc("PUT /control/settings", h(c.putSettings))
	mux.HandleFunc("POST /control/settings/regenerate-key", h(c.regenKey))

	mux.HandleFunc("GET /control/accounts", h(c.listAccounts))
	mux.HandleFunc("POST /control/accounts/import", h(c.importAccounts))
	mux.HandleFunc("POST /control/accounts/import/preview", h(c.previewImportAccounts))
	mux.HandleFunc("POST /control/accounts/import/commit", h(c.commitImportAccounts))
	mux.HandleFunc("DELETE /control/accounts/{id}", h(c.deleteAccount))
	mux.HandleFunc("POST /control/accounts/{id}/refresh", h(c.refreshAccount))
	mux.HandleFunc("POST /control/accounts/{id}/proxy", h(c.bindProxy))
	mux.HandleFunc("POST /control/accounts/{id}/test", h(c.testAccount))
	mux.HandleFunc("POST /control/accounts/{id}/status", h(c.setAccountStatus))

	mux.HandleFunc("POST /control/oauth/start", h(c.oauthStart))
	mux.HandleFunc("GET /control/oauth/poll", h(c.oauthPoll))

	mux.HandleFunc("GET /control/proxies", h(c.listProxies))
	mux.HandleFunc("POST /control/proxies", h(c.createProxy))
	mux.HandleFunc("PUT /control/proxies/{id}", h(c.updateProxy))
	mux.HandleFunc("DELETE /control/proxies/{id}", h(c.deleteProxy))
	mux.HandleFunc("POST /control/proxies/{id}/test", h(c.testProxy))

	mux.HandleFunc("GET /control/codex/status", h(c.codexStatus))
	mux.HandleFunc("POST /control/codex/apply", h(c.codexApply))
	mux.HandleFunc("POST /control/codex/restore", h(c.codexRestore))
	mux.HandleFunc("GET /control/codex/files", h(c.codexFiles))
	mux.HandleFunc("PUT /control/codex/files", h(c.codexWriteFiles))
	mux.HandleFunc("POST /control/codex/remote/test", h(c.codexRemoteTest))
	mux.HandleFunc("POST /control/codex/remote/inject", h(c.codexRemoteInject))
	mux.HandleFunc("GET /control/codex/remote/targets", h(c.codexRemoteTargets))
	mux.HandleFunc("POST /control/codex/remote/{id}/tunnel", h(c.codexRemoteTunnel))
	mux.HandleFunc("POST /control/codex/remote/{id}/restore", h(c.codexRemoteRestore))
	mux.HandleFunc("DELETE /control/codex/remote/{id}", h(c.codexRemoteDelete))

	mux.HandleFunc("GET /control/cloud/status", h(c.cloudStatus))
	mux.HandleFunc("POST /control/cloud/register", h(c.cloudRegister))
	mux.HandleFunc("POST /control/cloud/verify-email", h(c.cloudVerifyEmail))
	mux.HandleFunc("POST /control/cloud/resend-verification", h(c.cloudResendVerification))
	mux.HandleFunc("POST /control/cloud/cancel-registration", h(c.cloudCancelRegistration))
	mux.HandleFunc("POST /control/cloud/login", h(c.cloudLogin))
	mux.HandleFunc("POST /control/cloud/logout", h(c.cloudLogout))
	mux.HandleFunc("POST /control/cloud/sync", h(c.cloudSync))
	mux.HandleFunc("PUT /control/cloud/master-password", h(c.cloudChangePassword))
	mux.HandleFunc("POST /control/cloud/admin/overview", h(c.cloudAdminOverview))
	mux.HandleFunc("PATCH /control/cloud/admin/users/{id}", h(c.cloudAdminSetUserBanned))
	mux.HandleFunc("POST /control/cloud/admin/users/{id}/logout-all", h(c.cloudAdminLogoutUser))
	mux.HandleFunc("DELETE /control/cloud/admin/users/{id}", h(c.cloudAdminDeleteUser))
	mux.HandleFunc("PATCH /control/cloud/admin/settings", h(c.cloudAdminUpdateSettings))
	mux.HandleFunc("PATCH /control/cloud/admin/shares/{id}", h(c.cloudAdminSetShareRevoked))
	mux.HandleFunc("GET /control/cloud/shares", h(c.cloudShares))
	mux.HandleFunc("POST /control/cloud/shares", h(c.cloudCreateShare))
	mux.HandleFunc("PATCH /control/cloud/shares/{id}", h(c.cloudUpdateShare))
	mux.HandleFunc("GET /control/cloud/shares/{id}/usage", h(c.cloudShareUsage))

	mux.HandleFunc("GET /control/models", h(c.listModels))
	mux.HandleFunc("GET /control/pricing", h(c.pricing))

	mux.HandleFunc("GET /control/logs", h(c.logs))
	mux.HandleFunc("GET /control/logs/export", h(c.exportLogs))
	mux.HandleFunc("DELETE /control/logs", h(c.clearLogs))
	mux.HandleFunc("GET /control/stats", h(c.stats))
	mux.HandleFunc("POST /control/diagnostics/runs", h(c.createDiagnosticRun))
	mux.HandleFunc("GET /control/diagnostics/runs/{id}", h(c.getDiagnosticRun))
	mux.HandleFunc("GET /control/diagnostics/runs/{id}/report", h(c.diagnosticReport))
}

// WithCORS wraps a handler with permissive CORS headers and handles preflight
// OPTIONS requests. The control API is loopback-only and token-protected, so
// allowing any origin is safe and lets the WebView (or a dev browser on a
// different port) reach it.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		allowHeaders := "Content-Type, X-Control-Token, Authorization, X-Import-Preview-SHA256, X-Validate-After-Import, X-Confirm-Clear"
		if requested := r.Header.Get("Access-Control-Request-Headers"); requested != "" {
			allowHeaders = requested
		}
		w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (c *Control) authWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-Control-Token")
		if provided == "" {
			provided = extractBearer(r.Header.Get("Authorization"))
		}
		if provided != c.token {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid control token"})
			return
		}
		next(w, r)
	}
}

func extractBearer(h string) string {
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(h)
}

// --- status / server ---

func (c *Control) status(w http.ResponseWriter, r *http.Request) {
	accounts, _ := c.store.ListAccounts()
	cfg := c.settings.Get()
	host := "127.0.0.1"
	if cfg.AllowLAN {
		host = "0.0.0.0"
	}
	lanAddresses := localLANAddresses(c.server.Port())
	writeJSON(w, http.StatusOK, map[string]any{
		"version":          c.version,
		"server_running":   c.server.Running(),
		"port":             c.server.Port(),
		"host":             host,
		"endpoint":         "http://127.0.0.1:" + strconv.Itoa(c.server.Port()) + "/v1",
		"lan_addresses":    lanAddresses,
		"local_api_key":    cfg.LocalAPIKey,
		"account_count":    len(accounts),
		"schema_version":   c.store.SchemaVersion(),
		"migration_backup": c.store.MigrationBackup(),
	})
}

func (c *Control) createDiagnosticRun(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, c.diagnostics.Start())
}

func (c *Control) getDiagnosticRun(w http.ResponseWriter, r *http.Request) {
	run, err := c.diagnostics.Get(r.PathValue("id"))
	if errors.Is(err, diagnostics.ErrRunNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "diagnostic run not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (c *Control) diagnosticReport(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "text" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "format must be json or text"})
		return
	}
	data, contentType, err := c.diagnostics.Report(r.PathValue("id"), format)
	if errors.Is(err, diagnostics.ErrRunNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "diagnostic run not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="amber-diagnostics-%s.%s"`, r.PathValue("id"), format))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (c *Control) serverStart(w http.ResponseWriter, r *http.Request) {
	if err := c.server.Start(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"server_running": true, "port": c.server.Port()})
}

func (c *Control) serverStop(w http.ResponseWriter, r *http.Request) {
	if err := c.server.Stop(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"server_running": false})
}

// --- settings ---

func (c *Control) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, c.settings.Get())
}

func (c *Control) putSettings(w http.ResponseWriter, r *http.Request) {
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	var in store.Settings
	if err := json.Unmarshal(encoded, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	cur := c.settings.Get()
	for key, values := range map[string]struct {
		target  *bool
		current bool
	}{
		"allow_lan":           {target: &in.AllowLAN, current: cur.AllowLAN},
		"inject_instructions": {target: &in.InjectInstr, current: cur.InjectInstr},
		"auto_start_server":   {target: &in.AutoStartServer, current: cur.AutoStartServer},
		"tls_fingerprint":     {target: &in.TLSFingerprint, current: cur.TLSFingerprint},
		"auto_recovery":       {target: &in.AutoRecovery, current: cur.AutoRecovery},
	} {
		if _, exists := fields[key]; !exists {
			*values.target = values.current
		}
	}
	// Preserve the API key unless explicitly regenerated.
	if in.LocalAPIKey == "" {
		in.LocalAPIKey = cur.LocalAPIKey
	}
	if in.ListenPort == 0 {
		in.ListenPort = cur.ListenPort
	}
	if in.ListenPort < 1 || in.ListenPort > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "端口无效：必须在 1-65535 之间"})
		return
	}
	if in.CodexModel == "" {
		in.CodexModel = cur.CodexModel
	}
	if in.DefaultModel == "" {
		in.DefaultModel = cur.DefaultModel
	}
	if in.UserAgent == "" {
		in.UserAgent = cur.UserAgent
	}
	if in.Originator == "" {
		in.Originator = cur.Originator
	}
	if in.Language == "" {
		in.Language = cur.Language
	}
	if in.AccountStrategy == "" {
		in.AccountStrategy = cur.AccountStrategy
	}
	switch in.AccountStrategy {
	case gateway.StrategyFailover, gateway.StrategyRoundRobin, gateway.StrategyQuotaAware:
	default:
		writeControlError(w, http.StatusBadRequest, "invalid_account_strategy", "invalid account scheduling strategy", false, nil)
		return
	}
	if in.LogRetentionDays != 0 && in.LogRetentionDays != 7 && in.LogRetentionDays != 30 && in.LogRetentionDays != 90 {
		writeControlError(w, http.StatusBadRequest, "invalid_log_retention", "log retention must be 0, 7, 30, or 90 days", false, nil)
		return
	}
	if in.MaxLogRows == 0 {
		in.MaxLogRows = cur.MaxLogRows
	}
	if in.MaxLogRows < 1000 || in.MaxLogRows > 1000000 {
		writeControlError(w, http.StatusBadRequest, "invalid_max_log_rows", "max log rows must be between 1,000 and 1,000,000", false, nil)
		return
	}
	if in.CompatProfile == "" {
		in.CompatProfile = cur.CompatProfile
	}
	switch in.CompatProfile {
	case "standard":
		if _, provided := fields["compatibility_profile"]; provided {
			in.TLSFingerprint = false
		}
	case "codex":
		if _, provided := fields["compatibility_profile"]; provided {
			in.TLSFingerprint = true
		}
	default:
		writeControlError(w, http.StatusBadRequest, "invalid_compatibility_profile", "compatibility profile must be standard or codex", false, nil)
		return
	}
	if err := c.settings.Save(in); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	listenChanged := in.ListenPort != cur.ListenPort || in.AllowLAN != cur.AllowLAN
	if listenChanged && c.server != nil && c.server.Running() {
		if err := c.server.Restart(); err != nil {
			rollbackErr := c.settings.Save(cur)
			message := err.Error()
			if rollbackErr != nil {
				message += "; restore settings: " + rollbackErr.Error()
			}
			writeControlError(w, http.StatusInternalServerError, "server_restart_failed", message, true, nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, c.settings.Get())
}

func localLANAddresses(port int) []string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return []string{}
	}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, address := range addresses {
		var ip net.IP
		switch value := address.(type) {
		case *net.IPNet:
			ip = value.IP
		case *net.IPAddr:
			ip = value.IP
		}
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.To4() == nil {
			continue
		}
		value := "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)) + "/v1"
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func (c *Control) regenKey(w http.ResponseWriter, r *http.Request) {
	cur := c.settings.Get()
	cur.LocalAPIKey = store.GenerateLocalAPIKey()
	if err := c.settings.Save(cur); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"local_api_key": cur.LocalAPIKey})
}

// --- accounts ---

// accountUsage is the per-account usage + cost roll-up returned alongside the
// account list.
type accountUsage struct {
	AccountID        int64   `json:"account_id"`
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

func (c *Control) usageByAccount() map[int64]*accountUsage {
	rows, err := c.store.UsageByAccountModel()
	if err != nil {
		return nil
	}
	out := make(map[int64]*accountUsage)
	for _, r := range rows {
		u := out[r.AccountID]
		if u == nil {
			u = &accountUsage{AccountID: r.AccountID}
			out[r.AccountID] = u
		}
		u.Requests += r.Requests
		u.PromptTokens += r.PromptTokens
		u.CachedTokens += r.CachedTokens
		u.CompletionTokens += r.CompletionTokens
		u.ReasoningTokens += r.ReasoningTokens
		u.TotalTokens += r.TotalTokens
		u.CostUSD += openai.CostUSD(r.Model, r.PromptTokens, r.CachedTokens, r.CompletionTokens)
	}
	return out
}

func (c *Control) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := c.store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	usage := c.usageByAccount()
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts, "usage": usage})
}

// testAccount runs a live connectivity probe through the full anti-ban pipeline
// and updates the account status based on the result.
func (c *Control) testAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	acc, err := c.store.GetAccount(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "account not found"})
		return
	}
	var body struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
	defer cancel()
	res := c.engine.TestAccount(ctx, acc, body.Model, body.Prompt)
	writeJSON(w, http.StatusOK, res)
}

// setAccountStatus lets the user force an account's status (e.g. reset a
// rate-limited/errored account back to active).
func (c *Control) setAccountStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	status := store.AccountStatus(strings.TrimSpace(body.Status))
	if status == "" {
		status = store.AccountActive
	}
	switch status {
	case store.AccountActive, store.AccountDisabled, store.AccountRateLimited, store.AccountRefreshFailed:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid status"})
		return
	}
	if err := c.store.SetAccountStatus(id, status, ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	acc, _ := c.store.GetAccount(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "account": acc})
}

// importAccounts accepts a batch of accounts as JSON. The body may be a bare
// array of entries or an object of the form {"accounts": [...]}.
func (c *Control) importAccounts(w http.ResponseWriter, r *http.Request) {
	raw, ok := readImportBody(w, r)
	if !ok {
		return
	}
	result, err := c.mgr.CommitImport(r.Context(), raw, importSHA(raw), false)
	if err != nil {
		writeImportError(w, err)
		return
	}
	errors := []string{}
	for _, row := range result.Rows {
		if row.ErrorMessage != "" {
			errors = append(errors, fmt.Sprintf("row %d: %s", row.Index, row.ErrorMessage))
		}
	}
	writeJSON(w, http.StatusOK, account.ImportResult{
		Imported: result.Imported, Updated: result.Updated, Skipped: result.Skipped + result.Failed, Errors: errors,
	})
}

func (c *Control) previewImportAccounts(w http.ResponseWriter, r *http.Request) {
	raw, ok := readImportBody(w, r)
	if !ok {
		return
	}
	preview, err := c.mgr.PreviewImport(r.Context(), raw)
	if err != nil {
		writeImportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (c *Control) commitImportAccounts(w http.ResponseWriter, r *http.Request) {
	raw, ok := readImportBody(w, r)
	if !ok {
		return
	}
	expected := r.Header.Get("X-Import-Preview-SHA256")
	validate, _ := strconv.ParseBool(r.Header.Get("X-Validate-After-Import"))
	result, err := c.mgr.CommitImport(r.Context(), raw, expected, validate)
	if err != nil {
		writeImportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func readImportBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	limited := http.MaxBytesReader(w, r.Body, account.MaxImportBytes)
	raw, err := io.ReadAll(limited)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeControlError(w, http.StatusRequestEntityTooLarge, "import_too_large", "import exceeds 10 MiB", false, nil)
		} else {
			writeControlError(w, http.StatusBadRequest, "import_read_failed", err.Error(), false, nil)
		}
		return nil, false
	}
	return raw, true
}

func importSHA(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func writeImportError(w http.ResponseWriter, err error) {
	var serviceErr *account.ImportServiceError
	if !errors.As(err, &serviceErr) {
		writeControlError(w, http.StatusInternalServerError, "import_failed", err.Error(), false, nil)
		return
	}
	status := http.StatusBadRequest
	switch serviceErr.Code {
	case "import_preview_mismatch", "import_duplicate_conflict":
		status = http.StatusConflict
	case "import_commit_failed":
		status = http.StatusInternalServerError
	}
	writeControlError(w, status, serviceErr.Code, serviceErr.Message, serviceErr.Retryable, serviceErr.Details)
}

func (c *Control) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := c.store.DeleteAccount(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (c *Control) refreshAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	acc, err := c.store.GetAccount(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "account not found"})
		return
	}
	var proxy *store.Proxy
	if acc.ProxyID != nil {
		proxy, _ = c.store.GetProxy(*acc.ProxyID)
	}
	client := newAuthHTTPClient(proxy)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := c.mgr.Refresh(ctx, client, id); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (c *Control) bindProxy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		ProxyID *int64 `json:"proxy_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := c.store.SetAccountProxy(id, body.ProxyID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- oauth ---

func (c *Control) oauthStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProxyID *int64 `json:"proxy_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	flow, err := c.oauth.Start(body.ProxyID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auth_url": flow.AuthURL, "state": flow.State})
}

func (c *Control) oauthPoll(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	done, errMsg, acc, found := c.oauth.Poll(state)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown state"})
		return
	}
	resp := map[string]any{"done": done, "error": errMsg}
	if acc != nil {
		resp["account"] = acc
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- proxies ---

func (c *Control) listProxies(w http.ResponseWriter, r *http.Request) {
	proxies, err := c.store.ListProxies()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proxies": proxies})
}

func (c *Control) createProxy(w http.ResponseWriter, r *http.Request) {
	var p store.Proxy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if p.Type == "" {
		p.Type = store.ProxyHTTP
	}
	created, err := c.store.CreateProxy(&p)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (c *Control) updateProxy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name          *string          `json:"name"`
		Type          *store.ProxyType `json:"type"`
		Host          *string          `json:"host"`
		Port          *int             `json:"port"`
		Username      *string          `json:"username"`
		Password      *string          `json:"password"`
		ClearPassword bool             `json:"clear_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	updated, err := c.store.UpdateProxyPatch(id, store.ProxyPatch{
		Name: body.Name, Type: body.Type, Host: body.Host, Port: body.Port, Username: body.Username,
		Password: body.Password, ClearPassword: body.ClearPassword,
	})
	if err == store.ErrNotFound {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "proxy not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (c *Control) deleteProxy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := c.store.DeleteProxy(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (c *Control) testProxy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	proxy, err := c.store.GetProxy(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "proxy not found"})
		return
	}
	writeJSON(w, http.StatusOK, testProxyLatency(r.Context(), proxy))
}

// listModels returns the model options (incl. reasoning-effort suffixed
// variants) offered in the connectivity-test / default-model pickers.
func (c *Control) listModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"models":              openai.ModelOptions(),
		"default_model":       openai.DefaultGatewayModel,
		"default_test_model":  openai.DefaultTestModel,
		"codex_default_model": openai.DefaultCodexModel,
	})
}

func (c *Control) pricing(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"price_version": openai.PriceVersion,
		"source_url":    openai.PriceSourceURL,
		"tier":          "standard",
		"models":        openai.PublishedModelPrices(),
	})
}

// --- logs / stats ---

func (c *Control) logs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 50
	}
	logs, err := c.store.RecentLogs(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (c *Control) stats(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days == 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	summary, err := c.store.Summary(since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	daily, _ := c.store.Daily(days)
	byModel, _ := c.store.ByModel(since)
	failures, _ := c.store.FailureBreakdown(since)
	health, _ := c.store.LogHealth()
	cfg := c.settings.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": summary, "daily": daily, "by_model": byModel, "failure_breakdown": failures,
		"retention": map[string]any{
			"days": cfg.LogRetentionDays, "max_rows": cfg.MaxLogRows,
			"retained_rows": health.Rows, "oldest_at": health.OldestAt, "newest_at": health.NewestAt,
		},
	})
}

func (c *Control) exportLogs(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		writeControlError(w, http.StatusBadRequest, "invalid_export_format", "format must be json or csv", false, nil)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	var since time.Time
	if days > 0 {
		since = time.Now().AddDate(0, 0, -days)
	}
	logs, err := c.store.LogsForExport(since)
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, "log_export_failed", err.Error(), true, nil)
		return
	}
	for _, entry := range logs {
		entry.AccountEmail = redact.MaskEmail(entry.AccountEmail)
		entry.Error = redact.Sanitize(entry.Error)
	}
	stamp := time.Now().Format("20060102-150405")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="amber-logs-%s.%s"`, stamp, format))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"exported_at": time.Now().UTC(), "logs": logs})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"id", "created_at", "request_id", "account_email", "requested_model", "resolved_model", "status_code", "error_kind", "attempt_count", "terminal_event", "prompt_tokens", "cached_tokens", "completion_tokens", "reasoning_tokens", "total_tokens", "estimated", "latency_ms", "stream", "error"})
	for _, entry := range logs {
		_ = writer.Write([]string{
			strconv.FormatInt(entry.ID, 10), entry.CreatedAt.UTC().Format(time.RFC3339), entry.RequestID, entry.AccountEmail,
			entry.RequestedModel, entry.ResolvedModel, strconv.Itoa(entry.StatusCode), entry.ErrorKind,
			strconv.Itoa(entry.AttemptCount), entry.TerminalEvent, strconv.Itoa(entry.PromptTokens),
			strconv.Itoa(entry.CachedTokens), strconv.Itoa(entry.CompletionTokens), strconv.Itoa(entry.ReasoningTokens),
			strconv.Itoa(entry.TotalTokens), strconv.FormatBool(entry.Estimated), strconv.FormatInt(entry.LatencyMS, 10),
			strconv.FormatBool(entry.Stream), entry.Error,
		})
	}
	writer.Flush()
}

func (c *Control) clearLogs(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Confirm-Clear") != "clear-request-logs" {
		writeControlError(w, http.StatusPreconditionFailed, "log_clear_confirmation_required", "log clear confirmation is required", false, nil)
		return
	}
	deleted, err := c.store.ClearLogs()
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, "log_clear_failed", err.Error(), true, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

// --- helpers ---

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeControlError(w http.ResponseWriter, status int, code, message string, retryable bool, details map[string]any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"code": code, "message": message, "retryable": retryable, "details": details,
	}})
}
