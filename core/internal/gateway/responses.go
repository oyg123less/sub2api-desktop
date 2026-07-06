package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

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

	cfg := e.settings()
	requestedModel, _ := body["model"].(string)
	model := normalizeModel(requestedModel, cfg.DefaultModel)
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

	candidates, err := e.selectAccounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account lookup failed: "+err.Error(), "internal_error")
		return
	}
	if len(candidates) == 0 {
		writeError(w, http.StatusServiceUnavailable, "没有可用账号，请先在应用内登录 ChatGPT 账号", "no_account")
		return
	}

	var lastErr string
	for _, acc := range candidates {
		result := e.forwardResponsesOnce(r.Context(), w, upstreamBody, requestedModel, acc, cfg)
		switch result.outcome {
		case outcomeSuccess:
			return
		case outcomeRateLimited:
			retry := result.retryAfter
			if retry <= 0 {
				retry = 10 * time.Minute
			}
			_ = e.store.SetRateLimited(acc.ID, time.Now().Add(retry))
			lastErr = result.errMsg
			continue
		case outcomeAuthFailed:
			_ = e.store.SetAccountStatus(acc.ID, store.AccountRefreshFailed, result.errMsg)
			lastErr = result.errMsg
			continue
		case outcomeClientClosed:
			return
		default:
			if !result.headersWritten {
				writeError(w, result.status, result.errMsg, "upstream_error")
			}
			return
		}
	}

	if lastErr == "" {
		lastErr = "所有账号均不可用（限额或需要重新登录）"
	}
	writeError(w, http.StatusServiceUnavailable, lastErr, "all_accounts_unavailable")
}

func (e *Engine) forwardResponsesOnce(ctx context.Context, w http.ResponseWriter, upstreamBody []byte, requestedModel string, acc *store.Account, cfg store.Settings) forwardResult {
	start := time.Now()

	var proxy *store.Proxy
	if acc.ProxyID != nil {
		if p, err := e.store.GetProxy(*acc.ProxyID); err == nil {
			proxy = p
		}
	}
	client, err := newHTTPClient(proxy, cfg.TLSFingerprint, 10*time.Minute)
	if err != nil {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error()}
	}
	authClient, _ := newHTTPClient(proxy, false, 60*time.Second)

	token, err := e.accounts.ValidAccessToken(ctx, authClient, acc)
	if err != nil {
		return forwardResult{outcome: outcomeAuthFailed, errMsg: "token refresh failed: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, UpstreamURL(), bytes.NewReader(upstreamBody))
	if err != nil {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: err.Error()}
	}
	setCodexHeaders(req, token, acc, cfg)

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return forwardResult{outcome: outcomeClientClosed}
		}
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusBadGateway, errMsg: "upstream request failed: " + err.Error()}
	}
	defer resp.Body.Close()

	usage := e.captureCodexUsage(acc, resp.Header)

	if resp.StatusCode == http.StatusTooManyRequests {
		e.logRequest(acc, requestedModel, resp.StatusCode, 0, 0, time.Since(start), true, "rate limited (429)")
		return forwardResult{outcome: outcomeRateLimited, status: resp.StatusCode, errMsg: "账号已限额（429）", retryAfter: rateLimitRetryAfter(resp.Header, usage)}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msg := readUpstreamError(resp.Body)
		e.logRequest(acc, requestedModel, resp.StatusCode, 0, 0, time.Since(start), true, msg)
		return forwardResult{outcome: outcomeAuthFailed, status: resp.StatusCode, errMsg: "鉴权失败: " + msg}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := readUpstreamError(resp.Body)
		e.logRequest(acc, requestedModel, resp.StatusCode, 0, 0, time.Since(start), true, msg)
		return forwardResult{outcome: outcomeUpstreamError, status: resp.StatusCode, errMsg: msg}
	}

	_ = e.store.TouchAccount(acc.ID)

	return e.relayResponsesSSE(w, resp.Body, requestedModel, acc, start)
}

// relayResponsesSSE copies the upstream Responses SSE stream to the client
// verbatim while extracting token usage from response.completed for logging.
func (e *Engine) relayResponsesSSE(w http.ResponseWriter, body io.Reader, model string, acc *store.Account, start time.Time) forwardResult {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: "streaming unsupported"}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	var usage *apicompat.ResponsesUsage
	sc := scanSSE(body)
	for sc.Scan() {
		line := sc.Text()
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return forwardResult{outcome: outcomeClientClosed, headersWritten: true}
		}
		if line == "" {
			flusher.Flush()
			continue
		}
		if evt, ok := parseSSEEvent(line); ok && evt.Response != nil && evt.Response.Usage != nil {
			usage = evt.Response.Usage
		}
	}
	flusher.Flush()

	prompt, completion := 0, 0
	if usage != nil {
		prompt, completion = usage.InputTokens, usage.OutputTokens
	}
	e.logRequest(acc, model, http.StatusOK, prompt, completion, time.Since(start), true, "")
	return forwardResult{outcome: outcomeSuccess, headersWritten: true}
}
