package control

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

// oauthState tracks a single in-progress login.
type oauthState struct {
	flow    *account.LoginFlow
	done    bool
	err     string
	account *store.Account
}

// oauthCoordinator manages the local callback listener and pending logins.
type oauthCoordinator struct {
	mgr      *account.Manager
	store    *store.Store
	settings func() store.Settings

	mu       sync.Mutex
	pending  map[string]*oauthState
	server   *http.Server
	listener net.Listener
	port     int
}

func newOAuthCoordinator(mgr *account.Manager, s *store.Store, settings func() store.Settings) *oauthCoordinator {
	return &oauthCoordinator{mgr: mgr, store: s, settings: settings, pending: map[string]*oauthState{}, port: 1455}
}

// Start begins a login flow: prepares PKCE, registers state, ensures the
// callback listener is running, and returns the authorization URL.
func (c *oauthCoordinator) Start(proxyID *int64) (*account.LoginFlow, error) {
	if err := c.ensureListener(); err != nil {
		return nil, err
	}
	flow, err := account.NewLoginFlow("", proxyID)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.pending[flow.State] = &oauthState{flow: flow}
	c.mu.Unlock()
	return flow, nil
}

// Poll returns the status of a login by state.
func (c *oauthCoordinator) Poll(state string) (done bool, errMsg string, acc *store.Account, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.pending[state]
	if !ok {
		return false, "", nil, false
	}
	return st.done, st.err, st.account, true
}

func (c *oauthCoordinator) ensureListener() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.server != nil {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", c.port))
	if err != nil {
		return fmt.Errorf("回调端口 %d 被占用，请关闭占用程序后重试: %w", c.port, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", c.handleCallback)
	c.server = &http.Server{Handler: mux}
	c.listener = ln
	go func() { _ = c.server.Serve(ln) }()
	return nil
}

func (c *oauthCoordinator) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	code := q.Get("code")
	oauthErr := q.Get("error")

	c.mu.Lock()
	st, ok := c.pending[state]
	c.mu.Unlock()
	if !ok {
		writeCallbackHTML(w, false, "无效或已过期的登录会话，请返回应用重试。")
		return
	}

	if oauthErr != "" {
		c.finish(state, "", nil, fmt.Sprintf("授权被拒绝: %s", oauthErr))
		writeCallbackHTML(w, false, "授权被拒绝，请返回应用重试。")
		return
	}
	if code == "" {
		c.finish(state, "", nil, "回调缺少授权码")
		writeCallbackHTML(w, false, "回调缺少授权码，请返回应用重试。")
		return
	}

	// Build an auth client honoring the flow's proxy (if any).
	var proxy *store.Proxy
	if st.flow.ProxyID != nil {
		if p, err := c.store.GetProxy(*st.flow.ProxyID); err == nil {
			proxy = p
		}
	}
	client := newAuthHTTPClient(proxy)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	acc, err := c.mgr.Exchange(ctx, client, st.flow, code)
	if err != nil {
		c.finish(state, "", nil, "令牌交换失败: "+err.Error())
		writeCallbackHTML(w, false, "登录失败："+err.Error())
		return
	}
	c.finish(state, "", acc, "")
	writeCallbackHTML(w, true, "登录成功！可以关闭此页面并返回应用。")
}

func (c *oauthCoordinator) finish(state, _ string, acc *store.Account, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if st, ok := c.pending[state]; ok {
		st.done = true
		st.err = errMsg
		st.account = acc
	}
}

func newAuthHTTPClient(proxy *store.Proxy) *http.Client {
	client, err := gateway.NewAuthClient(proxy)
	if err != nil || client == nil {
		return &http.Client{Timeout: 60 * time.Second}
	}
	return client
}

func writeCallbackHTML(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	color := "#16a34a"
	title := "登录成功"
	if !ok {
		color = "#dc2626"
		title = "登录失败"
	}
	fmt.Fprintf(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sub2API Desktop</title></head>
<body style="margin:0;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;background:#0f172a;color:#e2e8f0;display:flex;align-items:center;justify-content:center;height:100vh">
<div style="text-align:center;max-width:420px;padding:32px;background:#1e293b;border-radius:16px;box-shadow:0 10px 40px rgba(0,0,0,.4)">
<div style="font-size:48px;margin-bottom:12px">%s</div>
<h1 style="color:%s;font-size:20px;margin:0 0 8px">%s</h1>
<p style="color:#94a3b8;font-size:14px;line-height:1.6">%s</p>
</div></body></html>`, iconFor(ok), color, title, msg)
}

func iconFor(ok bool) string {
	if ok {
		return "&#10004;"
	}
	return "&#10008;"
}
