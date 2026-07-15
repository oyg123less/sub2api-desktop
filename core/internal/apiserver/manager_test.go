package apiserver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"sub2api-desktop/core/internal/store"
)

func TestManagerRestartSwitchesPorts(t *testing.T) {
	oldPort := freeTCPPort(t)
	newPort := freeTCPPort(t)
	settings := store.Settings{ListenPort: oldPort}
	manager := NewManager(&Handler{}, func() store.Settings { return settings })
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	assertHealthy(t, oldPort)

	settings.ListenPort = newPort
	if err := manager.Restart(); err != nil {
		t.Fatal(err)
	}
	if manager.Port() != newPort {
		t.Fatalf("active port = %d, want %d", manager.Port(), newPort)
	}
	assertHealthy(t, newPort)
	assertNotListening(t, oldPort)
}

func TestManagerRestartRollsBackWhenPortIsOccupied(t *testing.T) {
	oldPort := freeTCPPort(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	newPort := occupied.Addr().(*net.TCPAddr).Port
	settings := store.Settings{ListenPort: oldPort}
	manager := NewManager(&Handler{}, func() store.Settings { return settings })
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()

	settings.ListenPort = newPort
	if err := manager.Restart(); err == nil {
		t.Fatal("restart unexpectedly succeeded on an occupied port")
	}
	if manager.Port() != oldPort || !manager.Running() {
		t.Fatalf("manager did not restore old listener: running=%t port=%d", manager.Running(), manager.Port())
	}
	assertHealthy(t, oldPort)
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func assertHealthy(t *testing.T, port int) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", response.StatusCode)
	}
}

func assertNotListening(t *testing.T, port int) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatalf("old port %d is still listening", port)
	}
}
