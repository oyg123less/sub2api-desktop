package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/codexcfg"
	appcrypto "sub2api-desktop/core/internal/crypto"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

type codexGatewayStub struct {
	mu            sync.Mutex
	running       bool
	port          int
	instanceID    string
	startCalls    int
	startErr      error
	healthService string
	healthID      string
	modelsStatus  int
	models        []string
	listener      net.Listener
	server        *http.Server
}

func newCodexGatewayStub(t *testing.T, running bool) *codexGatewayStub {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stub := &codexGatewayStub{
		running:       false,
		port:          listener.Addr().(*net.TCPAddr).Port,
		instanceID:    "instance-current",
		healthService: "amber-gateway",
		healthID:      "instance-current",
		modelsStatus:  http.StatusOK,
		models:        []string{"gpt-5.6-sol"},
		listener:      listener,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		stub.mu.Lock()
		service, instanceID := stub.healthService, stub.healthID
		stub.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"service": service, "instance_id": instanceID})
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		status, models := stub.modelsStatus, append([]string(nil), stub.models...)
		stub.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer local-test-key" && status == http.StatusOK {
			status = http.StatusUnauthorized
		}
		w.WriteHeader(status)
		data := make([]map[string]string, 0, len(models))
		for _, model := range models {
			data = append(data, map[string]string{"id": model})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	stub.server = &http.Server{Handler: mux}
	if running {
		if err := stub.Start(); err != nil {
			t.Fatal(err)
		}
		stub.startCalls = 0
	}
	t.Cleanup(func() {
		_ = stub.Stop()
	})
	return stub
}

func (s *codexGatewayStub) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *codexGatewayStub) Port() int { return s.port }

func (s *codexGatewayStub) InstanceID() string { return s.instanceID }

func (s *codexGatewayStub) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls++
	if s.startErr != nil {
		return s.startErr
	}
	if s.running {
		return nil
	}
	s.running = true
	go func() { _ = s.server.Serve(s.listener) }()
	return nil
}

func (s *codexGatewayStub) Stop() error {
	s.mu.Lock()
	if !s.running {
		listener := s.listener
		s.listener = nil
		s.mu.Unlock()
		if listener != nil {
			return listener.Close()
		}
		return nil
	}
	s.running = false
	s.mu.Unlock()
	return s.server.Close()
}

func (s *codexGatewayStub) Restart() error { return nil }

func newCodexApplyTestControl(t *testing.T, server ServerController) (*Control, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	settings := &settingsPatchAccess{value: store.Settings{
		ListenPort:  server.Port(),
		LocalAPIKey: "local-test-key",
		CodexModel:  "gpt-5.6-sol",
	}}
	return &Control{settings: settings, server: server}, filepath.Join(home, ".codex")
}

func applyCodexForTest(control *Control) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/control/codex/apply", strings.NewReader(`{"model":"gpt-5.6-sol"}`))
	response := httptest.NewRecorder()
	control.codexApply(response, request)
	return response
}

func TestCodexApplyStartsGatewayBeforeWritingConfig(t *testing.T) {
	server := newCodexGatewayStub(t, false)
	control, codexDir := newCodexApplyTestControl(t, server)
	response := applyCodexForTest(control)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if server.startCalls != 1 {
		t.Fatalf("start calls=%d, want 1", server.startCalls)
	}
	manager, err := codexcfg.New(codexDir)
	if err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(fmt.Sprintf("http://127.0.0.1:%d/v1", server.Port()), "local-test-key")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Applied || status.Stale || status.Model != "gpt-5.6-sol" {
		t.Fatalf("unexpected applied status: %+v", status)
	}
}

func TestCodexApplyDoesNotWriteConfigWhenGatewayVerificationFails(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*codexGatewayStub)
		wantStatus int
		wantCode   string
	}{
		{
			name: "port conflict on start",
			configure: func(server *codexGatewayStub) {
				server.startErr = errors.New("bind: address already in use")
			},
			wantStatus: http.StatusConflict,
			wantCode:   "gateway_port_conflict",
		},
		{
			name: "non amber health endpoint",
			configure: func(server *codexGatewayStub) {
				server.healthService = "other-service"
			},
			wantStatus: http.StatusConflict,
			wantCode:   "gateway_port_conflict",
		},
		{
			name: "different amber instance",
			configure: func(server *codexGatewayStub) {
				server.healthID = "instance-other"
			},
			wantStatus: http.StatusConflict,
			wantCode:   "gateway_port_conflict",
		},
		{
			name: "api key rejected",
			configure: func(server *codexGatewayStub) {
				server.modelsStatus = http.StatusUnauthorized
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "gateway_health_check_failed",
		},
		{
			name: "selected model missing",
			configure: func(server *codexGatewayStub) {
				server.models = []string{"gpt-5.6"}
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "codex_model_unavailable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			running := test.name != "port conflict on start"
			server := newCodexGatewayStub(t, running)
			test.configure(server)
			control, codexDir := newCodexApplyTestControl(t, server)
			response := applyCodexForTest(control)
			body := response.Body.String()
			if response.Code != test.wantStatus || !strings.Contains(body, `"code":"`+test.wantCode+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, body)
			}
			if _, err := os.Stat(filepath.Join(codexDir, "config.toml")); !os.IsNotExist(err) {
				t.Fatalf("config was written after failed verification: %v", err)
			}
		})
	}
}

func TestCodexCatalogSyncLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	codexDir := filepath.Join(home, ".codex")

	cipher, err := appcrypto.LoadOrCreate(filepath.Join(home, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(home, "data.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	acc, err := st.CreateAccount(&store.Account{
		AccountType: store.AccountTypeAPIKey,
		BaseURL:     "https://api.moonshot.cn/v1",
		APIKey:      "sk-test",
		Email:       "kimi-account",
		Status:      store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	mgr := account.NewManager(st)
	engine := gateway.New(st, mgr, func() store.Settings {
		return store.Settings{
			ListenPort:  0,
			LocalAPIKey: "local-test-key",
			CodexModel:  "moonshot-v1-8k",
		}
	}, nil)

	server := newCodexGatewayStub(t, true)
	server.models = []string{"moonshot-v1-8k"}

	settings := &settingsPatchAccess{value: store.Settings{
		ListenPort:  server.Port(),
		LocalAPIKey: "local-test-key",
		CodexModel:  "moonshot-v1-8k",
	}}

	ctrl := &Control{
		store:    st,
		mgr:      mgr,
		engine:   engine,
		server:   server,
		settings: settings,
	}

	// 1. Apply Codex
	req := httptest.NewRequest(http.MethodPost, "/control/codex/apply", strings.NewReader(`{"model":"moonshot-v1-8k"}`))
	rec := httptest.NewRecorder()
	ctrl.codexApply(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply failed: status=%d body=%s", rec.Code, rec.Body.String())
	}

	catalogPath := filepath.Join(codexDir, codexcfg.CatalogFileName)
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("amber_models.json should exist after apply: %v", err)
	}
	if !strings.Contains(string(data), "moonshot-v1-8k") {
		t.Fatalf("amber_models.json missing moonshot-v1-8k: %s", string(data))
	}

	// 2. Disable account -> amber_models.json should update automatically
	statusReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/control/accounts/%d/status", acc.ID), strings.NewReader(`{"status":"disabled"}`))
	statusReq.SetPathValue("id", fmt.Sprintf("%d", acc.ID))
	statusRec := httptest.NewRecorder()
	ctrl.setAccountStatus(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("setAccountStatus failed: status=%d body=%s", statusRec.Code, statusRec.Body.String())
	}

	dataAfterDisable, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("amber_models.json should still exist: %v", err)
	}
	if strings.Contains(string(dataAfterDisable), "moonshot-v1-8k") {
		t.Fatalf("amber_models.json should no longer contain moonshot-v1-8k after account disabled: %s", string(dataAfterDisable))
	}

	// 3. Restore Codex -> amber_models.json should be removed
	restoreReq := httptest.NewRequest(http.MethodPost, "/control/codex/restore", nil)
	restoreRec := httptest.NewRecorder()
	ctrl.codexRestore(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("codexRestore failed: status=%d body=%s", restoreRec.Code, restoreRec.Body.String())
	}
	if _, err := os.Stat(catalogPath); !os.IsNotExist(err) {
		t.Fatalf("amber_models.json should be removed after restore: %v", err)
	}
}
