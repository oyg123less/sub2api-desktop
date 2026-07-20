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

	"sub2api-desktop/core/internal/codexcfg"
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
