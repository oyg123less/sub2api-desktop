package cloudsync

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/store"
)

type NetworkSettings struct {
	NetworkInfo
	Endpoint  string    `json:"endpoint,omitempty"`
	Fallback  bool      `json:"fallback"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type NetworkProbeStage struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type NetworkProbe struct {
	OK              bool                `json:"ok"`
	Target          string              `json:"target"`
	Endpoint        string              `json:"endpoint,omitempty"`
	Fallback        bool                `json:"fallback"`
	EffectiveSource string              `json:"effective_source"`
	ProxyName       string              `json:"proxy_name,omitempty"`
	ProxyType       string              `json:"proxy_type,omitempty"`
	Stages          []NetworkProbeStage `json:"stages"`
	ErrorCode       string              `json:"error_code,omitempty"`
	ErrorStage      string              `json:"error_stage,omitempty"`
	Error           string              `json:"error,omitempty"`
}

func (m *Manager) restoreNetworkClient() error {
	if m.client == nil || !m.client.configured() {
		return nil
	}
	if m.httpOverride != nil {
		m.client.setHTTPClient(m.httpOverride, nil)
		m.setNetworkInfo(NetworkInfo{Mode: store.CloudConnectionSystem, EffectiveFrom: "custom"})
		return nil
	}
	value, err := m.store.CloudConnectionSettings()
	if err != nil {
		m.client.setHTTPClient(nil, err)
		return err
	}
	httpClient, info, err := m.buildNetworkClient(value)
	if err != nil {
		m.client.setHTTPClient(nil, err)
		m.setNetworkInfo(NetworkInfo{Mode: value.Mode, ProxyID: value.ProxyID, EffectiveFrom: "unavailable"})
		return err
	}
	m.client.setHTTPClient(httpClient, nil)
	m.setNetworkInfo(info)
	return nil
}

func (m *Manager) setNetworkInfo(info NetworkInfo) {
	m.mu.Lock()
	m.networkInfo = info
	m.mu.Unlock()
}

func (m *Manager) ReloadNetworkSettings() error {
	err := m.restoreNetworkClient()
	m.closeRelaySession()
	return err
}

func (m *Manager) buildNetworkClient(value store.CloudConnectionSettings) (*http.Client, NetworkInfo, error) {
	if err := store.ValidateCloudConnectionSettings(value); err != nil {
		return nil, NetworkInfo{}, err
	}
	var selected *store.Proxy
	if value.Mode == store.CloudConnectionProxy {
		proxy, err := m.store.GetProxy(*value.ProxyID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, NetworkInfo{}, errors.New("selected Amber Cloud proxy is unavailable")
			}
			return nil, NetworkInfo{}, err
		}
		selected = proxy
	}
	return newConfiguredCloudHTTPClient(m.client.endpoint(), value, selected)
}

func (m *Manager) NetworkSettings() (NetworkSettings, error) {
	value, err := m.store.CloudConnectionSettings()
	if err != nil {
		return NetworkSettings{}, err
	}
	m.mu.RLock()
	info := m.networkInfo
	m.mu.RUnlock()
	return NetworkSettings{NetworkInfo: info, Endpoint: m.client.endpoint(), Fallback: m.client.usingFallback(), UpdatedAt: value.UpdatedAt}, nil
}

func (m *Manager) UpdateNetworkSettings(value store.CloudConnectionSettings) (NetworkSettings, error) {
	if m.httpOverride != nil {
		return NetworkSettings{}, errors.New("Amber Cloud network settings are unavailable with a custom HTTP client")
	}
	httpClient, info, err := m.buildNetworkClient(value)
	if err != nil {
		return NetworkSettings{}, err
	}
	if err := m.store.SaveCloudConnectionSettings(value); err != nil {
		return NetworkSettings{}, err
	}
	m.client.setHTTPClient(httpClient, nil)
	m.mu.Lock()
	m.networkInfo = info
	if m.lastErrorStage == "dns" || m.lastErrorStage == "connect" || m.lastErrorStage == "tls" || m.lastErrorStage == "timeout" || m.lastErrorStage == "network" || m.lastErrorStage == "http" || m.lastErrorStage == "response" || (m.lastErrorStage == "local" && m.lastErrorCode == "cloud_proxy_missing") {
		m.lastError = ""
		m.lastErrorCode = ""
		m.lastErrorStage = ""
		m.consecutiveFailures = 0
		m.nextRetryAt = time.Time{}
	}
	m.mu.Unlock()
	m.closeRelaySession()
	return m.NetworkSettings()
}

func (m *Manager) ProbeNetwork(ctx context.Context) NetworkProbe {
	result := m.probeNetworkOnce(ctx)
	if result.OK || len(m.client.endpoints()) < 2 {
		return result
	}
	previous := m.client.endpoint()
	if _, err := m.selectHealthyEndpoint(ctx); err != nil || strings.EqualFold(previous, m.client.endpoint()) {
		return result
	}
	result = m.probeNetworkOnce(ctx)
	if result.OK {
		m.mu.Lock()
		m.lastError = ""
		m.lastErrorCode = ""
		m.lastErrorStage = ""
		m.consecutiveFailures = 0
		m.nextRetryAt = time.Time{}
		m.mu.Unlock()
		m.closeRelaySession()
	}
	return result
}

func (m *Manager) selectHealthyEndpoint(ctx context.Context) (string, error) {
	if m.client == nil || !m.client.configured() {
		return "", errors.New("Amber Cloud is not configured")
	}
	httpClient, err := m.client.httpClient()
	if err != nil {
		return "", err
	}
	var lastErr error
	for _, endpoint := range m.client.endpoints() {
		attemptCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		request, requestErr := http.NewRequestWithContext(attemptCtx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/health", nil)
		if requestErr != nil {
			cancel()
			lastErr = requestErr
			continue
		}
		response, requestErr := httpClient.Do(request)
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
			_ = response.Body.Close()
		}
		cancel()
		if requestErr != nil {
			lastErr = requestErr
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			lastErr = fmt.Errorf("Amber Cloud health endpoint returned HTTP %d", response.StatusCode)
			continue
		}
		changed := m.client.useEndpoint(endpoint)
		if changed && m.httpOverride == nil {
			if err := m.restoreNetworkClient(); err != nil {
				return "", err
			}
		}
		return endpoint, nil
	}
	if lastErr == nil {
		lastErr = errors.New("Amber Cloud has no healthy endpoint")
	}
	return "", lastErr
}

func (m *Manager) probeNetworkOnce(ctx context.Context) NetworkProbe {
	endpoint := m.client.endpoint()
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return NetworkProbe{ErrorCode: "cloud_not_configured", ErrorStage: "local", Error: "Amber Cloud is not configured"}
	}
	m.mu.RLock()
	info := m.networkInfo
	m.mu.RUnlock()
	result := NetworkProbe{Target: parsed.Hostname(), Endpoint: endpoint, Fallback: m.client.usingFallback(), EffectiveSource: info.EffectiveFrom, ProxyName: info.ProxyName, ProxyType: info.ProxyType}
	stageOrder := []string{"dns", "connect", "tls", "http"}
	stages := make(map[string]*NetworkProbeStage, len(stageOrder))
	for _, id := range stageOrder {
		stages[id] = &NetworkProbeStage{ID: id, Status: "not_run"}
	}
	var stageMu sync.Mutex
	var dnsStart, connectStart, tlsStart time.Time
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			stageMu.Lock()
			dnsStart = time.Now()
			stages["dns"].Status = "running"
			stageMu.Unlock()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			stageMu.Lock()
			stages["dns"].LatencyMS = time.Since(dnsStart).Milliseconds()
			if info.Err != nil {
				stages["dns"].Status = "failed"
			} else {
				stages["dns"].Status = "ok"
			}
			stageMu.Unlock()
		},
		ConnectStart: func(_, _ string) {
			stageMu.Lock()
			connectStart = time.Now()
			stages["connect"].Status = "running"
			stageMu.Unlock()
		},
		ConnectDone: func(_, _ string, err error) {
			stageMu.Lock()
			stages["connect"].LatencyMS = time.Since(connectStart).Milliseconds()
			if err != nil {
				stages["connect"].Status = "failed"
			} else {
				stages["connect"].Status = "ok"
			}
			stageMu.Unlock()
		},
		TLSHandshakeStart: func() {
			stageMu.Lock()
			tlsStart = time.Now()
			stages["tls"].Status = "running"
			stageMu.Unlock()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			stageMu.Lock()
			stages["tls"].LatencyMS = time.Since(tlsStart).Milliseconds()
			if err != nil {
				stages["tls"].Status = "failed"
			} else {
				stages["tls"].Status = "ok"
			}
			stageMu.Unlock()
		},
	}
	request, _ := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, strings.TrimRight(endpoint, "/")+"/health", nil)
	start := time.Now()
	response, requestErr := m.client.do(request)
	stageMu.Lock()
	if requestErr == nil {
		stages["http"].LatencyMS = time.Since(start).Milliseconds()
		stages["http"].HTTPStatus = response.StatusCode
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			stages["http"].Status = "ok"
		} else {
			stages["http"].Status = "failed"
		}
	}
	for _, id := range stageOrder {
		if stages[id].Status == "not_run" || stages[id].Status == "running" {
			stages[id].Status = "skipped"
		}
		result.Stages = append(result.Stages, *stages[id])
	}
	stageMu.Unlock()
	if response != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
		_ = response.Body.Close()
	}
	if requestErr != nil {
		cloudErr := cloudTransportError(requestErr, 1)
		if errors.Is(requestErr, context.Canceled) {
			cloudErr = &CloudError{Code: "cloud_cancelled", Message: "Amber Cloud network probe was cancelled", Stage: "local"}
		}
		result.ErrorCode, result.ErrorStage, result.Error = cloudErr.Code, cloudErr.Stage, cloudErr.Message
		return result
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.ErrorCode = "cloud_http_" + strconv.Itoa(response.StatusCode)
		result.ErrorStage = "http"
		result.Error = "Amber Cloud health check failed"
		return result
	}
	result.OK = true
	return result
}
