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
	"sort"
	"strings"
	"sync"
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

func isStandardOpenAICompatible(acc *store.Account, model string) bool {
	if acc == nil || acc.AccountType != store.AccountTypeAPIKey {
		return false
	}
	if acc.Source == "cloud_share" {
		return false
	}
	base := strings.ToLower(strings.TrimSpace(acc.BaseURL))
	if strings.HasSuffix(base, "/responses") {
		return false
	}
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(base, "deepseek") || strings.Contains(m, "deepseek") {
		return true
	}
	if strings.Contains(base, "moonshot") || strings.Contains(m, "moonshot") || strings.Contains(m, "kimi") {
		return true
	}
	if strings.Contains(base, "zhipu") || strings.Contains(base, "bigmodel") || strings.Contains(m, "glm") {
		return true
	}
	if strings.Contains(base, "minimax") || strings.Contains(m, "abab") {
		return true
	}
	if m != "" && !strings.HasPrefix(m, "gpt-") && !strings.Contains(m, "codex") && !strings.HasPrefix(m, "o1") && !strings.HasPrefix(m, "o3") && !strings.HasPrefix(m, "o4") && !strings.HasPrefix(m, "chatgpt") {
		return true
	}
	return false
}

func apiKeyChatCompletionsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if normalized == "" {
		return "https://api.openai.com/v1/chat/completions"
	}
	if strings.HasSuffix(normalized, "/chat/completions") {
		return normalized
	}
	segment := normalized
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		segment = normalized[idx+1:]
	}
	if isVersionSegment(segment) {
		return normalized + "/chat/completions"
	}
	return normalized + "/v1/chat/completions"
}

func apiKeyModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if normalized == "" {
		return "https://api.openai.com/v1/models"
	}
	if strings.HasSuffix(normalized, "/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/chat/completions") {
		normalized = strings.TrimSuffix(normalized, "/chat/completions")
	} else if strings.HasSuffix(normalized, "/responses") {
		normalized = strings.TrimSuffix(normalized, "/responses")
	}
	segment := normalized
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		segment = normalized[idx+1:]
	}
	if isVersionSegment(segment) {
		return normalized + "/models"
	}
	return normalized + "/v1/models"
}

// Engine holds the dependencies for request forwarding.
type Engine struct {
	store              *store.Store
	accounts           *account.Manager
	settings           func() store.Settings
	logger             *slog.Logger
	scheduler          *Scheduler
	accountModelsCache sync.Map
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
	clientHeaders := upstreamClientHeadersFromRequest(r, cfg)
	requestedModel := chatReq.Model
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = uuid.NewString()
	}
	model, ok := resolveModel(requestedModel, cfg.DefaultModel)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown model: "+requestedModel, "invalid_request_error")
		return
	}
	chatReq.Model = model
	logModel := upstreamLogModel(model)

	candidates, releaseFirst, err := e.selectAccounts(r.Context(), model)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		writeAccountSelectionError(w, err)
		return
	}
	if len(candidates) == 0 {
		writeError(w, http.StatusServiceUnavailable, "没有可用账号或没有支持该模型的账号，请检查号池中对应账号是否已启用", "no_account")
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
		meta := forwardMeta{RequestID: requestID, RequestedModel: requestedModel, ResolvedModel: logModel, Attempt: attempt, Stream: chatReq.Stream, EstimatedPromptTokens: estimateTokensFromBytes(len(bodyBytes)), ClientHeaders: clientHeaders}
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
	ClientHeaders                            upstreamClientHeaders
}

var errBoundProxyUnavailable = errors.New("bound proxy is unavailable")

func (e *Engine) forwardOnce(ctx context.Context, w http.ResponseWriter, chatReq *apicompat.ChatCompletionsRequest, acc *store.Account, cfg store.Settings, meta forwardMeta) forwardResult {
	if isStandardOpenAICompatible(acc, chatReq.Model) {
		return e.forwardStandardChatCompletions(ctx, w, chatReq, acc, cfg, meta)
	}

	start := time.Now()

	// Resolve proxy + client.
	proxy, err := e.proxyForAccount(acc)
	if err != nil {
		message := errBoundProxyUnavailable.Error()
		e.logForward(acc, meta, http.StatusBadGateway, 0, 0, time.Since(start), message, "proxy_unavailable", "proxy_resolution_failed")
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadGateway, errMsg: message, retryable: true}
	}
	client, err := newHTTPClient(proxy, acc.NetworkMode, cfg.CompatProfile, 10*time.Minute)
	if err != nil {
		e.logForward(acc, meta, http.StatusInternalServerError, 0, 0, time.Since(start), err.Error(), "upstream_network_error", "client_setup_failed")
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error(), retryable: true}
	}
	authClient, _ := newHTTPClient(proxy, acc.NetworkMode, "standard", 60*time.Second)

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
	setCodexHeaders(req, token, acc, cfg, meta.ClientHeaders)

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

func (e *Engine) forwardStandardChatCompletions(ctx context.Context, w http.ResponseWriter, chatReq *apicompat.ChatCompletionsRequest, acc *store.Account, cfg store.Settings, meta forwardMeta) forwardResult {
	start := time.Now()
	proxy, err := e.proxyForAccount(acc)
	if err != nil {
		message := errBoundProxyUnavailable.Error()
		e.logForward(acc, meta, http.StatusBadGateway, 0, 0, time.Since(start), message, "proxy_unavailable", "proxy_resolution_failed")
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadGateway, errMsg: message, retryable: true}
	}
	client, err := newHTTPClient(proxy, acc.NetworkMode, cfg.CompatProfile, 10*time.Minute)
	if err != nil {
		e.logForward(acc, meta, http.StatusInternalServerError, 0, 0, time.Since(start), err.Error(), "upstream_network_error", "client_setup_failed")
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error(), retryable: true}
	}

	upstreamModel, effort := openai.MapCodexModel(chatReq.Model)
	reqCopy := *chatReq
	if upstreamModel != "" {
		reqCopy.Model = upstreamModel
	}
	if effort != "" && reqCopy.ReasoningEffort == "" {
		reqCopy.ReasoningEffort = effort
	}

	bodyBytes, err := json.Marshal(&reqCopy)
	if err != nil {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error()}
	}

	url := apiKeyChatCompletionsURL(acc.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error()}
	}
	token := acc.APIKey
	if strings.TrimSpace(token) == "" {
		token = acc.AccessToken
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if reqCopy.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}

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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := readUpstreamError(resp.Body)
		result := classifyUpstreamHTTPError(resp.StatusCode, resp.Header, msg, acc, nil)
		e.logForward(acc, meta, resp.StatusCode, 0, 0, time.Since(start), msg, result.errorKind, result.terminalEvent)
		return result
	}

	_ = e.store.TouchAccount(acc.ID)

	if reqCopy.Stream {
		return e.relayChatCompletionsSSE(ctx, w, resp.Body, acc, start, meta)
	}
	return e.relayChatCompletionsJSON(ctx, w, resp.Body, acc, start, meta)
}

func (e *Engine) relayChatCompletionsSSE(ctx context.Context, w http.ResponseWriter, body io.Reader, acc *store.Account, start time.Time, meta forwardMeta) forwardResult {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: "streaming unsupported"}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sc := scanSSE(body)
	outputBytes := 0
	var lastUsage *apicompat.ChatUsage

	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		if _, err := io.WriteString(w, line+"\n\n"); err != nil {
			e.logForward(acc, meta, 499, 0, 0, time.Since(start), "client cancelled stream", "client_cancelled", "client_cancelled")
			return forwardResult{outcome: outcomeClientClosed, headersWritten: true}
		}
		flusher.Flush()
		outputBytes += len(line)
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload != "" && payload != "[DONE]" {
			var chunk apicompat.ChatCompletionsChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err == nil && chunk.Usage != nil {
				lastUsage = chunk.Usage
			}
		}
	}
	usage := usageMetrics{Prompt: meta.EstimatedPromptTokens, Completion: outputBytes / 4}
	if lastUsage != nil {
		usage.Prompt = lastUsage.PromptTokens
		usage.Completion = lastUsage.CompletionTokens
	}
	e.logForwardUsage(acc, meta, http.StatusOK, usage, time.Since(start), "", "", "")
	return forwardResult{outcome: outcomeSuccess, status: http.StatusOK, headersWritten: true}
}

func (e *Engine) relayChatCompletionsJSON(ctx context.Context, w http.ResponseWriter, body io.Reader, acc *store.Account, start time.Time, meta forwardMeta) forwardResult {
	data, err := io.ReadAll(body)
	if err != nil {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadGateway, errMsg: "read response body: " + err.Error()}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)

	var chatResp apicompat.ChatCompletionsResponse
	usage := usageMetrics{Prompt: meta.EstimatedPromptTokens, Completion: len(data) / 4}
	if err := json.Unmarshal(data, &chatResp); err == nil && chatResp.Usage != nil {
		usage.Prompt = chatResp.Usage.PromptTokens
		usage.Completion = chatResp.Usage.CompletionTokens
	}
	e.logForwardUsage(acc, meta, http.StatusOK, usage, time.Since(start), "", "", "")
	return forwardResult{outcome: outcomeSuccess, status: http.StatusOK, headersWritten: true}
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
	client, err := newHTTPClient(proxy, acc.NetworkMode, "standard", 60*time.Second)
	if err != nil {
		return nil, err
	}
	if err := e.accounts.Refresh(ctx, client, acc.ID); err != nil {
		return nil, err
	}
	return e.store.GetAccount(acc.ID)
}

func (e *Engine) proxyForAccount(acc *store.Account) (*store.Proxy, error) {
	if acc == nil {
		return nil, nil
	}
	mode := store.ResolveAccountNetworkMode(acc.NetworkMode, acc.ProxyID)
	acc.NetworkMode = mode
	if mode != store.AccountNetworkProxy {
		return nil, nil
	}
	if acc.ProxyID == nil {
		return nil, fmt.Errorf("%w", errBoundProxyUnavailable)
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

func (e *Engine) selectAccounts(ctx context.Context, model string) ([]*store.Account, func(), error) {
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
		if model != "" && !e.accountSupportsModel(a, model) {
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
// request omits a model, or the request's model when non-empty.
func resolveModel(model, def string) (string, bool) {
	m := strings.TrimSpace(model)
	if m == "" {
		if strings.TrimSpace(def) == "" {
			return openai.DefaultGatewayModel, true
		}
		return def, true
	}
	if strings.ContainsAny(m, " \t\r\n\"'") {
		return "", false
	}
	return m, true
}

func (e *Engine) accountSupportsModel(acc *store.Account, requestedModel string) bool {
	if acc == nil {
		return false
	}
	baseModel := strings.ToLower(strings.TrimSpace(openai.StripReasoningEffortSuffix(requestedModel)))
	if baseModel == "" {
		return true
	}

	if cached, ok := e.accountModelsCache.Load(acc.ID); ok {
		if models, ok := cached.([]string); ok && len(models) > 0 {
			for _, m := range models {
				if strings.EqualFold(m, baseModel) {
					return true
				}
			}
			return false
		}
	}

	for _, m := range store.AccountProviderModels(acc) {
		if strings.EqualFold(m, baseModel) {
			return true
		}
	}

	if acc.AccountType == store.AccountTypeOAuth {
		return strings.HasPrefix(baseModel, "gpt-") ||
			strings.Contains(baseModel, "codex") ||
			strings.HasPrefix(baseModel, "o1") ||
			strings.HasPrefix(baseModel, "o3") ||
			strings.HasPrefix(baseModel, "o4") ||
			strings.HasPrefix(baseModel, "chatgpt")
	}

	base := strings.ToLower(strings.TrimSpace(acc.BaseURL))
	if strings.Contains(base, "deepseek") {
		return strings.Contains(baseModel, "deepseek")
	}
	if strings.Contains(base, "moonshot") {
		return strings.HasPrefix(baseModel, "moonshot") || strings.HasPrefix(baseModel, "kimi")
	}
	if strings.Contains(base, "zhipu") || strings.Contains(base, "bigmodel") {
		return strings.HasPrefix(baseModel, "glm")
	}
	if strings.Contains(base, "minimax") {
		return strings.HasPrefix(baseModel, "abab")
	}
	if strings.Contains(base, "openai.com") || base == "" {
		return strings.HasPrefix(baseModel, "gpt-") ||
			strings.HasPrefix(baseModel, "o1") ||
			strings.HasPrefix(baseModel, "o3") ||
			strings.HasPrefix(baseModel, "o4") ||
			strings.Contains(baseModel, "codex")
	}

	return true
}

// ActiveModels returns the unique list of model IDs supported by all currently
// active accounts in the pool, plus any configured custom models.
func (e *Engine) ActiveModels() []string {
	if e == nil || e.store == nil {
		return nil
	}
	all, err := e.store.ListAccounts()
	if err != nil {
		all = nil
	}
	received, err := e.store.ListActiveCloudReceivedAccounts()
	if err == nil {
		all = append(all, received...)
	}

	seen := make(map[string]struct{})
	var result []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}

	for _, a := range all {
		if a.Status != store.AccountActive {
			continue
		}
		if a.AccountType == store.AccountTypeAPIKey {
			if strings.TrimSpace(a.APIKey) == "" {
				continue
			}
		} else {
			if strings.TrimSpace(a.AccessToken) == "" && strings.TrimSpace(a.RefreshToken) == "" {
				continue
			}
		}

		var providerModels []string
		if cached, ok := e.accountModelsCache.Load(a.ID); ok {
			if models, ok := cached.([]string); ok && len(models) > 0 {
				providerModels = models
			}
		}
		if len(providerModels) == 0 {
			providerModels = store.AccountProviderModels(a)
		}
		for _, m := range providerModels {
			add(m)
		}
	}

	cfg := e.settings()
	if cfg.CustomModels != "" {
		for _, m := range openai.ParseModelList(cfg.CustomModels) {
			add(m)
		}
	}
	return result
}

// AccountModelsResult represents the models supported by an account and default selection.
type AccountModelsResult struct {
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model"`
	Source       string   `json:"source"`
}

// AccountAvailableModels returns models for an existing account, querying upstream /models for API-key accounts.
func (e *Engine) AccountAvailableModels(ctx context.Context, acc *store.Account) (AccountModelsResult, error) {
	if acc == nil {
		return AccountModelsResult{
			Models:       openai.ActiveModelOptions(nil),
			DefaultModel: openai.DefaultTestModel,
			Source:       "fallback",
		}, nil
	}

	if acc.AccountType == store.AccountTypeOAuth {
		models := store.AccountKnownModels(acc)
		if len(models) == 0 {
			models = openai.ActiveModelOptions(nil)
		}
		return AccountModelsResult{
			Models:       models,
			DefaultModel: openai.DefaultTestModel,
			Source:       "oauth",
		}, nil
	}

	baseLower := strings.ToLower(strings.TrimSpace(acc.BaseURL))
	if strings.Contains(baseLower, "chatgpt.com/backend-api/codex/responses") {
		return AccountModelsResult{
			Models:       store.AccountKnownModels(&store.Account{AccountType: store.AccountTypeOAuth}),
			DefaultModel: openai.DefaultTestModel,
			Source:       "oauth",
		}, nil
	}

	apiKey := strings.TrimSpace(acc.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(acc.AccessToken)
	}

	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	models, err := e.fetchUpstreamModels(probeCtx, acc.BaseURL, apiKey, acc.ProxyID, acc.NetworkMode)
	if err == nil && len(models) > 0 {
		e.accountModelsCache.Store(acc.ID, models)
		return AccountModelsResult{
			Models:       models,
			DefaultModel: pickDefaultModel(models, acc.BaseURL),
			Source:       "upstream",
		}, nil
	}

	if cached, ok := e.accountModelsCache.Load(acc.ID); ok {
		if cachedModels, ok := cached.([]string); ok && len(cachedModels) > 0 {
			return AccountModelsResult{
				Models:       cachedModels,
				DefaultModel: pickDefaultModel(cachedModels, acc.BaseURL),
				Source:       "cache",
			}, nil
		}
	}

	known := store.AccountKnownModels(acc)
	if len(known) > 0 {
		return AccountModelsResult{
			Models:       known,
			DefaultModel: pickDefaultModel(known, acc.BaseURL),
			Source:       "known",
		}, nil
	}

	return AccountModelsResult{
		Models:       openai.ActiveModelOptions(nil),
		DefaultModel: openai.DefaultTestModel,
		Source:       "fallback",
	}, nil
}

// ProbeModels queries upstream /models for an arbitrary BaseURL and APIKey.
func (e *Engine) ProbeModels(ctx context.Context, baseURL, apiKey string, proxyID *int64, networkMode store.AccountNetworkMode) (AccountModelsResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	models, err := e.fetchUpstreamModels(probeCtx, baseURL, apiKey, proxyID, networkMode)
	if err == nil && len(models) > 0 {
		return AccountModelsResult{
			Models:       models,
			DefaultModel: pickDefaultModel(models, baseURL),
			Source:       "upstream",
		}, nil
	}

	mockAcc := &store.Account{
		AccountType: store.AccountTypeAPIKey,
		BaseURL:     baseURL,
	}
	known := store.AccountKnownModels(mockAcc)
	if len(known) > 0 {
		return AccountModelsResult{
			Models:       known,
			DefaultModel: pickDefaultModel(known, baseURL),
			Source:       "known",
		}, nil
	}

	return AccountModelsResult{
		Models:       openai.ActiveModelOptions(nil),
		DefaultModel: openai.DefaultTestModel,
		Source:       "fallback",
	}, nil
}

func (e *Engine) fetchUpstreamModels(ctx context.Context, baseURL, apiKey string, proxyID *int64, networkMode store.AccountNetworkMode) ([]string, error) {
	targetURL := apiKeyModelsURL(baseURL)
	var proxy *store.Proxy
	if proxyID != nil && e.store != nil {
		proxy, _ = e.store.GetProxy(*proxyID)
	}
	cfg := e.settings()
	client, err := newHTTPClient(proxy, networkMode, cfg.CompatProfile, 10*time.Second)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	var envelope struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []any `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&envelope); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var models []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			models = append(models, id)
		}
	}

	for _, item := range envelope.Data {
		add(item.ID)
	}
	for _, item := range envelope.Models {
		if s, ok := item.(string); ok {
			add(s)
		} else if m, ok := item.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				add(id)
			}
		}
	}

	if len(models) == 0 {
		return nil, errors.New("no models found in upstream response")
	}

	sortModelsByPriority(models, baseURL)
	return models, nil
}

func pickDefaultModel(models []string, baseURL string) string {
	if len(models) == 0 {
		return openai.DefaultTestModel
	}
	baseLower := strings.ToLower(baseURL)
	if strings.Contains(baseLower, "deepseek") {
		for _, m := range models {
			if strings.EqualFold(m, "deepseek-chat") {
				return m
			}
		}
	}
	if strings.Contains(baseLower, "moonshot") || strings.Contains(baseLower, "kimi") {
		for _, m := range models {
			if strings.EqualFold(m, "moonshot-v1-8k") || strings.Contains(strings.ToLower(m), "moonshot") {
				return m
			}
		}
	}
	for _, m := range models {
		mLower := strings.ToLower(m)
		if strings.Contains(mLower, "chat") && !strings.Contains(mLower, "embed") {
			return m
		}
	}
	for _, m := range models {
		mLower := strings.ToLower(m)
		if strings.Contains(mLower, "gpt") && !strings.Contains(mLower, "embed") {
			return m
		}
	}
	return models[0]
}

func sortModelsByPriority(models []string, baseURL string) {
	sort.SliceStable(models, func(i, j int) bool {
		a := strings.ToLower(models[i])
		b := strings.ToLower(models[j])
		scoreA := modelPriorityScore(a, baseURL)
		scoreB := modelPriorityScore(b, baseURL)
		if scoreA != scoreB {
			return scoreA > scoreB
		}
		return a < b
	})
}

func modelPriorityScore(name, baseURL string) int {
	baseLower := strings.ToLower(baseURL)
	if strings.Contains(baseLower, "deepseek") {
		if name == "deepseek-chat" {
			return 100
		}
		if name == "deepseek-reasoner" {
			return 90
		}
	}
	if strings.Contains(baseLower, "moonshot") {
		if strings.HasPrefix(name, "moonshot-v1-8k") {
			return 100
		}
		if strings.Contains(name, "moonshot") {
			return 90
		}
	}
	for _, dep := range []string{"embed", "whisper", "tts", "dall-e", "moderation", "davinci", "babbage"} {
		if strings.Contains(name, dep) {
			return 10
		}
	}
	if strings.Contains(name, "chat") {
		return 80
	}
	if strings.Contains(name, "reasoner") || strings.Contains(name, "pro") || strings.Contains(name, "turbo") || strings.Contains(name, "flash") {
		return 70
	}
	return 50
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

type upstreamClientHeaders struct {
	UserAgent  string
	Originator string
	OpenAIBeta string
	SessionID  string
}

func fallbackUpstreamClientHeaders(cfg store.Settings) upstreamClientHeaders {
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = store.DefaultUserAgent
	}
	originator := strings.TrimSpace(cfg.Originator)
	if originator == "" {
		originator = "codex_cli_rs"
	}
	return upstreamClientHeaders{
		UserAgent:  userAgent,
		Originator: originator,
		OpenAIBeta: "responses=experimental",
		SessionID:  uuid.NewString(),
	}
}

func upstreamClientHeadersFromRequest(req *http.Request, cfg store.Settings) upstreamClientHeaders {
	headers := fallbackUpstreamClientHeaders(cfg)
	if req == nil {
		return headers
	}

	incomingUserAgent := strings.TrimSpace(req.UserAgent())
	incomingOriginator := strings.TrimSpace(req.Header.Get("originator"))
	officialClient := false
	if openai.IsCodexOfficialClientRequestStrict(incomingUserAgent) {
		headers.UserAgent = incomingUserAgent
		officialClient = true
	}
	if openai.IsCodexOfficialClientOriginator(incomingOriginator) {
		headers.Originator = incomingOriginator
		officialClient = true
	}
	if !officialClient {
		return headers
	}
	if value := strings.TrimSpace(req.Header.Get("OpenAI-Beta")); value != "" {
		headers.OpenAIBeta = value
	}
	if value := strings.TrimSpace(req.Header.Get("session_id")); value != "" {
		headers.SessionID = value
	}
	return headers
}

// setCodexHeaders preserves a real Codex caller identity and otherwise uses
// Amber's current compatibility fallback.
func setCodexHeaders(req *http.Request, token string, acc *store.Account, cfg store.Settings, headers upstreamClientHeaders) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if acc.AccountType != store.AccountTypeAPIKey && acc.ChatGPTAccountID != "" {
		req.Header.Set("chatgpt-account-id", acc.ChatGPTAccountID)
	}
	if headers.UserAgent == "" {
		headers = fallbackUpstreamClientHeaders(cfg)
	}
	req.Header.Set("User-Agent", headers.UserAgent)
	req.Header.Set("originator", headers.Originator)
	req.Header.Set("OpenAI-Beta", headers.OpenAIBeta)
	if headers.SessionID == "" {
		headers.SessionID = uuid.NewString()
	}
	req.Header.Set("session_id", headers.SessionID)
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
