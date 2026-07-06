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

	testModel := normalizeModel(model, cfg.DefaultModel)
	if testModel == "" {
		testModel = "gpt-5.4"
	}
	testPrompt := strings.TrimSpace(prompt)
	if testPrompt == "" {
		testPrompt = "hi"
	}

	res := TestResult{Model: testModel, AccountStatus: string(acc.Status)}

	var proxy *store.Proxy
	if acc.ProxyID != nil {
		if p, err := e.store.GetProxy(*acc.ProxyID); err == nil {
			proxy = p
		}
	}
	client, err := newHTTPClient(proxy, cfg.TLSFingerprint, 90*time.Second)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	authClient, _ := newHTTPClient(proxy, false, 60*time.Second)

	token, err := e.accounts.ValidAccessToken(ctx, authClient, acc)
	if err != nil {
		_ = e.store.SetAccountStatus(acc.ID, store.AccountRefreshFailed, err.Error())
		res.Error = "令牌刷新失败: " + err.Error()
		res.AccountStatus = string(store.AccountRefreshFailed)
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
		return res
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, UpstreamURL(), bytes.NewReader(upstreamBody))
	if err != nil {
		res.Error = err.Error()
		return res
	}
	setCodexHeaders(req, token, acc, cfg)

	resp, err := client.Do(req)
	if err != nil {
		res.Status = http.StatusBadGateway
		res.Error = "上游请求失败: " + err.Error()
		e.logRequest(acc, testModel, res.Status, 0, 0, time.Since(start), true, res.Error)
		return res
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode

	usage := e.captureCodexUsage(acc, resp.Header)

	if resp.StatusCode == http.StatusTooManyRequests {
		until := time.Now().Add(rateLimitRetryAfter(resp.Header, usage))
		_ = e.store.SetRateLimited(acc.ID, until)
		res.AccountStatus = string(store.AccountRateLimited)
		res.Error = "账号已限额（429）"
		res.LatencyMS = time.Since(start).Milliseconds()
		e.logRequest(acc, testModel, resp.StatusCode, 0, 0, time.Since(start), true, res.Error)
		return res
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msg := readUpstreamError(resp.Body)
		_ = e.store.SetAccountStatus(acc.ID, store.AccountRefreshFailed, msg)
		res.AccountStatus = string(store.AccountRefreshFailed)
		res.Error = "鉴权失败: " + msg
		res.LatencyMS = time.Since(start).Milliseconds()
		e.logRequest(acc, testModel, resp.StatusCode, 0, 0, time.Since(start), true, res.Error)
		return res
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := readUpstreamError(resp.Body)
		res.Error = msg
		res.LatencyMS = time.Since(start).Milliseconds()
		e.logRequest(acc, testModel, resp.StatusCode, 0, 0, time.Since(start), true, msg)
		return res
	}

	// Consume the SSE stream, translating events so usage tokens and a short
	// text sample can be captured.
	state := apicompat.NewResponsesEventToChatState()
	state.Model = testModel
	state.IncludeUsage = true
	var sample strings.Builder
	sc := scanSSE(resp.Body)
	for sc.Scan() {
		evt, ok := parseSSEEvent(sc.Text())
		if !ok {
			continue
		}
		for _, chunk := range apicompat.ResponsesEventToChatChunks(evt, state) {
			for _, ch := range chunk.Choices {
				if ch.Delta.Content != nil && sample.Len() < 400 {
					sample.WriteString(*ch.Delta.Content)
				}
			}
		}
	}
	for _, chunk := range apicompat.FinalizeResponsesChatStream(state) {
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != nil && sample.Len() < 400 {
				sample.WriteString(*ch.Delta.Content)
			}
		}
	}

	prompt2, completion := usageCounts(state.Usage)
	_ = e.store.TouchAccount(acc.ID)
	_ = e.store.SetAccountStatus(acc.ID, store.AccountActive, "")
	res.OK = true
	res.AccountStatus = string(store.AccountActive)
	res.PromptTokens = prompt2
	res.CompletionTokens = completion
	res.TotalTokens = prompt2 + completion
	res.LatencyMS = time.Since(start).Milliseconds()
	res.Sample = strings.TrimSpace(sample.String())
	e.logRequest(acc, testModel, http.StatusOK, prompt2, completion, time.Since(start), true, "")
	return res
}
