// Package control implements the loopback control API used by the desktop
// shell/frontend to manage accounts, proxies, settings, statistics and the
// local API server lifecycle. It listens only on 127.0.0.1 and is protected by
// a random per-session token.
package control

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/store"
)

// ServerController lets the control API report and toggle the local API server.
type ServerController interface {
	Running() bool
	Port() int
	Start() error
	Stop() error
}

// SettingsAccess abstracts reading and persisting settings, so the control API
// can update live configuration owned by main.
type SettingsAccess interface {
	Get() store.Settings
	Save(store.Settings) error
}

// Control holds control API dependencies.
type Control struct {
	store    *store.Store
	mgr      *account.Manager
	oauth    *oauthCoordinator
	settings SettingsAccess
	server   ServerController
	token    string
	version  string
}

// New builds the control API.
func New(s *store.Store, mgr *account.Manager, settings SettingsAccess, server ServerController, token, version string) *Control {
	return &Control{
		store:    s,
		mgr:      mgr,
		oauth:    newOAuthCoordinator(mgr, s, settings.Get),
		settings: settings,
		server:   server,
		token:    token,
		version:  version,
	}
}

// Mount registers control routes.
func (c *Control) Mount(mux *http.ServeMux) {
	h := c.authWrap
	mux.HandleFunc("GET /control/status", h(c.status))
	mux.HandleFunc("POST /control/server/start", h(c.serverStart))
	mux.HandleFunc("POST /control/server/stop", h(c.serverStop))

	mux.HandleFunc("GET /control/settings", h(c.getSettings))
	mux.HandleFunc("PUT /control/settings", h(c.putSettings))
	mux.HandleFunc("POST /control/settings/regenerate-key", h(c.regenKey))

	mux.HandleFunc("GET /control/accounts", h(c.listAccounts))
	mux.HandleFunc("POST /control/accounts/import", h(c.importAccounts))
	mux.HandleFunc("DELETE /control/accounts/{id}", h(c.deleteAccount))
	mux.HandleFunc("POST /control/accounts/{id}/refresh", h(c.refreshAccount))
	mux.HandleFunc("POST /control/accounts/{id}/proxy", h(c.bindProxy))

	mux.HandleFunc("POST /control/oauth/start", h(c.oauthStart))
	mux.HandleFunc("GET /control/oauth/poll", h(c.oauthPoll))

	mux.HandleFunc("GET /control/proxies", h(c.listProxies))
	mux.HandleFunc("POST /control/proxies", h(c.createProxy))
	mux.HandleFunc("DELETE /control/proxies/{id}", h(c.deleteProxy))
	mux.HandleFunc("POST /control/proxies/{id}/test", h(c.testProxy))

	mux.HandleFunc("GET /control/codex/status", h(c.codexStatus))
	mux.HandleFunc("POST /control/codex/apply", h(c.codexApply))
	mux.HandleFunc("POST /control/codex/restore", h(c.codexRestore))

	mux.HandleFunc("GET /control/logs", h(c.logs))
	mux.HandleFunc("GET /control/stats", h(c.stats))
}

// WithCORS wraps a handler with permissive CORS headers and handles preflight
// OPTIONS requests. The control API is loopback-only and token-protected, so
// allowing any origin is safe and lets the WebView (or a dev browser on a
// different port) reach it.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Control-Token, Authorization")
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
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        c.version,
		"server_running": c.server.Running(),
		"port":           c.server.Port(),
		"host":           host,
		"endpoint":       "http://127.0.0.1:" + strconv.Itoa(c.server.Port()) + "/v1",
		"local_api_key":  cfg.LocalAPIKey,
		"account_count":  len(accounts),
	})
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
	var in store.Settings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	cur := c.settings.Get()
	// Preserve the API key unless explicitly regenerated.
	if in.LocalAPIKey == "" {
		in.LocalAPIKey = cur.LocalAPIKey
	}
	if in.ListenPort == 0 {
		in.ListenPort = cur.ListenPort
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
	if err := c.settings.Save(in); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, c.settings.Get())
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

func (c *Control) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := c.store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

// importAccounts accepts a batch of accounts as JSON. The body may be a bare
// array of entries or an object of the form {"accounts": [...]}.
func (c *Control) importAccounts(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	trimmed := strings.TrimSpace(string(raw))
	var entries []account.ImportEntry
	if strings.HasPrefix(trimmed, "{") {
		var wrapper struct {
			Accounts []account.ImportEntry `json:"accounts"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrapper); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON 解析失败: " + err.Error()})
			return
		}
		entries = wrapper.Accounts
	} else {
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON 解析失败: " + err.Error()})
			return
		}
	}
	if len(entries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "未找到可导入的账号"})
		return
	}
	res := c.mgr.Import(entries)
	writeJSON(w, http.StatusOK, res)
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
	latency, err := testProxyLatency(r.Context(), proxy)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency_ms": latency.Milliseconds()})
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
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":  summary,
		"daily":    daily,
		"by_model": byModel,
	})
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
