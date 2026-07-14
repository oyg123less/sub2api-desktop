// Package diagnostics runs bounded, read-only health checks and produces
// support reports with all sensitive values removed.
package diagnostics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/codexcfg"
	"sub2api-desktop/core/internal/redact"
	"sub2api-desktop/core/internal/store"
	apptransport "sub2api-desktop/core/internal/transport"
)

var ErrRunNotFound = errors.New("diagnostic run not found")

type ServerStatus interface {
	Running() bool
	Port() int
}

type CheckStatus string

const (
	StatusOK      CheckStatus = "ok"
	StatusWarning CheckStatus = "warning"
	StatusFailed  CheckStatus = "failed"
	StatusInfo    CheckStatus = "info"
)

type Check struct {
	ID         string         `json:"id"`
	Status     CheckStatus    `json:"status"`
	Title      string         `json:"title"`
	DurationMS int64          `json:"duration_ms"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
}

type Summary struct {
	OK      int `json:"ok"`
	Warning int `json:"warning"`
	Failed  int `json:"failed"`
}

type Run struct {
	RunID       string    `json:"run_id"`
	Status      string    `json:"status"`
	Progress    int       `json:"progress"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Summary     Summary   `json:"summary"`
	Checks      []Check   `json:"checks"`
}

type storedRun struct {
	mu  sync.RWMutex
	run Run
}

type Service struct {
	store      *store.Store
	settings   func() store.Settings
	server     ServerStatus
	dataDir    string
	version    string
	mu         sync.Mutex
	runs       map[string]*storedRun
	order      []string
	directHTTP *http.Client
}

func New(st *store.Store, settings func() store.Settings, server ServerStatus, dataDir, version string) *Service {
	return &Service{
		store: st, settings: settings, server: server, dataDir: dataDir, version: version,
		runs: make(map[string]*storedRun), directHTTP: &http.Client{Timeout: 15 * time.Second},
	}
}

type checkDefinition struct {
	id      string
	title   string
	timeout time.Duration
	run     func(context.Context) Check
}

func (s *Service) Start() Run {
	run := Run{RunID: newRunID(), Status: "running", Progress: 0, CreatedAt: time.Now(), Checks: []Check{}}
	stored := &storedRun{run: run}
	s.mu.Lock()
	s.runs[run.RunID] = stored
	s.order = append(s.order, run.RunID)
	if len(s.order) > 20 {
		delete(s.runs, s.order[0])
		s.order = s.order[1:]
	}
	s.mu.Unlock()
	go s.execute(stored)
	return cloneRun(stored)
}

func (s *Service) Get(id string) (Run, error) {
	s.mu.Lock()
	run := s.runs[id]
	s.mu.Unlock()
	if run == nil {
		return Run{}, ErrRunNotFound
	}
	return cloneRun(run), nil
}

func cloneRun(stored *storedRun) Run {
	stored.mu.RLock()
	defer stored.mu.RUnlock()
	result := stored.run
	result.Checks = append([]Check(nil), stored.run.Checks...)
	return result
}

func (s *Service) execute(stored *storedRun) {
	definitions := s.checks()
	results := make(chan Check, len(definitions))
	for _, definition := range definitions {
		definition := definition
		go func() {
			started := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), definition.timeout)
			defer cancel()
			check := definition.run(ctx)
			check.ID = definition.id
			check.Title = definition.title
			check.DurationMS = time.Since(started).Milliseconds()
			check.Message = redact.Sanitize(check.Message)
			results <- check
		}()
	}
	for range definitions {
		check := <-results
		stored.mu.Lock()
		stored.run.Checks = append(stored.run.Checks, check)
		stored.run.Progress = len(stored.run.Checks) * 100 / len(definitions)
		stored.mu.Unlock()
	}
	stored.mu.Lock()
	sort.SliceStable(stored.run.Checks, func(i, j int) bool {
		return statusRank(stored.run.Checks[i].Status) > statusRank(stored.run.Checks[j].Status)
	})
	for _, check := range stored.run.Checks {
		switch check.Status {
		case StatusFailed:
			stored.run.Summary.Failed++
		case StatusWarning:
			stored.run.Summary.Warning++
		default:
			stored.run.Summary.OK++
		}
	}
	stored.run.Status = "completed"
	stored.run.Progress = 100
	stored.run.CompletedAt = time.Now()
	stored.mu.Unlock()
}

func statusRank(status CheckStatus) int {
	switch status {
	case StatusFailed:
		return 3
	case StatusWarning:
		return 2
	case StatusInfo:
		return 1
	default:
		return 0
	}
}

func (s *Service) checks() []checkDefinition {
	return []checkDefinition{
		{"backend", "桌面后台", time.Second, s.checkBackend},
		{"data_dir", "数据目录", 2 * time.Second, s.checkDataDir},
		{"sqlite", "SQLite", 5 * time.Second, s.checkSQLite},
		{"api_listener", "API 监听", 2 * time.Second, s.checkAPI},
		{"accounts", "账号池", 2 * time.Second, s.checkAccounts},
		{"proxies", "代理", 15 * time.Second, s.checkProxies},
		{"oauth", "OAuth", 5 * time.Second, s.checkOAuth},
		{"chatgpt", "ChatGPT", 15 * time.Second, s.checkChatGPT},
		{"models", "模型", 3 * time.Second, s.checkModels},
		{"codex", "Codex 配置", 2 * time.Second, s.checkCodex},
		{"logs", "日志健康", 2 * time.Second, s.checkLogs},
		{"versions", "版本一致性", time.Second, s.checkVersions},
	}
}

func (s *Service) checkBackend(context.Context) Check {
	return Check{Status: StatusOK, Message: "sidecar control service is responsive", Details: map[string]any{"version": s.version}}
}

func (s *Service) checkDataDir(context.Context) Check {
	info, err := os.Stat(s.dataDir)
	if err != nil || !info.IsDir() {
		return failed("data directory is not readable", err)
	}
	probe := filepath.Join(s.dataDir, ".amber-diagnostic-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return failed("data directory is not writable", err)
	}
	_ = os.Remove(probe)
	missing := []string{}
	for _, name := range []string{"sub2api.db", "key"} {
		if _, err := os.Stat(filepath.Join(s.dataDir, name)); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return Check{Status: StatusFailed, Message: "required data files are missing", Details: map[string]any{"missing": missing}}
	}
	return Check{Status: StatusOK, Message: "data directory is readable and writable"}
}

func (s *Service) checkSQLite(context.Context) Check {
	if err := s.store.QuickCheck(); err != nil {
		return failed("SQLite quick_check failed", err)
	}
	return Check{Status: StatusOK, Message: "SQLite quick_check passed", Details: map[string]any{"schema_version": s.store.SchemaVersion()}}
}

func (s *Service) checkAPI(ctx context.Context) Check {
	if !s.server.Running() {
		return Check{Status: StatusFailed, Message: "local API server is stopped"}
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/health", s.server.Port()), nil)
	response, err := s.directHTTP.Do(request)
	if err != nil {
		return failed("local API health check failed", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Check{Status: StatusFailed, Message: fmt.Sprintf("local API health returned HTTP %d", response.StatusCode)}
	}
	return Check{Status: StatusOK, Message: "local API is listening", Details: map[string]any{"port": s.server.Port()}}
}

func (s *Service) checkAccounts(context.Context) Check {
	accounts, err := s.store.ListAccounts()
	if err != nil {
		return failed("account pool could not be read", err)
	}
	counts := map[string]int{}
	for _, account := range accounts {
		counts[string(account.Status)]++
	}
	status := StatusOK
	message := fmt.Sprintf("%d accounts are configured", len(accounts))
	if len(accounts) == 0 {
		status, message = StatusWarning, "no accounts are configured"
	} else if counts[string(store.AccountActive)] == 0 {
		status, message = StatusWarning, "no active accounts are available"
	}
	return Check{Status: status, Message: message, Details: map[string]any{"counts": counts}}
}

func (s *Service) checkProxies(ctx context.Context) Check {
	accounts, err := s.store.ListAccounts()
	if err != nil {
		return failed("proxy bindings could not be read", err)
	}
	used := map[int64]struct{}{}
	for _, account := range accounts {
		if account.ProxyID != nil {
			used[*account.ProxyID] = struct{}{}
		}
	}
	if len(used) == 0 {
		return Check{Status: StatusOK, Message: "no account-bound proxies require testing"}
	}
	failedIDs := []int64{}
	for id := range used {
		proxy, err := s.store.GetProxy(id)
		if err != nil {
			failedIDs = append(failedIDs, id)
			continue
		}
		client, err := apptransport.NewClient(apptransport.Options{
			Proxy: proxy, Purpose: apptransport.PurposeDiagnostic, FingerprintProfile: s.settings().CompatProfile, Timeout: 15 * time.Second,
		})
		if err != nil {
			failedIDs = append(failedIDs, id)
			continue
		}
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/cdn-cgi/trace", nil)
		response, err := client.Do(request)
		if err != nil {
			failedIDs = append(failedIDs, id)
			continue
		}
		response.Body.Close()
	}
	if len(failedIDs) > 0 {
		return Check{Status: StatusWarning, Message: "one or more bound proxies failed", Details: map[string]any{"failed_proxy_ids": failedIDs}}
	}
	return Check{Status: StatusOK, Message: fmt.Sprintf("%d bound proxies are reachable", len(used))}
}

func (s *Service) checkOAuth(context.Context) Check {
	listener, err := net.Listen("tcp", "127.0.0.1:1455")
	if err != nil {
		return Check{Status: StatusWarning, Message: "OAuth callback port 1455 is currently occupied"}
	}
	_ = listener.Close()
	accounts, _ := s.store.ListAccounts()
	refreshable := 0
	for _, account := range accounts {
		if account.RefreshToken != "" {
			refreshable++
		}
	}
	return Check{Status: StatusOK, Message: "OAuth callback port is available", Details: map[string]any{"refreshable_accounts": refreshable}}
}

func (s *Service) checkChatGPT(ctx context.Context) Check {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/cdn-cgi/trace", nil)
	response, err := s.directHTTP.Do(request)
	if err != nil {
		return failed("ChatGPT DNS/TLS/HTTP reachability failed", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return Check{Status: StatusFailed, Message: fmt.Sprintf("ChatGPT returned HTTP %d", response.StatusCode)}
	}
	return Check{Status: StatusOK, Message: fmt.Sprintf("ChatGPT is reachable (HTTP %d)", response.StatusCode)}
}

func (s *Service) checkModels(context.Context) Check {
	settings := s.settings()
	invalid := []string{}
	for _, model := range []string{settings.DefaultModel, settings.CodexModel} {
		value := strings.ToLower(strings.TrimSpace(model))
		if value == "" || (!strings.HasPrefix(value, "gpt-5") && !strings.Contains(value, "codex")) {
			invalid = append(invalid, model)
		}
	}
	if len(invalid) > 0 {
		return Check{Status: StatusWarning, Message: "one or more configured models are unsupported", Details: map[string]any{"invalid_models": invalid}}
	}
	return Check{Status: StatusOK, Message: "default and Codex models are supported"}
}

func (s *Service) checkCodex(context.Context) Check {
	manager, err := codexcfg.New("")
	if err != nil {
		return failed("Codex configuration directory is unavailable", err)
	}
	status, err := manager.Status("", "")
	if err != nil {
		return failed("Codex configuration could not be inspected", err)
	}
	if !status.ConfigExists {
		return Check{Status: StatusInfo, Message: "Codex configuration has not been created"}
	}
	if !status.Applied {
		return Check{Status: StatusWarning, Message: "Codex is not using the Amber provider"}
	}
	return Check{Status: StatusOK, Message: "Codex is configured for Amber"}
}

func (s *Service) checkLogs(context.Context) Check {
	health, err := s.store.LogHealth()
	if err != nil {
		return failed("request log health could not be read", err)
	}
	return Check{Status: StatusInfo, Message: fmt.Sprintf("%d request log rows", health.Rows), Details: map[string]any{
		"rows": health.Rows, "database_bytes": health.DatabaseBytes, "latest_error_kind": health.LatestErrorKind,
	}}
}

func (s *Service) checkVersions(context.Context) Check {
	if s.version == "" || strings.Contains(s.version, "dev") {
		return Check{Status: StatusWarning, Message: "sidecar is running a development or unknown version", Details: map[string]any{"sidecar": s.version}}
	}
	return Check{Status: StatusOK, Message: "sidecar version is available", Details: map[string]any{"sidecar": s.version}}
}

func failed(message string, err error) Check {
	if err != nil {
		message += ": " + err.Error()
	}
	return Check{Status: StatusFailed, Message: message}
}

func newRunID() string {
	bytes := make([]byte, 10)
	_, _ = rand.Read(bytes)
	return "diag_" + hex.EncodeToString(bytes)
}

func (s *Service) Report(id, format string) ([]byte, string, error) {
	run, err := s.Get(id)
	if err != nil {
		return nil, "", err
	}
	if format == "text" {
		var builder strings.Builder
		fmt.Fprintf(&builder, "Amber diagnostic report %s\nStatus: %s\nCreated: %s\n\n", run.RunID, run.Status, run.CreatedAt.Format(time.RFC3339))
		for _, check := range run.Checks {
			fmt.Fprintf(&builder, "[%s] %s (%dms): %s\n", strings.ToUpper(string(check.Status)), check.Title, check.DurationMS, check.Message)
		}
		return []byte(redact.Sanitize(builder.String())), "text/plain; charset=utf-8", nil
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return []byte(redact.Sanitize(string(data))), "application/json", nil
}
