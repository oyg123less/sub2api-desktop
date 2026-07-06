package openai

import "strings"

// ModelPrice holds per-1M-token prices in USD for a model family.
type ModelPrice struct {
	InputPerM  float64 // USD per 1,000,000 prompt tokens
	OutputPerM float64 // USD per 1,000,000 completion tokens
}

// defaultModelPrice is the fallback for GPT-5 class models (public list price).
var defaultModelPrice = ModelPrice{InputPerM: 1.25, OutputPerM: 10.0}

// modelPricePrefixes maps model-name prefixes to prices. Longest prefix wins.
// Prices mirror OpenAI's published GPT-5 family list prices (USD / 1M tokens);
// gpt-5.x versions reuse the base gpt-5 tier. This is only for a local usage
// estimate — the relay itself does not charge.
var modelPricePrefixes = []struct {
	prefix string
	price  ModelPrice
}{
	{"gpt-5.4-nano", ModelPrice{InputPerM: 0.05, OutputPerM: 0.40}},
	{"gpt-5-nano", ModelPrice{InputPerM: 0.05, OutputPerM: 0.40}},
	{"gpt-5.4-mini", ModelPrice{InputPerM: 0.25, OutputPerM: 2.0}},
	{"gpt-5-mini", ModelPrice{InputPerM: 0.25, OutputPerM: 2.0}},
	{"gpt-image", ModelPrice{InputPerM: 5.0, OutputPerM: 40.0}},
}

// PriceForModel returns the price tier for a model, falling back to the GPT-5
// base tier for unknown models.
func PriceForModel(model string) ModelPrice {
	m := strings.ToLower(strings.TrimSpace(model))
	best := ""
	price := defaultModelPrice
	for _, p := range modelPricePrefixes {
		if strings.HasPrefix(m, p.prefix) && len(p.prefix) > len(best) {
			best = p.prefix
			price = p.price
		}
	}
	return price
}

// CostUSD returns the estimated cost in USD for the given token counts.
func CostUSD(model string, promptTokens, completionTokens int64) float64 {
	p := PriceForModel(model)
	return float64(promptTokens)/1e6*p.InputPerM + float64(completionTokens)/1e6*p.OutputPerM
}
