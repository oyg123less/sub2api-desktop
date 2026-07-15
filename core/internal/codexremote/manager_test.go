package codexremote

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sub2api-desktop/core/internal/codexcfg"
)

type virtualRemote struct {
	mu       sync.Mutex
	files    map[string]string
	staged   []string
	dir      string
	commands []string
}

func (v *virtualRemote) Run(_ context.Context, command string, input []byte) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.commands = append(v.commands, command)
	configPath, authPath := v.dir+"/config.toml", v.dir+"/auth.json"
	switch {
	case strings.Contains(command, "mkdir -p"):
		for _, filename := range []string{configPath, authPath} {
			if _, backed := v.files[filename+".sub2api-bak"]; backed {
				continue
			}
			if _, absent := v.files[filename+".sub2api-absent"]; absent {
				continue
			}
			if content, exists := v.files[filename]; exists {
				v.files[filename+".sub2api-bak"] = content
			} else {
				v.files[filename+".sub2api-absent"] = ""
			}
		}
	case strings.Contains(command, "cat >"):
		v.staged = append(v.staged, string(input))
	case strings.Contains(command, "mv -f") && len(v.staged) == 2:
		v.files[configPath], v.files[authPath] = v.staged[0], v.staged[1]
		v.staged = nil
	case strings.Contains(command, "sub2api-absent") && strings.Contains(command, "elif"):
		for _, filename := range []string{configPath, authPath} {
			if original, exists := v.files[filename+".sub2api-bak"]; exists {
				v.files[filename] = original
				delete(v.files, filename+".sub2api-bak")
			} else if _, exists := v.files[filename+".sub2api-absent"]; exists {
				delete(v.files, filename)
				delete(v.files, filename+".sub2api-absent")
			}
		}
	}
	return nil, nil
}

func (v *virtualRemote) Listen(string, string) (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}
func (v *virtualRemote) SendRequest(string, bool, []byte) (bool, []byte, error) {
	return true, nil, nil
}
func (v *virtualRemote) Close() error { return nil }

func TestRemoteFileBackupReapplyAndRestore(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "existing"}[existing], func(t *testing.T) {
			remote := &virtualRemote{dir: "/home/test/.codex", files: map[string]string{}}
			if existing {
				remote.files[remote.dir+"/config.toml"] = "original-config"
				remote.files[remote.dir+"/auth.json"] = "original-auth"
			}
			operations := &sshRemoteOperations{connection: remote}
			config := codexcfg.RenderConfig("http://127.0.0.1:8080/v1", "gpt-5.6-sol")
			auth := codexcfg.RenderAuth("fixture-key")
			if err := operations.Inject(t.Context(), remote.dir, config, auth); err != nil {
				t.Fatal(err)
			}
			if err := operations.Inject(t.Context(), remote.dir, strings.Replace(config, "8080", "9090", 1), auth); err != nil {
				t.Fatal(err)
			}
			if err := operations.Restore(t.Context(), remote.dir); err != nil {
				t.Fatal(err)
			}
			_, configExists := remote.files[remote.dir+"/config.toml"]
			if existing && (!configExists || remote.files[remote.dir+"/config.toml"] != "original-config") {
				t.Fatal("existing remote config was not restored")
			}
			if !existing && configExists {
				t.Fatal("Amber-created remote config was not removed")
			}
		})
	}
}

type tunnelTestConnection struct {
	listener net.Listener
	addr     chan string
}

func (c *tunnelTestConnection) Run(context.Context, string, []byte) ([]byte, error) { return nil, nil }
func (c *tunnelTestConnection) Listen(string, string) (net.Listener, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		c.listener = listener
		c.addr <- listener.Addr().String()
	}
	return listener, err
}
func (c *tunnelTestConnection) SendRequest(string, bool, []byte) (bool, []byte, error) {
	return true, nil, nil
}
func (c *tunnelTestConnection) Close() error { return nil }

type stubRemoteOperations struct{}

func (stubRemoteOperations) Probe(context.Context) (Probe, error) {
	return Probe{OS: "Linux", Home: "/home/test", CodexDir: "/home/test/.codex"}, nil
}
func (stubRemoteOperations) Inject(context.Context, string, string, string) error { return nil }
func (stubRemoteOperations) Restore(context.Context, string) error                { return nil }

func TestManagerTunnelEnableDisableEnable(t *testing.T) {
	gatewayListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gatewayListener.Close()
	go func() {
		for {
			connection, err := gatewayListener.Accept()
			if err != nil {
				return
			}
			go func() { defer connection.Close(); _, _ = io.Copy(connection, connection) }()
		}
	}()

	manager, err := NewManager(nil, filepath.Join(t.TempDir(), "known_hosts"), gatewayListener.Addr().String, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	addresses := make(chan string, 4)
	manager.dial = func(context.Context, ProbeRequest, bool, bool) (*dialResult, error) {
		return &dialResult{connection: &tunnelTestConnection{addr: addresses}, fingerprint: "SHA256:test", known: true}, nil
	}
	manager.remote = func(remoteConnection) remoteOperations { return stubRemoteOperations{} }
	request := InjectRequest{
		Host: "example.test", Port: 22, User: "deploy", Password: "fixture-password", RemotePort: 8080,
		Model: "gpt-5.6-sol", Config: codexcfg.RenderConfig("http://127.0.0.1:8080/v1", "gpt-5.6-sol"),
		Auth: codexcfg.RenderAuth("fixture-key"), AcceptHostKey: true,
	}
	status, err := manager.Inject(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	firstAddress := <-addresses
	assertTunnelEcho(t, firstAddress)
	status, err = manager.SetTunnel(t.Context(), status.ID, false)
	if err != nil || status.TunnelStatus != StatusDisabled {
		t.Fatalf("disable status = %q err=%v", status.TunnelStatus, err)
	}
	status, err = manager.SetTunnel(t.Context(), status.ID, true)
	if err != nil || status.TunnelStatus != StatusConnected {
		t.Fatalf("enable status = %q err=%v", status.TunnelStatus, err)
	}
	secondAddress := <-addresses
	assertTunnelEcho(t, secondAddress)
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "fixture-password") || strings.Contains(string(encoded), "fixture-key") {
		t.Fatal("target status serialized a credential")
	}
}

func assertTunnelEcho(t *testing.T, address string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := connection.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(connection, buffer); err != nil || string(buffer) != "ping" {
		t.Fatalf("tunnel echo failed: %q err=%v", string(buffer), err)
	}
}
