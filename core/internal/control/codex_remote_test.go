package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sub2api-desktop/core/internal/codexcfg"
	"sub2api-desktop/core/internal/codexremote"
	"sub2api-desktop/core/internal/store"
)

type remoteControllerStub struct {
	probe      codexremote.Probe
	target     codexremote.TargetStatus
	probeErr   error
	injectErr  error
	tunnelErr  error
	lastInject codexremote.InjectRequest
}

func (s *remoteControllerStub) Probe(context.Context, codexremote.ProbeRequest) (codexremote.Probe, error) {
	return s.probe, s.probeErr
}
func (s *remoteControllerStub) Inject(_ context.Context, request codexremote.InjectRequest) (codexremote.TargetStatus, error) {
	s.lastInject = request
	return s.target, s.injectErr
}
func (s *remoteControllerStub) Targets() ([]codexremote.TargetStatus, error) {
	return []codexremote.TargetStatus{s.target}, nil
}
func (s *remoteControllerStub) SetTunnel(context.Context, int64, bool) (codexremote.TargetStatus, error) {
	return s.target, s.tunnelErr
}

func TestCodexRemoteDirectInjectValidationAndRendering(t *testing.T) {
	settings := &settingsPatchAccess{value: store.Settings{LocalAPIKey: "fixture-local-key", CodexModel: "gpt-5.6-sol"}}
	t.Run("valid", func(t *testing.T) {
		remote := &remoteControllerStub{target: codexremote.TargetStatus{Mode: codexremote.ModeDirect}}
		control := &Control{remoteCodex: remote, settings: settings}
		request := httptest.NewRequest(http.MethodPost, "/control/codex/remote/inject", strings.NewReader(
			`{"host":"example.test","user":"deploy","password":"fixture-password","mode":"direct","base_url":"https://api.example.test/v1/","api_key":"fixture-direct-key","model":"gpt-5.6-sol"}`))
		response := httptest.NewRecorder()
		control.codexRemoteInject(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
		}
		if remote.lastInject.BaseURL != "https://api.example.test/v1" || remote.lastInject.Mode != codexremote.ModeDirect {
			t.Fatalf("normalized request = %#v", remote.lastInject)
		}
		if remote.lastInject.Config != codexcfg.RenderConfig(remote.lastInject.BaseURL, "gpt-5.6-sol") ||
			remote.lastInject.Auth != codexcfg.RenderAuth("fixture-direct-key") {
			t.Fatal("direct request did not render the supplied Base URL and API key")
		}
		if strings.Contains(response.Body.String(), "fixture-direct-key") {
			t.Fatal("direct inject response exposed the API key")
		}
	})

	t.Run("saved target reuses key", func(t *testing.T) {
		remote := &remoteControllerStub{target: codexremote.TargetStatus{ID: 7, Mode: codexremote.ModeDirect}}
		control := &Control{remoteCodex: remote, settings: settings}
		request := httptest.NewRequest(http.MethodPost, "/control/codex/remote/inject", strings.NewReader(
			`{"id":7,"host":"example.test","user":"deploy","mode":"direct","base_url":"https://api.example.test/v1","model":"gpt-5.6-sol"}`))
		response := httptest.NewRecorder()
		control.codexRemoteInject(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
		}
		if remote.lastInject.APIKey != "" || remote.lastInject.Auth != "" || remote.lastInject.Config == "" {
			t.Fatal("saved direct reinject request was not deferred to the manager")
		}
	})

	for _, test := range []struct {
		name string
		body string
	}{
		{name: "invalid scheme", body: `{"mode":"direct","base_url":"ftp://api.example.test/v1","api_key":"key","model":"gpt-5.6-sol"}`},
		{name: "missing host", body: `{"mode":"direct","base_url":"https:///v1","api_key":"key","model":"gpt-5.6-sol"}`},
		{name: "userinfo", body: `{"mode":"direct","base_url":"https://key@api.example.test/v1","api_key":"key","model":"gpt-5.6-sol"}`},
		{name: "missing api key", body: `{"mode":"direct","base_url":"https://api.example.test/v1","model":"gpt-5.6-sol"}`},
		{name: "unsaved target missing api key", body: `{"id":-1,"mode":"direct","base_url":"https://api.example.test/v1","model":"gpt-5.6-sol"}`},
		{name: "invalid mode", body: `{"mode":"other","model":"gpt-5.6-sol"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			remote := &remoteControllerStub{}
			control := &Control{remoteCodex: remote, settings: settings}
			response := httptest.NewRecorder()
			control.codexRemoteInject(response, httptest.NewRequest(http.MethodPost, "/control/codex/remote/inject", strings.NewReader(test.body)))
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"invalid_request"`) {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
			if remote.lastInject.Config != "" || remote.lastInject.Auth != "" {
				t.Fatal("invalid direct request reached the manager")
			}
		})
	}
}

func TestCodexRemoteDirectTunnelReturnsBadRequest(t *testing.T) {
	remote := &remoteControllerStub{tunnelErr: &codexremote.Error{Code: "tunnel_not_applicable"}}
	control := &Control{remoteCodex: remote}
	request := httptest.NewRequest(http.MethodPost, "/control/codex/remote/1/tunnel", strings.NewReader(`{"enabled":true}`))
	request.SetPathValue("id", "1")
	response := httptest.NewRecorder()
	control.codexRemoteTunnel(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"tunnel_not_applicable"`) {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
}
func (s *remoteControllerStub) Restore(context.Context, int64) (codexremote.TargetStatus, error) {
	return s.target, nil
}
func (s *remoteControllerStub) Delete(int64) error { return nil }

func TestCodexRemoteHandlersDoNotExposeCredentials(t *testing.T) {
	remote := &remoteControllerStub{target: codexremote.TargetStatus{
		ID: 1, Host: "example.test", User: "deploy", TunnelStatus: codexremote.StatusConnected,
	}}
	settings := &settingsPatchAccess{value: store.Settings{LocalAPIKey: "fixture-local-key", CodexModel: "gpt-5.6-sol"}}
	control := &Control{remoteCodex: remote, settings: settings}
	request := httptest.NewRequest(http.MethodPost, "/control/codex/remote/inject", strings.NewReader(
		`{"host":"example.test","port":22,"user":"deploy","password":"fixture-password","remote_port":8080,"model":"gpt-5.6-sol","accept_host_key":true}`))
	response := httptest.NewRecorder()

	control.codexRemoteInject(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if remote.lastInject.Config == "" || remote.lastInject.Auth == "" {
		t.Fatal("remote inject handler did not render Codex files")
	}
	body := response.Body.String()
	if strings.Contains(body, "fixture-password") || strings.Contains(body, "fixture-local-key") {
		t.Fatal("remote inject response exposed a credential")
	}
}

func TestCodexRemoteHandlerErrorCodes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		probe       bool
		wantStatus  int
		wantCode    string
		fingerprint string
	}{
		{name: "auth failure", err: &codexremote.Error{Code: "auth_failed"}, probe: true, wantStatus: http.StatusUnauthorized, wantCode: "auth_failed"},
		{name: "host key mismatch", err: &codexremote.Error{Code: "host_key_mismatch", Fingerprint: "SHA256:test"}, wantStatus: http.StatusConflict, wantCode: "host_key_mismatch", fingerprint: "SHA256:test"},
		{name: "inject failure", err: &codexremote.Error{Code: "remote_command_failed"}, wantStatus: http.StatusBadGateway, wantCode: "remote_command_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote := &remoteControllerStub{}
			control := &Control{
				remoteCodex: remote,
				settings:    &settingsPatchAccess{value: store.Settings{LocalAPIKey: "fixture-key", CodexModel: "gpt-5.6-sol"}},
			}
			response := httptest.NewRecorder()
			if tt.probe {
				remote.probeErr = tt.err
				request := httptest.NewRequest(http.MethodPost, "/control/codex/remote/test", strings.NewReader(
					`{"host":"example.test","user":"deploy","password":"fixture-password"}`))
				control.codexRemoteTest(response, request)
			} else {
				remote.injectErr = tt.err
				request := httptest.NewRequest(http.MethodPost, "/control/codex/remote/inject", strings.NewReader(
					`{"host":"example.test","user":"deploy","password":"fixture-password","model":"gpt-5.6-sol"}`))
				control.codexRemoteInject(response, request)
			}
			if response.Code != tt.wantStatus || !strings.Contains(response.Body.String(), `"code":"`+tt.wantCode+`"`) {
				t.Fatalf("status = %d, expected code %s", response.Code, tt.wantCode)
			}
			if tt.fingerprint != "" && !strings.Contains(response.Body.String(), tt.fingerprint) {
				t.Fatal("host key mismatch response omitted the fingerprint")
			}
		})
	}

	for _, handler := range []func(*Control, http.ResponseWriter, *http.Request){
		func(c *Control, w http.ResponseWriter, r *http.Request) { c.codexRemoteTest(w, r) },
		func(c *Control, w http.ResponseWriter, r *http.Request) { c.codexRemoteInject(w, r) },
		func(c *Control, w http.ResponseWriter, r *http.Request) { c.codexRemoteTargets(w, r) },
		func(c *Control, w http.ResponseWriter, r *http.Request) { c.codexRemoteTunnel(w, r) },
		func(c *Control, w http.ResponseWriter, r *http.Request) { c.codexRemoteRestore(w, r) },
		func(c *Control, w http.ResponseWriter, r *http.Request) { c.codexRemoteDelete(w, r) },
	} {
		response := httptest.NewRecorder()
		handler(&Control{}, response, httptest.NewRequest(http.MethodPost, "/control/codex/remote", nil))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"remote_unavailable"`) {
			t.Fatalf("nil manager response = %d %s", response.Code, response.Body.String())
		}
	}
}
