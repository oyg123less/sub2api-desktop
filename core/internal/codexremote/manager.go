package codexremote

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"sub2api-desktop/core/internal/codexcfg"
	"sub2api-desktop/core/internal/store"
)

type runtimeTarget struct {
	record    *store.CodexRemoteTarget
	saved     bool
	tunnel    *tunnel
	lastError string
}

// Manager owns saved SSH targets and their active reverse tunnels.
type Manager struct {
	store        targetStore
	localAddress func() string
	logger       *slog.Logger
	dial         dialFunc
	remote       remoteFactory

	mu      sync.Mutex
	opMu    sync.Mutex
	targets map[int64]*runtimeTarget
	nextID  int64
}

func NewManager(st targetStore, knownHostsPath string, localAddress func() string, logger *slog.Logger) (*Manager, error) {
	knownHosts, err := newKnownHostStore(knownHostsPath)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	manager := &Manager{
		store: st, localAddress: localAddress, logger: logger, dial: makeSSHDialer(knownHosts),
		remote: func(connection remoteConnection) remoteOperations {
			return &sshRemoteOperations{connection: connection}
		},
		targets: make(map[int64]*runtimeTarget), nextID: -1,
	}
	if st != nil {
		targets, err := st.ListCodexRemoteTargets()
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			if target.Mode == "" {
				target.Mode = ModeTunnel
			}
			target.APIKey = ""
			manager.targets[target.ID] = &runtimeTarget{record: target, saved: true}
		}
	}
	return manager, nil
}

// ReloadSaved merges cloud-synced target records into the runtime manager and
// starts routing only for newly discovered saved tunnel targets.
func (m *Manager) ReloadSaved(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	targets, err := m.store.ListCodexRemoteTargets()
	if err != nil {
		return err
	}
	seen := make(map[int64]bool, len(targets))
	var startIDs []int64
	var removed []*tunnel
	m.mu.Lock()
	for _, target := range targets {
		seen[target.ID] = true
		if target.Mode == "" {
			target.Mode = ModeTunnel
		}
		target.APIKey = ""
		if current := m.targets[target.ID]; current != nil {
			current.record = target
			current.saved = true
			continue
		}
		m.targets[target.ID] = &runtimeTarget{record: target, saved: true}
		if target.Mode != ModeDirect && target.Injected && target.TunnelEnabled {
			startIDs = append(startIDs, target.ID)
		}
	}
	for id, current := range m.targets {
		if current.saved && !seen[id] {
			if current.tunnel != nil {
				removed = append(removed, current.tunnel)
			}
			delete(m.targets, id)
		}
	}
	m.mu.Unlock()
	for _, active := range removed {
		closeActiveTunnel(active)
	}
	for _, id := range startIDs {
		if err := m.enableTunnel(ctx, id); err != nil {
			m.setLastError(id, err.Error())
			m.logger.Warn("start cloud-synced Codex tunnel failed", "target_id", id, "error", err)
		}
	}
	return nil
}

func (m *Manager) Probe(ctx context.Context, request ProbeRequest) (Probe, error) {
	request, err := normalizeProbeRequest(request)
	if err != nil {
		return Probe{}, err
	}
	result, err := m.dial(ctx, request, true, false)
	if err != nil {
		return Probe{}, err
	}
	defer result.connection.Close()
	probe, err := m.remote(result.connection).Probe(ctx)
	if err != nil {
		return Probe{}, err
	}
	probe.HostKeyFingerprint = result.fingerprint
	probe.Known = result.known
	return probe, nil
}

func (m *Manager) Inject(ctx context.Context, request InjectRequest) (TargetStatus, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	request, existing, err := m.normalizeInjectRequest(request)
	if err != nil {
		return TargetStatus{}, err
	}
	if err := codexcfg.ValidateConfig(request.Config); err != nil {
		return TargetStatus{}, err
	}
	if err := codexcfg.ValidateAuth(request.Auth); err != nil {
		return TargetStatus{}, err
	}
	credentials := ProbeRequest{Host: request.Host, Port: request.Port, User: request.User, Password: request.Password}
	result, err := m.dial(ctx, credentials, request.AcceptHostKey, request.AcceptHostKey)
	if err != nil {
		return TargetStatus{}, err
	}
	connectionOwned := true
	defer func() {
		if connectionOwned {
			_ = result.connection.Close()
		}
	}()
	remote := m.remote(result.connection)
	probe, err := remote.Probe(ctx)
	if err != nil {
		return TargetStatus{}, err
	}
	if err := remote.Inject(ctx, probe.CodexDir, request.Config, request.Auth); err != nil {
		return TargetStatus{}, err
	}

	if existing != nil {
		m.closeTunnel(existing.record.ID)
	}
	var activeTunnel *tunnel
	if request.Mode == ModeTunnel {
		activeTunnel, err = startTunnel(result.connection, address("127.0.0.1", request.RemotePort), m.localAddress())
		if err != nil {
			if existing == nil {
				_ = remote.Restore(ctx, probe.CodexDir)
			}
			return TargetStatus{}, err
		}
		connectionOwned = false
	}

	apiKeyForStorage := request.APIKey
	if request.reuseAPIKey {
		apiKeyForStorage = ""
	}
	record := &store.CodexRemoteTarget{
		Name: request.Name, Host: request.Host, Port: request.Port, User: request.User, Password: request.Password,
		RemotePort: request.RemotePort, Model: request.Model, Mode: request.Mode, BaseURL: request.BaseURL, APIKey: apiKeyForStorage,
		TunnelEnabled: request.Mode == ModeTunnel, Injected: true,
	}
	request.APIKey = ""
	request.Auth = ""
	existingID := int64(0)
	saved := request.Save
	if existing != nil {
		existingID = existing.record.ID
		record.ID = existing.record.ID
		record.CreatedAt = existing.record.CreatedAt
		saved = existing.saved || request.Save
	}
	if saved {
		if m.store == nil {
			closeActiveTunnel(activeTunnel)
			if existing == nil {
				_ = remote.Restore(ctx, probe.CodexDir)
			}
			return TargetStatus{}, fmt.Errorf("remote target store is unavailable")
		}
		if record.ID > 0 {
			record, err = m.store.UpdateCodexRemoteTarget(record)
		} else {
			record, err = m.store.CreateCodexRemoteTarget(record)
		}
		if err != nil {
			closeActiveTunnel(activeTunnel)
			if existing == nil {
				_ = remote.Restore(ctx, probe.CodexDir)
			}
			return TargetStatus{}, err
		}
	} else if record.ID == 0 {
		m.mu.Lock()
		record.ID = m.nextID
		m.nextID--
		m.mu.Unlock()
	}
	record.APIKey = ""

	runtime := &runtimeTarget{record: record, saved: saved, tunnel: activeTunnel}
	m.mu.Lock()
	if existingID != 0 && existingID != record.ID {
		delete(m.targets, existingID)
	}
	m.targets[record.ID] = runtime
	m.mu.Unlock()
	if activeTunnel != nil {
		m.monitorTunnel(record.ID, activeTunnel)
	}
	return m.status(record.ID)
}

func closeActiveTunnel(active *tunnel) {
	if active != nil {
		_ = active.Close()
	}
}

func (m *Manager) Targets() ([]TargetStatus, error) {
	m.mu.Lock()
	ids := make([]int64, 0, len(m.targets))
	for id := range m.targets {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := make([]TargetStatus, 0, len(ids))
	for _, id := range ids {
		status, err := m.status(id)
		if err != nil {
			return nil, err
		}
		result = append(result, status)
	}
	return result, nil
}

func (m *Manager) SetTunnel(ctx context.Context, id int64, enabled bool) (TargetStatus, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	runtime, err := m.get(id)
	if err != nil {
		return TargetStatus{}, err
	}
	if runtime.record.Mode == ModeDirect {
		return TargetStatus{}, codedError("tunnel_not_applicable", nil)
	}
	if !enabled {
		runtime.record.TunnelEnabled = false
		if runtime.saved {
			updated, err := m.store.UpdateCodexRemoteTarget(runtime.record)
			if err != nil {
				return TargetStatus{}, err
			}
			runtime.record = updated
		}
		m.updateRuntimeRecord(id, runtime.record, "")
		m.closeTunnel(id)
		return m.status(id)
	}
	if !runtime.record.Injected {
		return TargetStatus{}, codedError("remote_command_failed", errors.New("target is not injected"))
	}
	runtime.record.TunnelEnabled = true
	if runtime.saved {
		updated, err := m.store.UpdateCodexRemoteTarget(runtime.record)
		if err != nil {
			return TargetStatus{}, err
		}
		runtime.record = updated
	}
	m.updateRuntimeRecord(id, runtime.record, "")
	if err := m.enableTunnel(ctx, id); err != nil {
		m.setLastError(id, err.Error())
		return TargetStatus{}, err
	}
	return m.status(id)
}

func (m *Manager) Restore(ctx context.Context, id int64) (TargetStatus, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	runtime, err := m.get(id)
	if err != nil {
		return TargetStatus{}, err
	}
	m.closeTunnel(id)
	credentials := ProbeRequest{Host: runtime.record.Host, Port: runtime.record.Port, User: runtime.record.User, Password: runtime.record.Password}
	result, err := m.dial(ctx, credentials, false, false)
	if err != nil {
		return TargetStatus{}, err
	}
	defer result.connection.Close()
	remote := m.remote(result.connection)
	probe, err := remote.Probe(ctx)
	if err != nil {
		return TargetStatus{}, err
	}
	if err := remote.Restore(ctx, probe.CodexDir); err != nil {
		return TargetStatus{}, err
	}
	runtime.record.Injected = false
	runtime.record.TunnelEnabled = false
	if runtime.saved {
		updated, err := m.store.UpdateCodexRemoteTarget(runtime.record)
		if err != nil {
			m.updateRuntimeRecord(id, runtime.record, "")
			return TargetStatus{}, err
		}
		runtime.record = updated
	}
	m.updateRuntimeRecord(id, runtime.record, "")
	return m.status(id)
}

func (m *Manager) Delete(id int64) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	runtime, err := m.get(id)
	if err != nil {
		return err
	}
	m.closeTunnel(id)
	if runtime.saved {
		if err := m.store.DeleteCodexRemoteTarget(id); err != nil {
			return err
		}
	}
	m.mu.Lock()
	delete(m.targets, id)
	m.mu.Unlock()
	return nil
}

func (m *Manager) RestoreSaved(ctx context.Context) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	m.mu.Lock()
	ids := []int64{}
	for id, runtime := range m.targets {
		if runtime.saved && runtime.record.Mode != ModeDirect && runtime.record.Injected && runtime.record.TunnelEnabled {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		if err := m.enableTunnel(ctx, id); err != nil {
			m.setLastError(id, err.Error())
			m.logger.Warn("restore Codex remote tunnel failed", "target_id", id, "error", err)
		}
	}
}

func (m *Manager) Close() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	m.mu.Lock()
	tunnels := []*tunnel{}
	for _, runtime := range m.targets {
		if runtime.tunnel != nil {
			tunnels = append(tunnels, runtime.tunnel)
			runtime.tunnel = nil
		}
	}
	m.mu.Unlock()
	var result error
	for _, value := range tunnels {
		result = errors.Join(result, value.Close())
	}
	return result
}

func (m *Manager) normalizeInjectRequest(request InjectRequest) (InjectRequest, *runtimeTarget, error) {
	var existing *runtimeTarget
	if request.ID != 0 {
		value, err := m.get(request.ID)
		if err != nil {
			return InjectRequest{}, nil, err
		}
		existing = value
		if strings.TrimSpace(request.Password) == "" {
			request.Password = value.record.Password
		}
	}
	probe, err := normalizeProbeRequest(ProbeRequest{Host: request.Host, Port: request.Port, User: request.User, Password: request.Password})
	if err != nil {
		return InjectRequest{}, nil, err
	}
	request.Host, request.Port, request.User, request.Password = probe.Host, probe.Port, probe.User, probe.Password
	request.Mode = strings.TrimSpace(request.Mode)
	if request.Mode == "" {
		request.Mode = ModeTunnel
	}
	switch request.Mode {
	case ModeTunnel:
		request.BaseURL = ""
		request.APIKey = ""
	case ModeDirect:
		request.BaseURL = strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
		request.APIKey = strings.TrimSpace(request.APIKey)
		if request.BaseURL == "" {
			return InjectRequest{}, nil, codedError("invalid_target", nil)
		}
		if request.APIKey == "" {
			if existing == nil || !existing.saved || existing.record.ID <= 0 || m.store == nil {
				return InjectRequest{}, nil, codedError("invalid_target", nil)
			}
			persisted, err := m.store.GetCodexRemoteTarget(existing.record.ID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return InjectRequest{}, nil, codedError("target_not_found", err)
				}
				return InjectRequest{}, nil, err
			}
			request.APIKey = strings.TrimSpace(persisted.APIKey)
			persisted.APIKey = ""
			if request.APIKey == "" {
				return InjectRequest{}, nil, codedError("invalid_target", nil)
			}
			request.reuseAPIKey = true
		}
		request.Auth = codexcfg.RenderAuth(request.APIKey)
	default:
		return InjectRequest{}, nil, codedError("invalid_target", nil)
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		request.Name = request.User + "@" + request.Host
	}
	request.Model = strings.TrimSpace(request.Model)
	if request.Model == "" {
		request.Model = codexcfg.DefaultModel
	}
	if request.RemotePort == 0 {
		request.RemotePort = 8080
	}
	if request.Mode == ModeTunnel && (request.RemotePort < 1 || request.RemotePort > 65535) {
		return InjectRequest{}, nil, codedError("invalid_target", nil)
	}
	return request, existing, nil
}

func (m *Manager) enableTunnel(ctx context.Context, id int64) error {
	runtime, err := m.get(id)
	if err != nil {
		return err
	}
	if runtime.record.Mode == ModeDirect {
		return codedError("tunnel_not_applicable", nil)
	}
	m.closeTunnel(id)
	credentials := ProbeRequest{Host: runtime.record.Host, Port: runtime.record.Port, User: runtime.record.User, Password: runtime.record.Password}
	result, err := m.dial(ctx, credentials, false, false)
	if err != nil {
		return err
	}
	remote := m.remote(result.connection)
	if _, err := remote.Probe(ctx); err != nil {
		_ = result.connection.Close()
		return err
	}
	active, err := startTunnel(result.connection, address("127.0.0.1", runtime.record.RemotePort), m.localAddress())
	if err != nil {
		_ = result.connection.Close()
		return err
	}
	m.mu.Lock()
	current := m.targets[id]
	if current == nil || !current.record.TunnelEnabled {
		m.mu.Unlock()
		_ = active.Close()
		return codedError("target_not_found", store.ErrNotFound)
	}
	current.tunnel = active
	current.lastError = ""
	m.mu.Unlock()
	m.monitorTunnel(id, active)
	return nil
}

func (m *Manager) monitorTunnel(id int64, active *tunnel) {
	go func() {
		<-active.Done()
		m.mu.Lock()
		defer m.mu.Unlock()
		if runtime := m.targets[id]; runtime != nil && runtime.tunnel == active {
			runtime.tunnel = nil
			if runtime.record.TunnelEnabled && active.Err() != nil {
				runtime.lastError = "SSH tunnel disconnected"
			}
		}
	}()
}

func (m *Manager) closeTunnel(id int64) {
	m.mu.Lock()
	runtime := m.targets[id]
	var active *tunnel
	if runtime != nil {
		active = runtime.tunnel
		runtime.tunnel = nil
	}
	m.mu.Unlock()
	if active != nil {
		_ = active.Close()
	}
}

func (m *Manager) get(id int64) (*runtimeTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime := m.targets[id]
	if runtime == nil {
		return nil, codedError("target_not_found", store.ErrNotFound)
	}
	copyRuntime := *runtime
	copyRecord := *runtime.record
	copyRuntime.record = &copyRecord
	return &copyRuntime, nil
}

func (m *Manager) updateRuntimeRecord(id int64, record *store.CodexRemoteTarget, lastError string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if runtime := m.targets[id]; runtime != nil {
		copyRecord := *record
		runtime.record = &copyRecord
		runtime.lastError = lastError
	}
}

func (m *Manager) setLastError(id int64, message string) {
	m.mu.Lock()
	if runtime := m.targets[id]; runtime != nil {
		runtime.lastError = message
	}
	m.mu.Unlock()
}

func (m *Manager) status(id int64) (TargetStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime := m.targets[id]
	if runtime == nil {
		return TargetStatus{}, codedError("target_not_found", store.ErrNotFound)
	}
	record := runtime.record
	status := StatusDown
	switch {
	case !record.Injected:
		status = StatusNotInjected
	case record.Mode == ModeDirect:
		status = StatusInjectedDirect
	case !record.TunnelEnabled:
		status = StatusDisabled
	case runtime.tunnel != nil:
		status = StatusConnected
	}
	mode := record.Mode
	if mode == "" {
		mode = ModeTunnel
	}
	baseURL := "http://127.0.0.1:" + fmt.Sprintf("%d", record.RemotePort) + "/v1"
	responseBaseURL := ""
	if mode == ModeDirect {
		baseURL = record.BaseURL
		responseBaseURL = record.BaseURL
	}
	return TargetStatus{
		ID: record.ID, Name: record.Name, Host: record.Host, Port: record.Port, User: record.User,
		RemotePort: record.RemotePort, Model: record.Model, Mode: mode, BaseURL: responseBaseURL,
		Saved: runtime.saved, Injected: record.Injected,
		TunnelEnabled: record.TunnelEnabled, TunnelStatus: status, LastError: runtime.lastError,
		ConfigPreview: codexcfg.RenderConfig(baseURL, record.Model),
		AuthPreview:   codexcfg.RenderAuth("********"), UpdatedAt: record.UpdatedAt,
	}, nil
}
