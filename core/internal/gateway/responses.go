package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"sub2api-desktop/core/internal/apicompat"
	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

// Responses handles POST /v1/responses: a native Responses API passthrough
// used by Codex CLI (wire_api = "responses"). The request body is forwarded
// to the ChatGPT Codex backend with model mapping and anti-ban disguise
// applied, and the upstream SSE stream is relayed back verbatim.
func (e *Engine) Responses(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error")
		return
	}
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error(), "invalid_request_error")
		return
	}
	for _, key := range []string{"stop", "n", "logprobs", "top_logprobs", "audio", "modalities"} {
		delete(body, key)
	}

	cfg := e.settings()
	requestedModel, _ := body["model"].(string)
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = uuid.NewString()
	}
	clientStream, _ := body["stream"].(bool)
	model, ok := resolveModel(requestedModel, cfg.DefaultModel)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown model: "+requestedModel+"（仅支持 gpt-5*/codex 系列模型）", "invalid_request_error")
		return
	}
	logModel := upstreamLogModel(model)
	upstreamModel, effort := openai.MapCodexModel(model)
	body["model"] = upstreamModel
	if effort != "" {
		if _, has := body["reasoning"]; !has {
			body["reasoning"] = map[string]any{"effort": effort, "summary": "auto"}
		} else if rm, ok := body["reasoning"].(map[string]any); ok {
			if s, _ := rm["effort"].(string); strings.TrimSpace(s) == "" {
				rm["effort"] = effort
			}
		}
	}
	// Anti-ban: never store, inject Codex instructions when absent.
	body["store"] = false
	if cfg.InjectInstr {
		if s, _ := body["instructions"].(string); strings.TrimSpace(s) == "" {
			body["instructions"] = openai.CodexBaseInstructionsForModel(upstreamModel)
		}
	}
	body["stream"] = true

	upstreamBody, err := json.Marshal(body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "internal_error")
		return
	}

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
		meta := forwardMeta{RequestID: requestID, RequestedModel: requestedModel, ResolvedModel: logModel, Attempt: attempt, Stream: clientStream, EstimatedPromptTokens: estimateTokensFromBytes(len(bodyBytes))}
		result := e.forwardResponsesOnce(r.Context(), w, upstreamBody, acc, cfg, meta)
		if result.outcome == outcomeAuthFailed && acc.AccountType == store.AccountTypeOAuth && acc.RefreshToken != "" {
			if refreshed, err := e.forceRefreshAccount(r.Context(), acc, cfg); err == nil {
				acc = refreshed
				attempt++
				meta.Attempt = attempt
				result = e.forwardResponsesOnce(r.Context(), w, upstreamBody, acc, cfg, meta)
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
			e.recordAccountRateLimitFor(acc, time.Now().Add(retry), result.statusReason)
			lastErr = result.errMsg
			lastStatus = 0
			continue
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

func (e *Engine) forwardResponsesOnce(ctx context.Context, w http.ResponseWriter, upstreamBody []byte, acc *store.Account, cfg store.Settings, meta forwardMeta) forwardResult {
	start := time.Now()

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

	if meta.Stream {
		return e.relayResponsesSSE(ctx, w, resp.Body, acc, start, meta)
	}
	return e.aggregateResponsesJSON(ctx, w, resp.Body, acc, start, meta)
}

// relayResponsesSSE copies the upstream Responses SSE stream to the client
// verbatim while extracting token usage from response.completed for logging.
func (e *Engine) relayResponsesSSE(ctx context.Context, w http.ResponseWriter, body io.Reader, acc *store.Account, start time.Time, meta forwardMeta) forwardResult {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: "streaming unsupported"}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	var usage *apicompat.ResponsesUsage
	outputBytes := 0
	terminal := &sseTerminal{}
	sc := scanSSE(body)
	for sc.Scan() {
		line := sc.Text()
		eventBytes := 0
		if evt, ok := parseSSEEvent(line); ok {
			terminal.observe(evt)
			eventBytes = responseEventOutputBytes(evt)
			if terminal.usage != nil {
				usage = terminal.usage
			}
			if terminal.errorKind != "" {
				writeStreamError(w, terminal.errorKind, terminal.message)
				flusher.Flush()
				metrics := bestUsage(usage, nil, meta, outputBytes)
				e.logForwardUsage(acc, meta, terminal.status, metrics, time.Since(start), terminal.message, terminal.errorKind, terminal.event)
				return forwardResult{outcome: outcomeUpstreamError, status: terminal.status, errMsg: terminal.message, headersWritten: true}
			}
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			metrics := bestUsage(usage, nil, meta, outputBytes)
			e.logForwardUsage(acc, meta, 499, metrics, time.Since(start), "client cancelled stream", "client_cancelled", "client_cancelled")
			return forwardResult{outcome: outcomeClientClosed, headersWritten: true}
		}
		outputBytes += eventBytes
		if line == "" {
			flusher.Flush()
		}
	}
	if ctx.Err() != nil || errors.Is(sc.Err(), context.Canceled) {
		metrics := bestUsage(usage, nil, meta, outputBytes)
		e.logForwardUsage(acc, meta, 499, metrics, time.Since(start), "client cancelled stream", "client_cancelled", "client_cancelled")
		return forwardResult{outcome: outcomeClientClosed, headersWritten: true}
	}
	if err := terminal.finish(sc.Err()); err != nil {
		writeStreamError(w, terminal.errorKind, terminal.message)
		flusher.Flush()
		metrics := bestUsage(usage, nil, meta, outputBytes)
		e.logForwardUsage(acc, meta, terminal.status, metrics, time.Since(start), terminal.message, terminal.errorKind, terminal.event)
		return forwardResult{outcome: outcomeUpstreamError, status: terminal.status, errMsg: terminal.message, headersWritten: true}
	}
	flusher.Flush()

	metrics := bestUsage(usage, nil, meta, outputBytes)
	e.logForwardUsage(acc, meta, http.StatusOK, metrics, time.Since(start), "", "", terminal.event)
	return forwardResult{outcome: outcomeSuccess, headersWritten: true}
}

func (e *Engine) aggregateResponsesJSON(ctx context.Context, w http.ResponseWriter, body io.Reader, acc *store.Account, start time.Time, meta forwardMeta) forwardResult {
	terminal := &sseTerminal{}
	accumulator := apicompat.NewBufferedResponseAccumulator()
	outputBytes := 0
	sc := scanSSE(body)
	for sc.Scan() {
		evt, ok := parseSSEEvent(sc.Text())
		if !ok {
			continue
		}
		terminal.observe(evt)
		outputBytes += responseEventOutputBytes(evt)
		if terminal.errorKind != "" {
			break
		}
		accumulator.ProcessEvent(evt)
	}
	if ctx.Err() != nil || errors.Is(sc.Err(), context.Canceled) {
		metrics := bestUsage(terminal.usage, nil, meta, outputBytes)
		e.logForwardUsage(acc, meta, 499, metrics, time.Since(start), "client cancelled request", "client_cancelled", "client_cancelled")
		return forwardResult{outcome: outcomeClientClosed}
	}
	if err := terminal.finish(sc.Err()); err != nil {
		metrics := bestUsage(terminal.usage, nil, meta, outputBytes)
		e.logForwardUsage(acc, meta, terminal.status, metrics, time.Since(start), terminal.message, terminal.errorKind, terminal.event)
		writeError(w, terminal.status, terminal.message, terminal.errorKind)
		return forwardResult{outcome: outcomeUpstreamError, status: terminal.status, errMsg: terminal.message, headersWritten: true}
	}

	resp := terminal.response
	if resp == nil {
		resp = &apicompat.ResponsesResponse{Status: "completed"}
	}
	if resp.Object == "" {
		resp.Object = "response"
	}
	if resp.Model == "" {
		resp.Model = meta.ResolvedModel
	}
	if resp.Usage == nil {
		resp.Usage = terminal.usage
	}
	accumulator.SupplementResponseOutput(resp)
	writeJSON(w, http.StatusOK, resp)
	metrics := bestUsage(resp.Usage, nil, meta, outputBytes)
	e.logForwardUsage(acc, meta, http.StatusOK, metrics, time.Since(start), "", "", terminal.event)
	return forwardResult{outcome: outcomeSuccess, headersWritten: true}
}

func responsesUsageCounts(usage *apicompat.ResponsesUsage) (int, int) {
	if usage == nil {
		return 0, 0
	}
	return usage.InputTokens, usage.OutputTokens
}
