package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"sub2api-desktop/core/internal/apicompat"
	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

// TestResult is the outcome of a single account connectivity test.
type TestResult struct {
	OK               bool   `json:"ok"`
	Status           int    `json:"status"`
	Error            string `json:"error,omitempty"`
	ErrorKind        string `json:"error_kind,omitempty"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	LatencyMS        int64  `json:"latency_ms"`
	Sample           string `json:"sample,omitempty"`
	AccountStatus    string `json:"account_status"`
}

// TestAccount sends a minimal Codex request through the exact same disguise
// pipeline (instruction injection, Codex headers, TLS fingerprint, proxy) used
// for real forwarding, so it doubles as a live check of the anti-ban path. It
// updates the account status (active / rate_limited / refresh_failed) based on
// the outcome and records the request in the usage logs.
func (e *Engine) TestAccount(ctx context.Context, acc *store.Account, model, prompt string) TestResult {
	start := time.Now()
	cfg := e.settings()

	testModel, ok := resolveModel(model, cfg.DefaultModel)
	if !ok {
		testModel = cfg.DefaultModel
	}
	if testModel == "" {
		testModel = openai.DefaultTestModel
	}
	testPrompt := strings.TrimSpace(prompt)
	if testPrompt == "" {
		testPrompt = "hi"
	}

	res := TestResult{Model: testModel, AccountStatus: string(acc.Status)}

	proxy, err := e.proxyForAccount(acc)
	if err != nil {
		res.Status = http.StatusBadGateway
		res.Error = errBoundProxyUnavailable.Error()
		res.ErrorKind = "proxy_unavailable"
		e.logRequestWithDetails(acc, requestLogDetails{
			ResolvedModel: testModel, Status: res.Status, Latency: time.Since(start), Stream: true,
			Error: res.Error, ErrorKind: "proxy_unavailable", TerminalEvent: "proxy_resolution_failed",
		})
		return res
	}
	client, err := newHTTPClient(proxy, cfg.CompatProfile, 90*time.Second)
	if err != nil {
		res.Error = err.Error()
		res.ErrorKind = "transport"
		return res
	}
	authClient, _ := newHTTPClient(proxy, "standard", 60*time.Second)

	token, err := e.accounts.ValidAccessToken(ctx, authClient, acc)
	if err != nil {
		if !isNetworkOrProxyError(err) {
			e.recordAccountAuthFailure(acc.ID, err.Error())
		}
		res.Error = "令牌刷新失败: " + err.Error()
		if isNetworkOrProxyError(err) {
			res.ErrorKind = "network"
		} else {
			res.ErrorKind = "authentication"
		}
		if updated, getErr := e.store.GetAccount(acc.ID); getErr == nil {
			res.AccountStatus = string(updated.Status)
		}
		e.logRequest(acc, testModel, http.StatusUnauthorized, 0, 0, time.Since(start), true, res.Error)
		return res
	}

	promptJSON, _ := json.Marshal(testPrompt)
	chatReq := &apicompat.ChatCompletionsRequest{
		Model:    testModel,
		Messages: []apicompat.ChatMessage{{Role: "user", Content: json.RawMessage(promptJSON)}},
		Stream:   true,
	}
	respReq, err := apicompat.ChatCompletionsToResponses(chatReq)
	if err != nil {
		res.Error = "构造请求失败: " + err.Error()
		res.ErrorKind = "local"
		return res
	}
	upstreamModel, effort := openai.MapCodexModel(testModel)
	respReq.Model = upstreamModel
	if respReq.Reasoning == nil && effort != "" {
		respReq.Reasoning = &apicompat.ResponsesReasoning{Effort: effort, Summary: "auto"}
	}
	applyAntiBan(respReq, upstreamModel, cfg)
	upstreamBody, err := json.Marshal(respReq)
	if err != nil {
		res.Error = err.Error()
		res.ErrorKind = "local"
		return res
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURLForAccount(acc), bytes.NewReader(upstreamBody))
	if err != nil {
		res.Error = err.Error()
		res.ErrorKind = "local"
		return res
	}
	setCodexHeaders(req, token, acc, cfg)

	resp, err := client.Do(req)
	if err != nil {
		res.Status = http.StatusBadGateway
		res.Error = "上游请求失败: " + err.Error()
		res.ErrorKind = "network"
		e.logRequest(acc, testModel, res.Status, 0, 0, time.Since(start), true, res.Error)
		return res
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode

	usage := e.captureCodexUsage(acc, resp.Header)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := readUpstreamError(resp.Body)
		result := classifyUpstreamHTTPError(resp.StatusCode, resp.Header, msg, acc, usage)
		switch result.outcome {
		case outcomeRateLimited:
			_ = e.store.SetRateLimited(acc.ID, time.Now().Add(result.retryAfter), result.statusReason)
		case outcomeAuthFailed:
			e.recordAccountAuthFailure(acc.ID, msg)
		}
		if updated, getErr := e.store.GetAccount(acc.ID); getErr == nil {
			res.AccountStatus = string(updated.Status)
		}
		res.Error = result.errMsg
		res.ErrorKind = result.errorKind
		res.LatencyMS = time.Since(start).Milliseconds()
		e.logRequestWithDetails(acc, requestLogDetails{ResolvedModel: testModel, Status: resp.StatusCode, Latency: time.Since(start), Stream: true, Error: msg, ErrorKind: result.errorKind, TerminalEvent: result.terminalEvent})
		return res
	}

	// Consume the SSE stream, translating events so usage tokens and a short
	// text sample can be captured.
	state := apicompat.NewResponsesEventToChatState()
	state.Model = testModel
	state.IncludeUsage = true
	var sample strings.Builder
	terminal := &sseTerminal{}
	sc := scanSSE(resp.Body)
	for sc.Scan() {
		evt, ok := parseSSEEvent(sc.Text())
		if !ok {
			continue
		}
		terminal.observe(evt)
		if terminal.errorKind != "" {
			break
		}
		for _, chunk := range apicompat.ResponsesEventToChatChunks(evt, state) {
			for _, ch := range chunk.Choices {
				if ch.Delta.Content != nil && sample.Len() < 400 {
					sample.WriteString(*ch.Delta.Content)
				}
			}
		}
	}
	if err := terminal.finish(sc.Err()); err != nil {
		res.Status = terminal.status
		res.Error = terminal.message
		res.ErrorKind = terminal.errorKind
		res.LatencyMS = time.Since(start).Milliseconds()
		e.logRequestWithDetails(acc, requestLogDetails{ResolvedModel: testModel, Status: terminal.status, Latency: time.Since(start), Stream: true, Error: terminal.message, ErrorKind: terminal.errorKind, TerminalEvent: terminal.event})
		return res
	}

	prompt2, completion := usageCounts(state.Usage)
	_ = e.store.TouchAccount(acc.ID)
	_ = e.store.RecordAccountTestSuccess(acc.ID)
	res.OK = true
	if updated, getErr := e.store.GetAccount(acc.ID); getErr == nil {
		res.AccountStatus = string(updated.Status)
	}
	res.PromptTokens = prompt2
	res.CompletionTokens = completion
	res.TotalTokens = prompt2 + completion
	res.LatencyMS = time.Since(start).Milliseconds()
	res.Sample = strings.TrimSpace(sample.String())
	e.logRequest(acc, testModel, http.StatusOK, prompt2, completion, time.Since(start), true, "")
	return res
}
