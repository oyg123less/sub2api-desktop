package apiserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"sub2api-desktop/core/internal/store"
)

// Manager owns the lifecycle of the local /v1 API server and implements the
// control.ServerController interface.
type Manager struct {
	handler  *Handler
	settings func() store.Settings

	mu      sync.Mutex
	server  *http.Server
	running bool
	port    int
}

// NewManager creates an API server manager.
func NewManager(h *Handler, settings func() store.Settings) *Manager {
	return &Manager{handler: h, settings: settings, port: settings().ListenPort}
}

// Running reports whether the API server is up.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Port returns the active listen port while running, otherwise the port
// currently configured in settings.
func (m *Manager) Port() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return m.port
	}
	return m.settings().ListenPort
}

// Start binds and serves the API. It is safe to call when already running.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}
	cfg := m.settings()
	host := "127.0.0.1"
	if cfg.AllowLAN {
		host = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", host, cfg.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("端口 %d 被占用或无法绑定: %w", cfg.ListenPort, err)
	}

	mux := http.NewServeMux()
	m.handler.Mount(mux)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 15 * time.Second}
	m.server = srv
	m.running = true
	m.port = cfg.ListenPort
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Serve exited unexpectedly; mark as stopped.
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
		}
	}()
	return nil
}

// Stop gracefully shuts down the API server.
func (m *Manager) Stop() error {
	m.mu.Lock()
	srv := m.server
	m.running = false
	m.server = nil
	m.mu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
