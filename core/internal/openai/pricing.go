package openai

import (
	"log/slog"
	"strings"
)

const (
	PriceVersion   = "2026-07-16"
	PriceSourceURL = "https://developers.openai.com/api/docs/pricing"
)

// ModelPrice holds OpenAI Standard-tier prices in USD per 1M tokens.
// Nil cached prices mean that OpenAI does not publish a cached-input discount.
type ModelPrice struct {
	Model                string   `json:"model"`
	InputPerM            float64  `json:"input_per_m"`
	CachedPerM           *float64 `json:"cached_per_m,omitempty"`
	OutputPerM           float64  `json:"output_per_m"`
	LongContextThreshold int64    `json:"long_context_threshold,omitempty"`
	LongInputPerM        float64  `json:"long_input_per_m,omitempty"`
	LongCachedPerM       *float64 `json:"long_cached_per_m,omitempty"`
	LongOutputPerM       float64  `json:"long_output_per_m,omitempty"`
}

func price(value float64) *float64 { return &value }

var publishedModelPrices = []ModelPrice{
	{Model: "gpt-5.6-sol", InputPerM: 5.00, CachedPerM: price(0.50), OutputPerM: 30.00, LongContextThreshold: 272_000, LongInputPerM: 10.00, LongCachedPerM: price(1.00), LongOutputPerM: 45.00},
	{Model: "gpt-5.6-terra", InputPerM: 2.50, CachedPerM: price(0.25), OutputPerM: 15.00, LongContextThreshold: 272_000, LongInputPerM: 5.00, LongCachedPerM: price(0.50), LongOutputPerM: 22.50},
	{Model: "gpt-5.6-luna", InputPerM: 1.00, CachedPerM: price(0.10), OutputPerM: 6.00, LongContextThreshold: 272_000, LongInputPerM: 2.00, LongCachedPerM: price(0.20), LongOutputPerM: 9.00},
	{Model: "gpt-5.5-pro", InputPerM: 30.00, OutputPerM: 180.00, LongContextThreshold: 272_000, LongInputPerM: 60.00, LongOutputPerM: 270.00},
	{Model: "gpt-5.5", InputPerM: 5.00, CachedPerM: price(0.50), OutputPerM: 30.00, LongContextThreshold: 272_000, LongInputPerM: 10.00, LongCachedPerM: price(1.00), LongOutputPerM: 45.00},
	{Model: "gpt-5.4-mini", InputPerM: 0.75, CachedPerM: price(0.075), OutputPerM: 4.50},
	{Model: "gpt-5.4-nano", InputPerM: 0.20, CachedPerM: price(0.02), OutputPerM: 1.25},
	{Model: "gpt-5.4-pro", InputPerM: 30.00, OutputPerM: 180.00, LongContextThreshold: 272_000, LongInputPerM: 60.00, LongOutputPerM: 270.00},
	{Model: "gpt-5.4", InputPerM: 2.50, CachedPerM: price(0.25), OutputPerM: 15.00, LongContextThreshold: 272_000, LongInputPerM: 5.00, LongCachedPerM: price(0.50), LongOutputPerM: 22.50},
	{Model: "gpt-5.3-codex", InputPerM: 1.75, CachedPerM: price(0.175), OutputPerM: 14.00},
}

// PublishedModelPrices returns a copy suitable for the control API.
func PublishedModelPrices() []ModelPrice {
	result := make([]ModelPrice, len(publishedModelPrices))
	copy(result, publishedModelPrices)
	return result
}

// LookupModelPrice uses longest-prefix matching for snapshots and effort
// suffixes while keeping the displayed catalog free of duplicate aliases.
func LookupModelPrice(model string) (ModelPrice, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	bestLength := 0
	var result ModelPrice
	for _, candidate := range publishedModelPrices {
		if strings.HasPrefix(normalized, candidate.Model) && len(candidate.Model) > bestLength {
			result = candidate
			bestLength = len(candidate.Model)
		}
	}
	return result, bestLength > 0
}

// PriceForModel falls back to gpt-5.6-sol and records the mismatch without
// leaking request contents.
func PriceForModel(model string) ModelPrice {
	if result, ok := LookupModelPrice(model); ok {
		return result
	}
	fallback := publishedModelPrices[0]
	slog.Warn("unknown model pricing fallback", "model", strings.TrimSpace(model), "fallback", fallback.Model)
	return fallback
}

// CostUSD applies cached-input discounts and switches the entire request to
// the long-context tier once prompt tokens exceed the published threshold.
func CostUSD(model string, promptTokens, cachedTokens, completionTokens int64) float64 {
	p := PriceForModel(model)
	return CostUSDForPrice(p, promptTokens, cachedTokens, completionTokens)
}

// CostUSDForPrice prices one request using an already resolved catalog entry.
// Callers that aggregate logs must invoke this before summing requests so the
// long-context threshold is evaluated per request rather than on token totals.
func CostUSDForPrice(p ModelPrice, promptTokens, cachedTokens, completionTokens int64) float64 {
	if promptTokens < 0 {
		promptTokens = 0
	}
	if completionTokens < 0 {
		completionTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if cachedTokens > promptTokens {
		cachedTokens = promptTokens
	}
	inputRate, outputRate, cachedRate := p.InputPerM, p.OutputPerM, p.InputPerM
	if p.CachedPerM != nil {
		cachedRate = *p.CachedPerM
	}
	if p.LongContextThreshold > 0 && promptTokens > p.LongContextThreshold {
		inputRate, outputRate, cachedRate = p.LongInputPerM, p.LongOutputPerM, p.LongInputPerM
		if p.LongCachedPerM != nil {
			cachedRate = *p.LongCachedPerM
		}
	}
	uncachedTokens := promptTokens - cachedTokens
	return float64(uncachedTokens)/1e6*inputRate + float64(cachedTokens)/1e6*cachedRate + float64(completionTokens)/1e6*outputRate
}
