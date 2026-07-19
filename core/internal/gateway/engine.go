// Package gateway forwards OpenAI-compatible Chat Completions requests to the
// ChatGPT Codex backend, applying anti-ban disguise (instruction injection,
// Codex client headers, TLS fingerprint), streaming translation, account
// selection and 429 failover.
package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/apicompat"
	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/redact"
	"sub2api-desktop/core/internal/store"
	apptransport "sub2api-desktop/core/internal/transport"
)

// UpstreamURL returns the effective upstream endpoint, allowing override via the
// SUB2API_UPSTREAM_URL environment variable (used for tests).
func UpstreamURL() string {
	if v := os.Getenv("SUB2API_UPSTREAM_URL"); v != "" {
		return v
	}
	return openai.CodexResponsesURL
}

func upstreamURLForAccount(acc *store.Account) string {
	if acc != nil && acc.AccountType == store.AccountTypeAPIKey {
		if baseURL := strings.TrimSpace(acc.BaseURL); baseURL != "" {
			return apiKeyResponsesURL(baseURL)
		}
		return openai.CodexResponsesURL
	}
	return UpstreamURL()
}

// apiKeyResponsesURL resolves the Responses endpoint for an API-key account:
// a full endpoint ending in "/responses" is used as-is, a base URL ending in a
// version segment (e.g. "/v1") gets "/responses" appended, and a bare base URL
// gets "/v1/responses" appended.
func apiKeyResponsesURL(base string) string {
	normalized := strings.TrimRight(base, "/")
	if strings.HasSuffix(normalized, "/responses") {
		return normalized
	}
	segment := normalized
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		segment = normalized[idx+1:]
	}
	if isVersionSegment(segment) {
		return normalized + "/responses"
	}
	return normalized + "/v1/responses"
}

func isVersionSegment(s string) bool {
	s = strings.ToLower(s)
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Engine holds the dependencies for request forwarding.
type Engine struct {
	store     *store.Store
	accounts  *account.Manager
	settings  func() store.Settings
	logger    *slog.Logger
	scheduler *Scheduler
}

// New constructs a gateway engine.
func New(s *store.Store, mgr *account.Manager, settings func() store.Settings, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{store: s, accounts: mgr, settings: settings, logger: logger, scheduler: NewScheduler()}
}

// apiError is the OpenAI-style error envelope.
type apiError struct {
	Error apiErrorBody `json:"error"`
}
type apiErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

func writeError(w http.ResponseWriter, status int, msg, typ string) {
	writeErrorCode(w, status, msg, typ, "")
}

func writeErrorCode(w http.ResponseWriter, status int, msg, typ, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{Error: apiErrorBody{Message: msg, Type: typ, Code: code}})
}

var errAccountQueueTimeout = errors.New("account queue wait timed out")

const accountQueueWaitTimeout = 60 * time.Second

func writeAccountSelectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAccountQueueFull):
		w.Header().Set("Retry-After", "1")
		writeErrorCode(w, http.StatusTooManyRequests, "所有账号的等待队列均已满，请稍后重试", "rate_limit_error", "account_queue_full")
	case errors.Is(err, errAccountQueueTimeout):
		w.Header().Set("Retry-After", "1")
		writeErrorCode(w, http.StatusServiceUnavailable, "等待可用账号超时，请稍后重试", "server_error", "account_queue_timeout")
	default:
		writeError(w, http.StatusInternalServerError, "account lookup failed: "+err.Error(), "internal_error")
	}
}

// ChatCompletions handles POST /v1/chat/completions.
func (e *Engine) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error")
		return
	}
	var chatReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error(), "invalid_request_error")
		return
	}
	if len(chatReq.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages is required", "invalid_request_error")
		return
	}

	cfg := e.settings()
	requestedModel := chatReq.Model
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = uuid.NewString()
	}
	model, ok := resolveModel(requestedModel, cfg.DefaultModel)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown model: "+requestedModel+"（仅支持 gpt-5*/codex 系列模型）", "invalid_request_error")
		return
	}
	chatReq.Model = model
	logModel := upstreamLogModel(model)

	candidates, releaseFirst, err := e.selectAccounts(r.Context())
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		writeAccountSelectionError(w, err)
		return
	}
	if len(candidates) == 0 {
		writeError(w, http.StatusServiceUnavailable, "没有可用账号，请先在应用内登录 ChatGPT 账号", "no_account")
		return
	}

	var lastErr string
	lastStatus := 0
	attempt := 0
	for index, acc := range candidates {
		release := releaseFirst
		if index > 0 {
			var acquired bool
			release, acquired = e.scheduler.TryAcquire(acc.ID, acc.MaxConcurrency)
			if !acquired {
				continue
			}
		}
		attempt++
		meta := forwardMeta{RequestID: requestID, RequestedModel: requestedModel, ResolvedModel: logModel, Attempt: attempt, Stream: chatReq.Stream, EstimatedPromptTokens: estimateTokensFromBytes(len(bodyBytes))}
		result := e.forwardOnce(r.Context(), w, &chatReq, acc, cfg, meta)
		if result.outcome == outcomeAuthFailed && acc.AccountType == store.AccountTypeOAuth && acc.RefreshToken != "" {
			if refreshed, err := e.forceRefreshAccount(r.Context(), acc, cfg); err == nil {
				acc = refreshed
				attempt++
				meta.Attempt = attempt
				result = e.forwardOnce(r.Context(), w, &chatReq, acc, cfg, meta)
			} else if isNetworkOrProxyError(err) {
				result = forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadGateway, errMsg: "token refresh connection failed: " + err.Error(), retryable: true}
			} else {
				result.errMsg = "token refresh failed: " + err.Error()
			}
		}
		release()
		switch result.outcome {
		case outcomeSuccess:
			e.recordAccountSuccessFor(acc)
			return
		case outcomeRateLimited:
			retry := result.retryAfter
			if retry <= 0 {
				retry = 30 * time.Second
			}
			until := time.Now().Add(retry)
			e.recordAccountRateLimitFor(acc, until, result.statusReason)
			lastErr = result.errMsg
			lastStatus = 0
			continue // try next account
		case outcomeAuthFailed:
			e.recordAccountAuthFailureFor(acc, result.errMsg, result.errorKind)
			lastErr = result.errMsg
			lastStatus = 0
			continue
		case outcomeClientClosed:
			return
		default:
			if shouldFailoverUpstreamError(result) {
				lastErr = result.errMsg
				lastStatus = result.status
				continue
			}
			// Non-retryable upstream/other error: surface immediately if we
			// have not written a response yet.
			if !result.headersWritten {
				writeError(w, result.status, result.errMsg, "upstream_error")
			}
			return
		}
	}

	if lastErr == "" {
		lastErr = "所有账号均不可用（限额或需要重新登录）"
	}
	if lastStatus >= http.StatusInternalServerError {
		writeError(w, lastStatus, lastErr, "upstream_error")
		return
	}
	writeError(w, http.StatusServiceUnavailable, lastErr, "all_accounts_unavailable")
}

type forwardOutcome int

const (
	outcomeSuccess forwardOutcome = iota
	outcomeRateLimited
	outcomeAuthFailed
	outcomeUpstreamError
	outcomeClientClosed
)

type forwardResult struct {
	outcome        forwardOutcome
	status         int
	errMsg         string
	headersWritten bool
	retryAfter     time.Duration
	retryable      bool
	statusReason   string
	errorKind      string
	terminalEvent  string
}

type forwardMeta struct {
	RequestID, RequestedModel, ResolvedModel string
	Attempt, EstimatedPromptTokens           int
	Stream                                   bool
}

var errBoundProxyUnavailable = errors.New("bound proxy is unavailable")

func (e *Engine) forwardOnce(ctx context.Context, w http.ResponseWriter, chatReq *apicompat.ChatCompletionsRequest, acc *store.Account, cfg store.Settings, meta forwardMeta) forwardResult {
	start := time.Now()

	// Resolve proxy + client.
	proxy, err := e.proxyForAccount(acc)
	if err != nil {
		message := errBoundProxyUnavailable.Error()
		e.logForward(acc, meta, http.StatusBadGateway, 0, 0, time.Since(start), message, "proxy_unavailable", "proxy_resolution_failed")
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadGateway, errMsg: message, retryable: true}
	}
	client, err := newHTTPClient(proxy, cfg.CompatProfile, 10*time.Minute)
	if err != nil {
		e.logForward(acc, meta, http.StatusInternalServerError, 0, 0, time.Since(start), err.Error(), "upstream_network_error", "client_setup_failed")
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error(), retryable: true}
	}
	authClient, _ := newHTTPClient(proxy, "standard", 60*time.Second)

	token, err := e.accounts.ValidAccessToken(ctx, authClient, acc)
	if err != nil {
		if ctx.Err() != nil {
			e.logForwardUsage(acc, meta, 499, estimatedUsage(meta, 0), time.Since(start), "client cancelled request", "client_cancelled", "request_cancelled")
			return forwardResult{outcome: outcomeClientClosed}
		}
		if isNetworkOrProxyError(err) {
			e.logForward(acc, meta, http.StatusBadGateway, 0, 0, time.Since(start), err.Error(), "upstream_network_error", "token_refresh_connection_failed")
			return forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadGateway, errMsg: "token refresh connection failed: " + err.Error(), retryable: true}
		}
		return forwardResult{outcome: outcomeAuthFailed, errMsg: "token refresh failed: " + err.Error()}
	}

	// Build the Responses request.
	respReq, err := apicompat.ChatCompletionsToResponses(chatReq)
	if err != nil {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadRequest, errMsg: "request transform failed: " + err.Error()}
	}
	// Map the requested model (possibly carrying a reasoning-effort suffix
	// like gpt-5.4-high) to the canonical upstream model + reasoning.effort.
	upstreamModel, effort := openai.MapCodexModel(chatReq.Model)
	respReq.Model = upstreamModel
	if respReq.Reasoning == nil && effort != "" {
		respReq.Reasoning = &apicompat.ResponsesReasoning{Effort: effort, Summary: "auto"}
	}
	applyAntiBan(respReq, upstreamModel, cfg)

	upstreamBody, err := json.Marshal(respReq)
	if err != nil {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURLForAccount(acc), bytes.NewReader(upstreamBody))
	if err != nil {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error()}
	}
	setCodexHeaders(req, token, acc, cfg)

	markRelayUpstreamStarted(ctx)
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			e.logForwardUsage(acc, meta, 499, estimatedUsage(meta, 0), time.Since(start), "client cancelled request", "client_cancelled", "request_cancelled")
			return forwardResult{outcome: outcomeClientClosed}
		}
		message := "upstream request failed: " + err.Error()
		e.logForward(acc, meta, http.StatusBadGateway, 0, 0, time.Since(start), message, "upstream_network_error", "request_failed")
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadGateway, errMsg: message, retryable: true}
	}
	defer resp.Body.Close()

	usage := e.captureCodexUsage(acc, resp.Header)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := readUpstreamError(resp.Body)
		result := classifyUpstreamHTTPError(resp.StatusCode, resp.Header, msg, acc, usage)
		e.logForward(acc, meta, resp.StatusCode, 0, 0, time.Since(start), msg, result.errorKind, result.terminalEvent)
		return result
	}

	_ = e.store.TouchAccount(acc.ID)

	// Stream and translate.
	if chatReq.Stream {
		return e.streamResponse(ctx, w, resp.Body, chatReq, acc, start, meta)
	}
	return e.aggregateResponse(ctx, w, resp.Body, chatReq, acc, start, meta)
}

func shouldFailoverUpstreamError(result forwardResult) bool {
	return result.outcome == outcomeUpstreamError && !result.headersWritten &&
		(result.retryable || result.status >= http.StatusInternalServerError)
}

func isNetworkOrProxyError(err error) bool {
	if errors.Is(err, errBoundProxyUnavailable) {
		return true
	}
	var transportErr *apptransport.Error
	if errors.As(err, &transportErr) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func (e *Engine) forceRefreshAccount(ctx context.Context, acc *store.Account, cfg store.Settings) (*store.Account, error) {
	_ = cfg
	if acc.AccountType == store.AccountTypeAPIKey {
		return nil, errors.New("API-key accounts cannot refresh OAuth tokens")
	}
	proxy, err := e.proxyForAccount(acc)
	if err != nil {
		return nil, err
	}
	client, err := newHTTPClient(proxy, "standard", 60*time.Second)
	if err != nil {
		return nil, err
	}
	if err := e.accounts.Refresh(ctx, client, acc.ID); err != nil {
		return nil, err
	}
	return e.store.GetAccount(acc.ID)
}

func (e *Engine) proxyForAccount(acc *store.Account) (*store.Proxy, error) {
	if acc == nil || acc.ProxyID == nil {
		return nil, nil
	}
	proxy, err := e.store.GetProxy(*acc.ProxyID)
	if err != nil {
		e.logger.Warn("bound proxy unavailable", "account_id", acc.ID, "proxy_id", *acc.ProxyID)
		return nil, fmt.Errorf("%w", errBoundProxyUnavailable)
	}
	return proxy, nil
}

func (e *Engine) logForward(acc *store.Account, meta forwardMeta, status, prompt, completion int, latency time.Duration, message, kind, terminal string) {
	e.logForwardUsage(acc, meta, status, usageMetrics{Prompt: prompt, Completion: completion}, latency, message, kind, terminal)
}

func (e *Engine) logForwardUsage(acc *store.Account, meta forwardMeta, status int, usage usageMetrics, latency time.Duration, message, kind, terminal string) {
	e.logRequestWithDetails(acc, requestLogDetails{
		RequestID: meta.RequestID, RequestedModel: meta.RequestedModel, ResolvedModel: meta.ResolvedModel,
		Status: status, Prompt: usage.Prompt, Cached: usage.Cached, Completion: usage.Completion, Reasoning: usage.Reasoning, Estimated: usage.Estimated, Attempts: meta.Attempt,
		Latency: latency, Stream: meta.Stream, Error: message, ErrorKind: kind, TerminalEvent: terminal,
	})
}

func readUpstreamError(body io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(body, 64<<10))
	s := strings.TrimSpace(redact.Sanitize(string(data)))
	if s == "" {
		return "upstream returned an error"
	}
	if len(s) > 800 {
		s = s[:800]
	}
	return s
}

func (e *Engine) selectAccounts(ctx context.Context) ([]*store.Account, func(), error) {
	cfg := e.settings()
	if cfg.AutoRecovery {
		_ = e.store.RecoverExpiredRateLimits(time.Now())
	}
	all, err := e.store.ListAccounts()
	if err != nil {
		return nil, func() {}, err
	}
	received, err := e.store.ListActiveCloudReceivedAccounts()
	if err != nil {
		return nil, func() {}, err
	}
	all = append(all, received...)
	now := time.Now()
	relayUID := relayAccountUID(ctx)
	var out []*store.Account
	for _, a := range all {
		if relayUID != "" && a.ClientUID != relayUID {
			continue
		}
		if a.Status == store.AccountDisabled {
			continue
		}
		if a.Status == store.AccountRateLimited && a.RateLimitedUntil != nil && now.Before(*a.RateLimitedUntil) {
			continue
		}
		if a.Status == store.AccountRefreshFailed && a.NextRetryAt != nil && now.Before(*a.NextRetryAt) {
			continue
		}
		if a.AccountType == store.AccountTypeAPIKey {
			if strings.TrimSpace(a.APIKey) == "" {
				continue
			}
		} else if strings.TrimSpace(a.AccessToken) == "" && strings.TrimSpace(a.RefreshToken) == "" {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return out, func() {}, nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, accountQueueWaitTimeout)
	defer cancel()
	ordered, release, err := e.scheduler.OrderAndWait(waitCtx, out, cfg.AccountStrategy)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return nil, func() {}, errAccountQueueTimeout
	}
	return ordered, release, err
}

func (e *Engine) AccountRuntime(accountID int64) (int, int) {
	return e.scheduler.Runtime(accountID)
}

func (e *Engine) recordAccountAuthFailure(accountID int64, message string) {
	if isSevereAccountFailure(message) {
		_ = e.store.RecordSevereAccountFailure(accountID, "auto_disabled_account_inactive")
		return
	}
	_ = e.store.RecordAccountFailure(accountID, message)
}

func (e *Engine) recordAccountAuthFailureFor(account *store.Account, message, errorKind string) {
	if account == nil {
		return
	}
	if account.Source == "cloud_share" && account.CloudUserID > 0 && account.CloudGrantID != "" {
		reason := strings.TrimSpace(errorKind)
		if reason == "" {
			reason = message
		}
		_ = e.store.SetCloudReceivedAccountHealth(account.CloudUserID, account.CloudGrantID, false, false, reason)
		return
	}
	if errorKind == "share_access_revoked" {
		_ = e.store.SetAccountStatus(account.ID, store.AccountDisabled, "share_access_revoked")
		return
	}
	e.recordAccountAuthFailure(account.ID, message)
}

func (e *Engine) recordAccountSuccessFor(account *store.Account) {
	if account == nil {
		return
	}
	if account.Source == "cloud_share" && account.CloudUserID > 0 && account.CloudGrantID != "" {
		_ = e.store.UpdateCloudReceivedAccountHealth(account.CloudUserID, account.CloudGrantID, true, "")
		return
	}
	_ = e.store.RecordAccountSuccess(account.ID)
}

func (e *Engine) recordAccountTestSuccessFor(account *store.Account) {
	if account == nil {
		return
	}
	if account.Source == "cloud_share" && account.CloudUserID > 0 && account.CloudGrantID != "" {
		_ = e.store.UpdateCloudReceivedAccountHealth(account.CloudUserID, account.CloudGrantID, true, "")
		return
	}
	_ = e.store.RecordAccountTestSuccess(account.ID)
}

func (e *Engine) recordAccountRateLimitFor(account *store.Account, until time.Time, reason string) {
	if account == nil {
		return
	}
	if account.Source == "cloud_share" && account.CloudUserID > 0 && account.CloudGrantID != "" {
		_ = e.store.UpdateCloudReceivedAccountHealth(account.CloudUserID, account.CloudGrantID, false, reason)
		return
	}
	_ = e.store.SetRateLimited(account.ID, until, reason)
}

func (e *Engine) currentAccountState(account *store.Account) *store.Account {
	if account == nil {
		return nil
	}
	var updated *store.Account
	var err error
	if account.Source == "cloud_share" {
		updated, err = e.store.GetCloudReceivedAccount(account.ID)
	} else {
		updated, err = e.store.GetAccount(account.ID)
	}
	if err != nil {
		return account
	}
	return updated
}

func isSevereAccountFailure(message string) bool {
	message = strings.ToLower(message)
	for _, marker := range []string{
		"personal access token owner is inactive",
		"biscuit_baker_service_auth_credential_error_status",
		"account owner is inactive",
		"account has been deactivated",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// resolveModel returns the model to forward: the configured default when the
// request omits a model, or the request's model when it belongs to the
// gpt-5*/codex families. Anything else is rejected (no silent fallback).
func resolveModel(model, def string) (string, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return def, true
	}
	if strings.HasPrefix(m, "gpt-5") || strings.Contains(m, "codex") {
		return model, true
	}
	return "", false
}

// upstreamLogModel is the model name stored in request logs: the canonical
// upstream model actually called, with any reasoning-effort suffix kept
// (e.g. gpt-5 → gpt-5.4, gpt-5.3-high → gpt-5.3-codex-high).
func upstreamLogModel(model string) string {
	upstream, effort := openai.MapCodexModel(model)
	if effort != "" {
		return upstream + "-" + effort
	}
	return upstream
}

// applyAntiBan injects instructions and forces store=false.
func applyAntiBan(req *apicompat.ResponsesRequest, model string, cfg store.Settings) {
	storeFalse := false
	req.Store = &storeFalse
	if cfg.InjectInstr && strings.TrimSpace(req.Instructions) == "" {
		req.Instructions = openai.CodexBaseInstructionsForModel(model)
	}
	req.Stream = true
}

// setCodexHeaders disguises the request as the official Codex CLI.
func setCodexHeaders(req *http.Request, token string, acc *store.Account, cfg store.Settings) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if acc.AccountType != store.AccountTypeAPIKey && acc.ChatGPTAccountID != "" {
		req.Header.Set("chatgpt-account-id", acc.ChatGPTAccountID)
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = store.DefaultUserAgent
	}
	req.Header.Set("User-Agent", ua)
	originator := cfg.Originator
	if originator == "" {
		originator = "codex_cli_rs"
	}
	req.Header.Set("originator", originator)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("session_id", uuid.NewString())
}

func (e *Engine) logRequest(acc *store.Account, model string, status, prompt, completion int, latency time.Duration, stream bool, errMsg string) {
	e.logRequestWithDetails(acc, requestLogDetails{ResolvedModel: model, Status: status, Prompt: prompt, Completion: completion, Latency: latency, Stream: stream, Error: errMsg})
}

type requestLogDetails struct {
	RequestID, RequestedModel, ResolvedModel string
	Status, Prompt, Cached, Completion       int
	Reasoning, Attempts                      int
	Latency                                  time.Duration
	Stream, Estimated                        bool
	Error, ErrorKind, TerminalEvent          string
}

func (e *Engine) logRequestWithDetails(acc *store.Account, detail requestLogDetails) {
	var accID *int64
	email := ""
	if acc != nil {
		accID = &acc.ID
		email = acc.Email
	}
	_ = e.store.InsertLog(&store.RequestLog{
		AccountID:        accID,
		AccountEmail:     email,
		Model:            detail.ResolvedModel,
		StatusCode:       detail.Status,
		PromptTokens:     detail.Prompt,
		CachedTokens:     detail.Cached,
		CompletionTokens: detail.Completion,
		ReasoningTokens:  detail.Reasoning,
		TotalTokens:      detail.Prompt + detail.Completion,
		Estimated:        detail.Estimated,
		LatencyMS:        detail.Latency.Milliseconds(),
		Stream:           detail.Stream,
		Error:            detail.Error,
		RequestID:        detail.RequestID,
		RequestedModel:   detail.RequestedModel,
		ResolvedModel:    detail.ResolvedModel,
		ErrorKind:        detail.ErrorKind,
		AttemptCount:     detail.Attempts,
		TerminalEvent:    detail.TerminalEvent,
	})
}

// scanSSE returns a bufio.Scanner configured for large SSE lines.
func scanSSE(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return sc
}

func parseSSEEvent(line string) (*apicompat.ResponsesStreamEvent, bool) {
	if !strings.HasPrefix(line, "data:") {
		return nil, false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" || payload == "[DONE]" {
		return nil, false
	}
	var evt apicompat.ResponsesStreamEvent
	if err := json.Unmarshal([]byte(payload), &evt); err != nil {
		return nil, false
	}
	if evt.Type == "" {
		return nil, false
	}
	return &evt, true
}

var _ = fmt.Sprintf
