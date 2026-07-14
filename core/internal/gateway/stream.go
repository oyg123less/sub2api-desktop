package gateway

import (
	"io"
	"net/http"
	"strings"
	"time"

	"sub2api-desktop/core/internal/apicompat"
	"sub2api-desktop/core/internal/store"
)

// streamResponse translates the upstream Responses SSE into Chat Completions
// SSE and writes it to the client as it arrives.
func (e *Engine) streamResponse(w http.ResponseWriter, body io.Reader, chatReq *apicompat.ChatCompletionsRequest, acc *store.Account, start time.Time, meta forwardMeta) forwardResult {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return forwardResult{outcome: outcomeUpstreamError, status: http.StatusInternalServerError, errMsg: "streaming unsupported"}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	streamStarted := false
	startStream := func() {
		if streamStarted {
			return
		}
		streamStarted = true
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}

	model := responseModel(meta)
	state := apicompat.NewResponsesEventToChatState()
	state.Model = model
	state.IncludeUsage = chatReq.StreamOptions != nil && chatReq.StreamOptions.IncludeUsage

	write := func(chunk apicompat.ChatCompletionsChunk) bool {
		chunk.Model = model
		sse, err := apicompat.ChatChunkToSSE(chunk)
		if err != nil {
			return false
		}
		startStream()
		if _, err := io.WriteString(w, sse); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	terminal := &sseTerminal{}
	sc := scanSSE(body)
	for sc.Scan() {
		line := sc.Text()
		evt, ok := parseSSEEvent(line)
		if !ok {
			continue
		}
		terminal.observe(evt)
		if terminal.errorKind != "" {
			prompt, completion := usageCounts(state.Usage)
			e.logForward(acc, meta, terminal.status, prompt, completion, time.Since(start), terminal.message, terminal.errorKind, terminal.event)
			if !streamStarted {
				return forwardResult{outcome: outcomeUpstreamError, status: terminal.status, errMsg: terminal.message, retryable: true}
			}
			writeStreamError(w, terminal.errorKind, terminal.message)
			flusher.Flush()
			return forwardResult{outcome: outcomeUpstreamError, status: terminal.status, errMsg: terminal.message, headersWritten: true}
		}
		for _, chunk := range apicompat.ResponsesEventToChatChunks(evt, state) {
			if !write(chunk) {
				e.logForward(acc, meta, 499, 0, 0, time.Since(start), "client cancelled stream", "client_cancelled", "client_cancelled")
				return forwardResult{outcome: outcomeClientClosed, headersWritten: true}
			}
		}
	}
	if err := terminal.finish(sc.Err()); err != nil {
		prompt, completion := usageCounts(state.Usage)
		e.logForward(acc, meta, terminal.status, prompt, completion, time.Since(start), terminal.message, terminal.errorKind, terminal.event)
		if !streamStarted {
			return forwardResult{outcome: outcomeUpstreamError, status: terminal.status, errMsg: terminal.message, retryable: true}
		}
		writeStreamError(w, terminal.errorKind, terminal.message)
		flusher.Flush()
		return forwardResult{outcome: outcomeUpstreamError, status: terminal.status, errMsg: terminal.message, headersWritten: true}
	}
	startStream()
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()

	prompt, completion := usageCounts(state.Usage)
	e.logForward(acc, meta, http.StatusOK, prompt, completion, time.Since(start), "", "", terminal.event)
	return forwardResult{outcome: outcomeSuccess, headersWritten: true}
}

// aggregateResponse consumes the upstream SSE and assembles a single
// non-streaming Chat Completions response.
func (e *Engine) aggregateResponse(w http.ResponseWriter, body io.Reader, chatReq *apicompat.ChatCompletionsRequest, acc *store.Account, start time.Time, meta forwardMeta) forwardResult {
	model := responseModel(meta)
	state := apicompat.NewResponsesEventToChatState()
	state.Model = model
	state.IncludeUsage = true

	var content strings.Builder
	var reasoning strings.Builder
	toolCalls := map[int]*apicompat.ChatToolCall{}
	var toolOrder []int
	finishReason := "stop"

	apply := func(chunk apicompat.ChatCompletionsChunk) {
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != nil {
				content.WriteString(*ch.Delta.Content)
			}
			if ch.Delta.ReasoningContent != nil {
				reasoning.WriteString(*ch.Delta.ReasoningContent)
			}
			for _, tc := range ch.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				existing, ok := toolCalls[idx]
				if !ok {
					nc := apicompat.ChatToolCall{Index: tc.Index, ID: tc.ID, Type: "function"}
					toolCalls[idx] = &nc
					toolOrder = append(toolOrder, idx)
					existing = &nc
				}
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				existing.Function.Arguments += tc.Function.Arguments
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				finishReason = *ch.FinishReason
			}
		}
	}

	terminal := &sseTerminal{}
	sc := scanSSE(body)
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
			apply(chunk)
		}
	}
	if err := terminal.finish(sc.Err()); err != nil {
		prompt, completion := usageCounts(state.Usage)
		e.logForward(acc, meta, terminal.status, prompt, completion, time.Since(start), terminal.message, terminal.errorKind, terminal.event)
		writeError(w, terminal.status, terminal.message, terminal.errorKind)
		return forwardResult{outcome: outcomeUpstreamError, status: terminal.status, errMsg: terminal.message, headersWritten: true}
	}

	msg := apicompat.ChatMessage{Role: "assistant"}
	text := content.String()
	msg.Content = jsonString(text)
	if reasoning.Len() > 0 {
		msg.ReasoningContent = reasoning.String()
	}
	for _, idx := range toolOrder {
		tc := toolCalls[idx]
		tc.Index = nil // omit index in non-streaming responses
		msg.ToolCalls = append(msg.ToolCalls, *tc)
	}

	resp := apicompat.ChatCompletionsResponse{
		ID:      state.ID,
		Object:  "chat.completion",
		Created: state.Created,
		Model:   model,
		Choices: []apicompat.ChatChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: state.Usage,
	}

	prompt, completion := usageCounts(state.Usage)
	writeJSON(w, http.StatusOK, resp)
	e.logForward(acc, meta, http.StatusOK, prompt, completion, time.Since(start), "", "", terminal.event)
	return forwardResult{outcome: outcomeSuccess, headersWritten: true}
}

func responseModel(meta forwardMeta) string {
	if strings.TrimSpace(meta.RequestedModel) == "" {
		return meta.ResolvedModel
	}
	return meta.RequestedModel
}

func usageCounts(u *apicompat.ChatUsage) (int, int) {
	if u == nil {
		return 0, 0
	}
	return u.PromptTokens, u.CompletionTokens
}
