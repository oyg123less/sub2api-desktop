package codexremote

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

type scriptedConnection struct {
	run       func(string, []byte) ([]byte, error)
	listen    func(string, string) (net.Listener, error)
	closeCall int
}

func (c *scriptedConnection) Run(_ context.Context, command string, input []byte) ([]byte, error) {
	if c.run == nil {
		return nil, nil
	}
	return c.run(command, input)
}

func (c *scriptedConnection) Listen(network, address string) (net.Listener, error) {
	if c.listen != nil {
		return c.listen(network, address)
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

func (c *scriptedConnection) SendRequest(string, bool, []byte) (bool, []byte, error) {
	return true, nil, nil
}

func (c *scriptedConnection) Close() error {
	c.closeCall++
	return nil
}

func TestSSHRemoteProbe(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		runErr   error
		wantOS   string
		wantCode string
	}{
		{name: "linux", output: "Linux\n/home/deploy\n", wantOS: "Linux"},
		{name: "macos CRLF", output: "Darwin\r\n/Users/deploy\r\n", wantOS: "Darwin"},
		{name: "windows", output: "Windows_NT\nC:/Users/deploy\n", wantCode: "unsupported_os"},
		{name: "missing home", output: "Linux\n", wantCode: "remote_command_failed"},
		{name: "relative home", output: "Linux\nhome/deploy\n", wantCode: "remote_command_failed"},
		{name: "command error", runErr: errors.New("command failed"), wantCode: "remote_command_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connection := &scriptedConnection{run: func(string, []byte) ([]byte, error) {
				return []byte(tt.output), tt.runErr
			}}
			probe, err := (&sshRemoteOperations{connection: connection}).Probe(t.Context())
			if tt.wantCode != "" {
				assertRemoteErrorCode(t, err, tt.wantCode)
				return
			}
			if err != nil || probe.OS != tt.wantOS || !strings.HasSuffix(probe.CodexDir, "/.codex") {
				t.Fatalf("probe = %#v, err=%v", probe, err)
			}
		})
	}
}

func TestSSHRemoteFileFailures(t *testing.T) {
	validConfig := "model = \"gpt-5.6\"\nmodel_provider = \"sub2api\"\n[model_providers.sub2api]\nname = \"Sub2API\"\nbase_url = \"http://127.0.0.1:8080/v1\"\nwire_api = \"responses\"\nrequires_openai_auth = true\n"
	validAuth := `{"OPENAI_API_KEY":"fixture"}`

	t.Run("validation", func(t *testing.T) {
		operations := &sshRemoteOperations{connection: &scriptedConnection{}}
		if err := operations.Inject(t.Context(), "/home/test/.codex", "invalid", validAuth); err == nil {
			t.Fatal("invalid config was accepted")
		}
		if err := operations.Inject(t.Context(), "/home/test/.codex", validConfig, "invalid"); err == nil {
			t.Fatal("invalid auth was accepted")
		}
	})

	for _, failAt := range []int{1, 2, 3, 4} {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			calls := 0
			connection := &scriptedConnection{run: func(string, []byte) ([]byte, error) {
				calls++
				if calls == failAt {
					return nil, errors.New("remote failure")
				}
				return nil, nil
			}}
			err := (&sshRemoteOperations{connection: connection}).Inject(t.Context(), "/home/test/.codex", validConfig, validAuth)
			assertRemoteErrorCode(t, err, "remote_command_failed")
		})
	}

	connection := &scriptedConnection{run: func(string, []byte) ([]byte, error) {
		return nil, errors.New("restore failure")
	}}
	assertRemoteErrorCode(t, (&sshRemoteOperations{connection: connection}).Restore(t.Context(), "/home/test/.codex"), "remote_command_failed")
}

func TestNormalizeProbeRequest(t *testing.T) {
	normalized, err := normalizeProbeRequest(ProbeRequest{Host: " deploy@example.test ", Password: "secret"})
	if err != nil || normalized.Host != "example.test" || normalized.User != "deploy" || normalized.Port != 22 {
		t.Fatalf("normalized = %#v, err=%v", normalized, err)
	}
	for _, request := range []ProbeRequest{
		{Host: "", User: "deploy", Password: "secret"},
		{Host: "example.test", User: "", Password: "secret"},
		{Host: "example.test", User: "deploy", Password: ""},
		{Host: "example.test", User: "deploy", Password: "secret", Port: 65536},
	} {
		_, err := normalizeProbeRequest(request)
		assertRemoteErrorCode(t, err, "invalid_target")
	}
}

func TestRemoteErrorMessages(t *testing.T) {
	for _, code := range []string{
		"auth_failed", "host_key_unknown", "host_key_mismatch", "unsupported_os", "invalid_target",
		"target_not_found", "tunnel_failed", "remote_command_failed", "connection_failed",
	} {
		err := codedError(code, errors.New("cause"))
		if err.Error() == "" || !errors.Is(err, err.(*Error).cause) {
			t.Fatalf("invalid error behavior for %q", code)
		}
	}
}

func assertRemoteErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var remoteError *Error
	if !errors.As(err, &remoteError) || remoteError.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
}
