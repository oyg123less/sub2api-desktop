package codexremote

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"sub2api-desktop/core/internal/codexcfg"
	"sub2api-desktop/core/internal/store"
)

type memoryTargetStore struct {
	mu      sync.Mutex
	nextID  int64
	targets map[int64]*store.CodexRemoteTarget
	listErr error
}

func newMemoryTargetStore() *memoryTargetStore {
	return &memoryTargetStore{nextID: 1, targets: make(map[int64]*store.CodexRemoteTarget)}
}

func cloneTarget(target *store.CodexRemoteTarget) *store.CodexRemoteTarget {
	copyTarget := *target
	return &copyTarget
}

func (s *memoryTargetStore) CreateCodexRemoteTarget(target *store.CodexRemoteTarget) (*store.CodexRemoteTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	created := cloneTarget(target)
	created.ID = s.nextID
	s.nextID++
	created.CreatedAt = time.Now()
	created.UpdatedAt = created.CreatedAt
	s.targets[created.ID] = cloneTarget(created)
	return cloneTarget(created), nil
}

func (s *memoryTargetStore) UpdateCodexRemoteTarget(target *store.CodexRemoteTarget) (*store.CodexRemoteTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.targets[target.ID]; !ok {
		return nil, store.ErrNotFound
	}
	updated := cloneTarget(target)
	updated.UpdatedAt = time.Now()
	s.targets[updated.ID] = cloneTarget(updated)
	return cloneTarget(updated), nil
}

func (s *memoryTargetStore) GetCodexRemoteTarget(id int64) (*store.CodexRemoteTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target := s.targets[id]
	if target == nil {
		return nil, store.ErrNotFound
	}
	return cloneTarget(target), nil
}

func (s *memoryTargetStore) ListCodexRemoteTargets() ([]*store.CodexRemoteTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	ids := make([]int64, 0, len(s.targets))
	for id := range s.targets {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := make([]*store.CodexRemoteTarget, 0, len(ids))
	for _, id := range ids {
		result = append(result, cloneTarget(s.targets[id]))
	}
	return result, nil
}

func (s *memoryTargetStore) DeleteCodexRemoteTarget(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.targets[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.targets, id)
	return nil
}

type lifecycleRemote struct {
	mu           sync.Mutex
	injectCalls  int
	restoreCalls int
	probeErr     error
	lastConfig   string
	lastAuth     string
}

func (r *lifecycleRemote) Probe(context.Context) (Probe, error) {
	if r.probeErr != nil {
		return Probe{}, r.probeErr
	}
	return Probe{OS: "Linux", Home: "/home/test", CodexDir: "/home/test/.codex"}, nil
}

func (r *lifecycleRemote) Inject(_ context.Context, _ string, config, auth string) error {
	r.mu.Lock()
	r.injectCalls++
	r.lastConfig = config
	r.lastAuth = auth
	r.mu.Unlock()
	return nil
}

func TestManagerDirectTargetLifecycle(t *testing.T) {
	storage := newMemoryTargetStore()
	operations := &lifecycleRemote{}
	manager, err := NewManager(storage, filepath.Join(t.TempDir(), "known_hosts"), func() string { return "127.0.0.1:1" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	listenCalls := 0
	dialCalls := 0
	manager.dial = func(context.Context, ProbeRequest, bool, bool) (*dialResult, error) {
		dialCalls++
		return &dialResult{connection: &scriptedConnection{listen: func(string, string) (net.Listener, error) {
			listenCalls++
			return nil, errors.New("direct mode attempted to create a tunnel")
		}}, fingerprint: "SHA256:test", known: true}, nil
	}
	manager.remote = func(remoteConnection) remoteOperations { return operations }

	const baseURL = "https://api.example.test/v1"
	const apiKey = "fixture-direct-key"
	request := InjectRequest{
		Host: "example.test", User: "deploy", Password: "fixture-password", Save: true, AcceptHostKey: true,
		Model: "gpt-5.6", Mode: ModeDirect, BaseURL: baseURL, APIKey: apiKey,
		Config: codexcfg.RenderConfig(baseURL, "gpt-5.6"), Auth: codexcfg.RenderAuth(apiKey),
	}
	injected, err := manager.Inject(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if injected.Mode != ModeDirect || injected.BaseURL != baseURL || injected.TunnelEnabled || injected.TunnelStatus != StatusInjectedDirect {
		t.Fatalf("direct target status = %#v", injected)
	}
	if listenCalls != 0 || dialCalls != 1 {
		t.Fatalf("direct inject listen calls = %d, dial calls = %d", listenCalls, dialCalls)
	}
	operations.mu.Lock()
	lastConfig, lastAuth := operations.lastConfig, operations.lastAuth
	operations.mu.Unlock()
	if lastConfig != request.Config || lastAuth != request.Auth {
		t.Fatal("direct inject did not pass the rendered config and auth to the remote operation")
	}
	encoded, err := json.Marshal(injected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), apiKey) {
		t.Fatal("direct target response exposed the API key")
	}
	_, err = manager.SetTunnel(t.Context(), injected.ID, true)
	assertRemoteErrorCode(t, err, "tunnel_not_applicable")
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewManager(storage, filepath.Join(t.TempDir(), "known_hosts"), func() string { return "127.0.0.1:1" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	reloadDialCalls := 0
	reloaded.dial = func(context.Context, ProbeRequest, bool, bool) (*dialResult, error) {
		reloadDialCalls++
		return &dialResult{connection: &scriptedConnection{}}, nil
	}
	reloaded.remote = func(remoteConnection) remoteOperations { return operations }
	reloaded.RestoreSaved(t.Context())
	if reloadDialCalls != 0 {
		t.Fatalf("startup restoration dialed a direct target %d times", reloadDialCalls)
	}
	targets, err := reloaded.Targets()
	if err != nil || len(targets) != 1 || targets[0].TunnelStatus != StatusInjectedDirect {
		t.Fatalf("reloaded direct targets = %#v, err=%v", targets, err)
	}
	restored, err := reloaded.Restore(t.Context(), injected.ID)
	if err != nil || restored.Injected || restored.TunnelStatus != StatusNotInjected {
		t.Fatalf("restored direct target = %#v, err=%v", restored, err)
	}
	if reloadDialCalls != 1 {
		t.Fatalf("direct restore dial calls = %d, want 1", reloadDialCalls)
	}
}

func (r *lifecycleRemote) Restore(context.Context, string) error {
	r.mu.Lock()
	r.restoreCalls++
	r.mu.Unlock()
	return nil
}

func lifecycleDialer(listenErr error) dialFunc {
	return func(context.Context, ProbeRequest, bool, bool) (*dialResult, error) {
		connection := &scriptedConnection{listen: func(string, string) (net.Listener, error) {
			if listenErr != nil {
				return nil, listenErr
			}
			return net.Listen("tcp", "127.0.0.1:0")
		}}
		return &dialResult{connection: connection, fingerprint: "SHA256:test", known: true}, nil
	}
}

func TestManagerSavedTargetLifecycle(t *testing.T) {
	storage := newMemoryTargetStore()
	operations := &lifecycleRemote{}
	manager, err := NewManager(storage, filepath.Join(t.TempDir(), "known_hosts"), func() string { return "127.0.0.1:1" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.dial = lifecycleDialer(nil)
	manager.remote = func(remoteConnection) remoteOperations { return operations }

	request := InjectRequest{
		Host: "example.test", User: "deploy", Password: "fixture-password", Save: true, AcceptHostKey: true,
		RemotePort: 8080, Model: "gpt-5.6", Config: codexcfg.RenderConfig("http://127.0.0.1:8080/v1", "gpt-5.6"),
		Auth: codexcfg.RenderAuth("fixture-key"),
	}
	injected, err := manager.Inject(t.Context(), request)
	if err != nil || !injected.Saved || injected.TunnelStatus != StatusConnected {
		t.Fatalf("injected = %#v, err=%v", injected, err)
	}
	targets, err := manager.Targets()
	if err != nil || len(targets) != 1 || targets[0].AuthPreview == "" {
		t.Fatalf("targets = %#v, err=%v", targets, err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewManager(storage, filepath.Join(t.TempDir(), "known_hosts"), func() string { return "127.0.0.1:1" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	reloaded.dial = lifecycleDialer(nil)
	reloaded.remote = func(remoteConnection) remoteOperations { return operations }
	reloaded.RestoreSaved(t.Context())
	targets, err = reloaded.Targets()
	if err != nil || len(targets) != 1 || targets[0].TunnelStatus != StatusConnected {
		t.Fatalf("restored targets = %#v, err=%v", targets, err)
	}
	restored, err := reloaded.Restore(t.Context(), injected.ID)
	if err != nil || restored.TunnelStatus != StatusNotInjected || restored.TunnelEnabled {
		t.Fatalf("restored = %#v, err=%v", restored, err)
	}
	if err := reloaded.Delete(injected.ID); err != nil {
		t.Fatal(err)
	}
	targets, err = reloaded.Targets()
	if err != nil || len(targets) != 0 {
		t.Fatalf("targets after delete = %#v, err=%v", targets, err)
	}
}

func TestManagerProbeAndFailureStates(t *testing.T) {
	manager, err := NewManager(nil, filepath.Join(t.TempDir(), "known_hosts"), func() string { return "127.0.0.1:1" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	operations := &lifecycleRemote{}
	manager.remote = func(remoteConnection) remoteOperations { return operations }
	manager.dial = lifecycleDialer(nil)
	probe, err := manager.Probe(t.Context(), ProbeRequest{Host: "example.test", User: "deploy", Password: "secret"})
	if err != nil || probe.HostKeyFingerprint != "SHA256:test" || !probe.Known {
		t.Fatalf("probe = %#v, err=%v", probe, err)
	}
	if _, err := manager.Probe(t.Context(), ProbeRequest{}); err == nil {
		t.Fatal("invalid probe target was accepted")
	}

	manager.dial = lifecycleDialer(errors.New("remote bind failed"))
	request := InjectRequest{
		Host: "example.test", User: "deploy", Password: "secret", AcceptHostKey: true,
		Config: codexcfg.RenderConfig("http://127.0.0.1:8080/v1", "gpt-5.6"), Auth: codexcfg.RenderAuth("key"),
	}
	_, err = manager.Inject(t.Context(), request)
	assertRemoteErrorCode(t, err, "tunnel_failed")
	operations.mu.Lock()
	restoreCalls := operations.restoreCalls
	operations.mu.Unlock()
	if restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", restoreCalls)
	}
	if err := manager.Delete(999); err == nil {
		t.Fatal("missing target delete succeeded")
	}
}

func TestManagerRejectsUnavailableStoreAndNotInjectedTunnel(t *testing.T) {
	_, err := NewManager(&memoryTargetStore{listErr: errors.New("list failed")}, filepath.Join(t.TempDir(), "known_hosts"), func() string { return "127.0.0.1:1" }, nil)
	if err == nil {
		t.Fatal("store list failure was ignored")
	}

	storage := newMemoryTargetStore()
	created, err := storage.CreateCodexRemoteTarget(&store.CodexRemoteTarget{
		Name: "saved", Host: "example.test", Port: 22, User: "deploy", Password: "secret", RemotePort: 8080,
		Model: "gpt-5.6", Injected: false, TunnelEnabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(storage, filepath.Join(t.TempDir(), "known_hosts"), func() string { return "127.0.0.1:1" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.SetTunnel(t.Context(), created.ID, true)
	assertRemoteErrorCode(t, err, "remote_command_failed")
}

func TestManagerPromotesEphemeralTargetWithoutDuplicate(t *testing.T) {
	storage := newMemoryTargetStore()
	manager, err := NewManager(storage, filepath.Join(t.TempDir(), "known_hosts"), func() string { return "127.0.0.1:1" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.dial = lifecycleDialer(nil)
	manager.remote = func(remoteConnection) remoteOperations { return &lifecycleRemote{} }
	request := InjectRequest{
		Host: "example.test", User: "deploy", Password: "secret", AcceptHostKey: true,
		Config: codexcfg.RenderConfig("http://127.0.0.1:8080/v1", "gpt-5.6"), Auth: codexcfg.RenderAuth("key"),
	}
	ephemeral, err := manager.Inject(t.Context(), request)
	if err != nil || ephemeral.ID >= 0 || ephemeral.Saved {
		t.Fatalf("ephemeral = %#v, err=%v", ephemeral, err)
	}
	request.ID = ephemeral.ID
	request.Password = ""
	request.Save = true
	saved, err := manager.Inject(t.Context(), request)
	if err != nil || saved.ID <= 0 || !saved.Saved {
		t.Fatalf("saved = %#v, err=%v", saved, err)
	}
	targets, err := manager.Targets()
	if err != nil || len(targets) != 1 || targets[0].ID != saved.ID {
		t.Fatalf("targets = %#v, err=%v", targets, err)
	}
}
