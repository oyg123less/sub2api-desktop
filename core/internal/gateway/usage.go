package gateway

import (
	"strings"

	"sub2api-desktop/core/internal/apicompat"
)

type usageMetrics struct {
	Prompt, Cached, Completion, Reasoning int
	Estimated                             bool
}

func estimateTokensFromBytes(byteCount int) int {
	if byteCount <= 0 {
		return 0
	}
	return (byteCount + 3) / 4
}

func estimatedUsage(meta forwardMeta, outputBytes int) usageMetrics {
	return usageMetrics{
		Prompt:     meta.EstimatedPromptTokens,
		Completion: estimateTokensFromBytes(outputBytes),
		Estimated:  true,
	}
}

func metricsFromResponses(usage *apicompat.ResponsesUsage) (usageMetrics, bool) {
	if usage == nil {
		return usageMetrics{}, false
	}
	result := usageMetrics{Prompt: usage.InputTokens, Completion: usage.OutputTokens}
	if usage.InputTokensDetails != nil {
		result.Cached = usage.InputTokensDetails.CachedTokens
	}
	if usage.OutputTokensDetails != nil {
		result.Reasoning = usage.OutputTokensDetails.ReasoningTokens
	}
	return result, true
}

func metricsFromChat(usage *apicompat.ChatUsage) (usageMetrics, bool) {
	if usage == nil {
		return usageMetrics{}, false
	}
	result := usageMetrics{Prompt: usage.PromptTokens, Completion: usage.CompletionTokens}
	if usage.PromptTokensDetails != nil {
		result.Cached = usage.PromptTokensDetails.CachedTokens
	}
	if usage.CompletionTokensDetails != nil {
		result.Reasoning = usage.CompletionTokensDetails.ReasoningTokens
	}
	return result, true
}

func bestUsage(responses *apicompat.ResponsesUsage, chat *apicompat.ChatUsage, meta forwardMeta, outputBytes int) usageMetrics {
	if usage, ok := metricsFromResponses(responses); ok {
		return usage
	}
	if usage, ok := metricsFromChat(chat); ok {
		return usage
	}
	return estimatedUsage(meta, outputBytes)
}

func responseEventOutputBytes(event *apicompat.ResponsesStreamEvent) int {
	if event == nil || !strings.HasSuffix(event.Type, ".delta") {
		return 0
	}
	if event.Delta != "" {
		return len([]byte(event.Delta))
	}
	return len([]byte(event.Arguments))
}

func chatChunkOutputBytes(chunk apicompat.ChatCompletionsChunk) int {
	total := 0
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != nil {
			total += len([]byte(*choice.Delta.Content))
		}
		if choice.Delta.ReasoningContent != nil {
			total += len([]byte(*choice.Delta.ReasoningContent))
		}
		for _, call := range choice.Delta.ToolCalls {
			total += len([]byte(call.Function.Arguments))
		}
	}
	return total
}
