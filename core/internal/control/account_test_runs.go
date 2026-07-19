package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

const (
	accountTestWorkers   = 3
	accountProxyWorkers  = 2
	accountTestResultTTL = 30 * time.Minute
)

var (
	errAccountTestRunActive   = errors.New("an account test run is already active")
	errAccountTestRunNotFound = errors.New("account test run not found")
)

type accountTestRunItem struct {
	AccountID     int64  `json:"account_id"`
	AccountLabel  string `json:"account_label"`
	Status        string `json:"status"`
	HTTPStatus    int    `json:"http_status,omitempty"`
	LatencyMS     int64  `json:"latency_ms,omitempty"`
	ErrorKind     string `json:"error_kind,omitempty"`
	Error         string `json:"error,omitempty"`
	AccountStatus string `json:"account_status,omitempty"`
}

type accountTestRunSnapshot struct {
	RunID      string               `json:"run_id"`
	Status     string               `json:"status"`
	Model      string               `json:"model"`
	Total      int                  `json:"total"`
	Completed  int                  `json:"completed"`
	Succeeded  int                  `json:"succeeded"`
	Failed     int                  `json:"failed"`
	Cancelled  int                  `json:"cancelled"`
	Skipped    int                  `json:"skipped"`
	Running    int                  `json:"running"`
	StartedAt  time.Time            `json:"started_at"`
	FinishedAt *time.Time           `json:"finished_at,omitempty"`
	Results    []accountTestRunItem `json:"results"`
}

type accountTestRun struct {
	id         string
	status     string
	model      string
	order      []int64
	items      map[int64]*accountTestRunItem
	startedAt  time.Time
	finishedAt time.Time
	cancel     context.CancelFunc
}

type accountTestRuns struct {
	mu      sync.Mutex
	store   *store.Store
	engine  *gateway.Engine
	current *accountTestRun
	limits  map[string]chan struct{}
}

func newAccountTestRuns(st *store.Store, engine *gateway.Engine) *accountTestRuns {
	return &accountTestRuns{store: st, engine: engine, limits: make(map[string]chan struct{})}
}

func maskedAccountLabel(account *store.Account) string {
	value := strings.TrimSpace(account.Email)
	if value == "" {
		value = strings.TrimSpace(account.BaseURL)
	}
	if at := strings.IndexByte(value, '@'); at > 0 {
		return value[:1] + "***" + value[at:]
	}
	if len(value) > 24 {
		return value[:21] + "..."
	}
	if value == "" {
		return fmt.Sprintf("#%d", account.ID)
	}
	return value
}

func (r *accountTestRuns) cleanupLocked(now time.Time) {
	if r.current != nil && !r.current.finishedAt.IsZero() && now.Sub(r.current.finishedAt) >= accountTestResultTTL {
		r.current = nil
	}
}

func (r *accountTestRuns) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked(time.Now())
	return r.current != nil && r.current.status == "running"
}

func (r *accountTestRuns) Start(scope string, requested []int64, model string) (accountTestRunSnapshot, error) {
	if r.engine == nil {
		return accountTestRunSnapshot{}, errors.New("account test engine is unavailable")
	}
	accounts, err := r.store.ListAccounts()
	if err != nil {
		return accountTestRunSnapshot{}, err
	}
	received, err := r.store.ListCloudReceivedAccounts()
	if err != nil {
		return accountTestRunSnapshot{}, err
	}
	accounts = append(accounts, received...)
	selected := make(map[int64]struct{}, len(requested))
	if scope == "selected" {
		if len(requested) == 0 || len(requested) > store.MaxAccountBatchSize {
			return accountTestRunSnapshot{}, fmt.Errorf("account_ids must contain between 1 and %d entries", store.MaxAccountBatchSize)
		}
		for _, id := range requested {
			if id == 0 {
				return accountTestRunSnapshot{}, errors.New("account_ids must contain non-zero integers")
			}
			selected[id] = struct{}{}
		}
	} else if scope != "all" {
		return accountTestRunSnapshot{}, errors.New("scope must be selected or all")
	}
	order := make([]int64, 0, len(accounts))
	items := make(map[int64]*accountTestRunItem)
	for _, account := range accounts {
		if scope == "selected" {
			if _, ok := selected[account.ID]; !ok {
				continue
			}
		}
		order = append(order, account.ID)
		items[account.ID] = &accountTestRunItem{AccountID: account.ID, AccountLabel: maskedAccountLabel(account), Status: "pending"}
	}
	if len(order) == 0 {
		return accountTestRunSnapshot{}, errors.New("no matching accounts were found")
	}
	if len(order) > store.MaxAccountBatchSize {
		return accountTestRunSnapshot{}, fmt.Errorf("account test runs support at most %d accounts", store.MaxAccountBatchSize)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked(time.Now())
	if r.current != nil && r.current.status == "running" {
		return r.snapshotLocked(r.current), errAccountTestRunActive
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()
	run := &accountTestRun{
		id: fmt.Sprintf("account-test-%d", now.UnixNano()), status: "running", model: strings.TrimSpace(model),
		order: order, items: items, startedAt: now, cancel: cancel,
	}
	r.current = run
	snapshot := r.snapshotLocked(run)
	go r.execute(ctx, run)
	return snapshot, nil
}

func (r *accountTestRuns) limiter(account *store.Account) chan struct{} {
	key := "direct"
	capacity := accountTestWorkers
	if account.ProxyID != nil {
		key = fmt.Sprintf("proxy-%d", *account.ProxyID)
		capacity = accountProxyWorkers
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := r.limits[key]
	if limit == nil {
		limit = make(chan struct{}, capacity)
		r.limits[key] = limit
	}
	return limit
}

func (r *accountTestRuns) execute(ctx context.Context, run *accountTestRun) {
	jobs := make(chan int64)
	var workers sync.WaitGroup
	for range accountTestWorkers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range jobs {
				r.executeOne(ctx, run, id)
			}
		}()
	}
	for _, id := range run.order {
		select {
		case <-ctx.Done():
			break
		case jobs <- id:
			continue
		}
		break
	}
	close(jobs)
	workers.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range run.items {
		if item.Status == "pending" || item.Status == "running" {
			item.Status = "cancelled"
		}
	}
	if ctx.Err() != nil {
		run.status = "cancelled"
	} else {
		run.status = "completed"
	}
	run.finishedAt = time.Now()
}

func (r *accountTestRuns) executeOne(ctx context.Context, run *accountTestRun, id int64) {
	r.mu.Lock()
	item := run.items[id]
	if item != nil {
		item.Status = "running"
	}
	r.mu.Unlock()
	var account *store.Account
	var err error
	if id < 0 {
		account, err = r.store.GetCloudReceivedAccount(id)
	} else {
		account, err = r.store.GetAccount(id)
	}
	if err != nil {
		r.mu.Lock()
		if errors.Is(err, store.ErrNotFound) {
			item.Status, item.ErrorKind = "skipped", "account_missing"
		} else {
			item.Status, item.ErrorKind, item.Error = "failed", "local", truncateAccountTestError(err.Error())
		}
		r.mu.Unlock()
		return
	}
	limit := r.limiter(account)
	select {
	case limit <- struct{}{}:
		defer func() { <-limit }()
	case <-ctx.Done():
		r.mu.Lock()
		item.Status = "cancelled"
		r.mu.Unlock()
		return
	}
	testCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
	result := r.engine.TestAccount(testCtx, account, run.model, "")
	cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	item.HTTPStatus = result.Status
	item.LatencyMS = result.LatencyMS
	item.AccountStatus = result.AccountStatus
	item.ErrorKind = result.ErrorKind
	item.Error = truncateAccountTestError(result.Error)
	if result.OK {
		item.Status = "succeeded"
	} else if errors.Is(testCtx.Err(), context.Canceled) && ctx.Err() != nil {
		item.Status = "cancelled"
	} else {
		item.Status = "failed"
		if item.ErrorKind == "" {
			item.ErrorKind = "upstream"
		}
	}
}

func truncateAccountTestError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 800 {
		return value[:797] + "..."
	}
	return value
}

func (r *accountTestRuns) snapshotLocked(run *accountTestRun) accountTestRunSnapshot {
	result := accountTestRunSnapshot{RunID: run.id, Status: run.status, Model: run.model, Total: len(run.order), StartedAt: run.startedAt}
	if !run.finishedAt.IsZero() {
		finished := run.finishedAt
		result.FinishedAt = &finished
	}
	for _, id := range run.order {
		item := *run.items[id]
		result.Results = append(result.Results, item)
		switch item.Status {
		case "succeeded":
			result.Succeeded++
			result.Completed++
		case "failed":
			result.Failed++
			result.Completed++
		case "cancelled":
			result.Cancelled++
			result.Completed++
		case "skipped":
			result.Skipped++
			result.Completed++
		case "running":
			result.Running++
		}
	}
	return result
}

func (r *accountTestRuns) Current() (*accountTestRunSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked(time.Now())
	if r.current == nil {
		return nil, nil
	}
	snapshot := r.snapshotLocked(r.current)
	return &snapshot, nil
}

func (r *accountTestRuns) Get(id string) (accountTestRunSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked(time.Now())
	if r.current == nil || r.current.id != strings.TrimSpace(id) {
		return accountTestRunSnapshot{}, errAccountTestRunNotFound
	}
	return r.snapshotLocked(r.current), nil
}

func (r *accountTestRuns) Cancel(id string) (accountTestRunSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil || r.current.id != strings.TrimSpace(id) {
		return accountTestRunSnapshot{}, errAccountTestRunNotFound
	}
	if r.current.status == "running" {
		r.current.cancel()
	}
	return r.snapshotLocked(r.current), nil
}

func (c *Control) startAccountTestRun(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Scope      string  `json:"scope"`
		AccountIDs []int64 `json:"account_ids"`
		Model      string  `json:"model"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&request); err != nil {
		writeControlError(w, http.StatusBadRequest, "invalid_account_test_run", "invalid account test request", false, nil)
		return
	}
	c.accountOpsMu.Lock()
	snapshot, err := c.accountTests.Start(request.Scope, request.AccountIDs, request.Model)
	c.accountOpsMu.Unlock()
	if errors.Is(err, errAccountTestRunActive) {
		writeControlError(w, http.StatusConflict, "account_test_run_active", err.Error(), true, map[string]any{"run_id": snapshot.RunID})
		return
	}
	if err != nil {
		writeControlError(w, http.StatusBadRequest, "invalid_account_test_run", err.Error(), false, nil)
		return
	}
	writeJSON(w, http.StatusAccepted, snapshot)
}

func (c *Control) activeAccountTestRun(w http.ResponseWriter, _ *http.Request) {
	run, err := c.accountTests.Current()
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, "account_test_run_failed", err.Error(), true, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

func (c *Control) accountTestRun(w http.ResponseWriter, r *http.Request) {
	run, err := c.accountTests.Get(r.PathValue("id"))
	if errors.Is(err, errAccountTestRunNotFound) {
		writeControlError(w, http.StatusNotFound, "account_test_run_not_found", err.Error(), false, nil)
		return
	}
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, "account_test_run_failed", err.Error(), true, nil)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (c *Control) cancelAccountTestRun(w http.ResponseWriter, r *http.Request) {
	run, err := c.accountTests.Cancel(r.PathValue("id"))
	if errors.Is(err, errAccountTestRunNotFound) {
		writeControlError(w, http.StatusNotFound, "account_test_run_not_found", err.Error(), false, nil)
		return
	}
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, "account_test_run_failed", err.Error(), true, nil)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
