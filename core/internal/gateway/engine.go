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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/apicompat"
	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

// defaultUpstreamURL is the ChatGPT Codex responses endpoint.
const defaultUpstreamURL = "https://chatgpt.com/backend-api/codex/responses"

// UpstreamURL returns the effective upstream endpoint, allowing override via the
// SUB2API_UPSTREAM_URL environment variable (used for tests).
func UpstreamURL() string {
	if v := os.Getenv("SUB2API_UPSTREAM_URL"); v != "" {
		return v
	}
	return defaultUpstreamURL
}

// Engine holds the dependencies for request forwarding.
type Engine struct {
	store    *store.Store
	accounts *account.Manager
	settings func() store.Settings
	logger   *slog.Logger
}

// New constructs a gateway engine.
func New(s *store.Store, mgr *account.Manager, settings func() store.Settings, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{store: s, accounts: mgr, settings: settings, logger: logger}
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{Error: apiErrorBody{Message: msg, Type: typ}})
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
	model, ok := resolveModel(requestedModel, cfg.DefaultModel)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown model: "+requestedModel+"（仅支持 gpt-5*/codex 系列模型）", "invalid_request_error")
		return
	}
	chatReq.Model = model
	logModel := upstreamLogModel(model)

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
		result := e.forwardOnce(r.Context(), w, &chatReq, requestedModel, logModel, acc, cfg)
		switch result.outcome {
		case outcomeSuccess:
			return
		case outcomeRateLimited:
			retry := result.retryAfter
			if retry <= 0 {
				retry = 10 * time.Minute
			}
			until := time.Now().Add(retry)
			_ = e.store.SetRateLimited(acc.ID, until)
			lastErr = result.errMsg
			continue // try next account
		case outcomeAuthFailed:
			_ = e.store.SetAccountStatus(acc.ID, store.AccountRefreshFailed, result.errMsg)
			lastErr = result.errMsg
			continue
		case outcomeClientClosed:
			return
		default:
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
}

func (e *Engine) forwardOnce(ctx context.Context, w http.ResponseWriter, chatReq *apicompat.ChatCompletionsRequest, requestedModel, logModel string, acc *store.Account, cfg store.Settings) forwardResult {
	start := time.Now()

	// Resolve proxy + client.
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
		e.logRequest(acc, logModel, resp.StatusCode, 0, 0, time.Since(start), chatReq.Stream, "rate limited (429)")
		return forwardResult{outcome: outcomeRateLimited, status: resp.StatusCode, errMsg: "账号已限额（429）", retryAfter: rateLimitRetryAfter(resp.Header, usage)}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msg := readUpstreamError(resp.Body)
		e.logRequest(acc, logModel, resp.StatusCode, 0, 0, time.Since(start), chatReq.Stream, msg)
		return forwardResult{outcome: outcomeAuthFailed, status: resp.StatusCode, errMsg: "鉴权失败: " + msg}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := readUpstreamError(resp.Body)
		e.logRequest(acc, logModel, resp.StatusCode, 0, 0, time.Since(start), chatReq.Stream, msg)
		return forwardResult{outcome: outcomeUpstreamError, status: resp.StatusCode, errMsg: msg}
	}

	_ = e.store.TouchAccount(acc.ID)

	// Stream and translate.
	if chatReq.Stream {
		return e.streamResponse(w, resp.Body, chatReq, requestedModel, logModel, acc, start)
	}
	return e.aggregateResponse(w, resp.Body, chatReq, requestedModel, logModel, acc, start)
}

func readUpstreamError(body io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(body, 64<<10))
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "upstream returned an error"
	}
	if len(s) > 800 {
		s = s[:800]
	}
	return s
}

func (e *Engine) selectAccounts() ([]*store.Account, error) {
	all, err := e.store.ListAccounts()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var out []*store.Account
	for _, a := range all {
		if a.Status == store.AccountDisabled {
			continue
		}
		if a.Status == store.AccountRateLimited && a.RateLimitedUntil != nil && now.Before(*a.RateLimitedUntil) {
			continue
		}
		out = append(out, a)
	}
	return out, nil
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
	if acc.ChatGPTAccountID != "" {
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
	var accID *int64
	email := ""
	if acc != nil {
		accID = &acc.ID
		email = acc.Email
	}
	_ = e.store.InsertLog(&store.RequestLog{
		AccountID:        accID,
		AccountEmail:     email,
		Model:            model,
		StatusCode:       status,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
		LatencyMS:        latency.Milliseconds(),
		Stream:           stream,
		Error:            errMsg,
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
