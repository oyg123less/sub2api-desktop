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

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	server      *http.Server
	listener    net.Listener
	serveDone   chan struct{}
	running     bool
	port        int
	allowLAN    bool
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

func (m *Manager) InstanceID() string { return m.handler.InstanceID() }

// Start binds and serves the API. It is safe to call when already running.
func (m *Manager) Start() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	return m.startWithSettings(m.settings())
}

func (m *Manager) startWithSettings(cfg store.Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}
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
	done := make(chan struct{})
	m.server = srv
	m.listener = ln
	m.serveDone = done
	m.running = true
	m.port = cfg.ListenPort
	m.allowLAN = cfg.AllowLAN
	go func() {
		defer close(done)
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
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	return m.stop()
}

func (m *Manager) stop() error {
	m.mu.Lock()
	srv := m.server
	ln := m.listener
	done := m.serveDone
	m.running = false
	m.server = nil
	m.listener = nil
	m.serveDone = nil
	m.mu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := srv.Shutdown(ctx)
	// Shutdown only closes listeners that Serve has already registered;
	// close ours explicitly and wait for the serve goroutine to exit so
	// the port is guaranteed to be released before returning.
	if ln != nil {
		if closeErr := ln.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) && err == nil {
			err = closeErr
		}
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return err
}

// Restart applies the latest listen settings. If the new address cannot be
// bound, the previous listener is restored before the error is returned.
func (m *Manager) Restart() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	m.mu.Lock()
	wasRunning := m.running
	previous := store.Settings{ListenPort: m.port, AllowLAN: m.allowLAN}
	m.mu.Unlock()
	if !wasRunning {
		return nil
	}
	if err := m.stop(); err != nil {
		rollbackErr := m.startWithSettings(previous)
		if rollbackErr != nil {
			return fmt.Errorf("stop API server for restart: %w (restore previous listener: %v)", err, rollbackErr)
		}
		return fmt.Errorf("stop API server for restart: %w", err)
	}
	if err := m.startWithSettings(m.settings()); err != nil {
		rollbackErr := m.startWithSettings(previous)
		if rollbackErr != nil {
			return fmt.Errorf("restart API server: %w (restore previous listener: %v)", err, rollbackErr)
		}
		return fmt.Errorf("restart API server: %w (previous listener restored)", err)
	}
	return nil
}
