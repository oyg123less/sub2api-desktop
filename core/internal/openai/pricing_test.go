package openai

import (
	"math"
	"testing"
)

func TestPublishedPricingAndLongestPrefix(t *testing.T) {
	tests := []struct {
		model  string
		input  float64
		cached float64
		output float64
	}{
		{"gpt-5.6-sol-high", 5, 0.5, 30},
		{"gpt-5.6-terra-2026-07-16", 2.5, 0.25, 15},
		{"gpt-5.6-luna", 1, 0.1, 6},
		{"gpt-5.5-pro", 30, 0, 180},
		{"gpt-5.4-mini", 0.75, 0.075, 4.5},
		{"gpt-5.4-nano", 0.2, 0.02, 1.25},
		{"gpt-5.3-codex-high", 1.75, 0.175, 14},
	}
	for _, tt := range tests {
		p, ok := LookupModelPrice(tt.model)
		if !ok || p.InputPerM != tt.input || p.OutputPerM != tt.output {
			t.Fatalf("%s price = %#v, ok=%v", tt.model, p, ok)
		}
		if tt.cached == 0 && p.CachedPerM != nil {
			t.Fatalf("%s unexpectedly has cached pricing", tt.model)
		}
		if tt.cached != 0 && (p.CachedPerM == nil || *p.CachedPerM != tt.cached) {
			t.Fatalf("%s cached price = %#v", tt.model, p.CachedPerM)
		}
	}
}

func TestCostUSDUsesCachedAndLongContextTiers(t *testing.T) {
	short := CostUSD("gpt-5.4", 200_000, 50_000, 10_000)
	wantShort := 150_000.0/1e6*2.5 + 50_000.0/1e6*0.25 + 10_000.0/1e6*15
	if math.Abs(short-wantShort) > 1e-9 {
		t.Fatalf("short cost = %f, want %f", short, wantShort)
	}
	boundary := CostUSD("gpt-5.4", 272_000, 0, 1)
	if math.Abs(boundary-(272_000.0/1e6*2.5+1.0/1e6*15)) > 1e-9 {
		t.Fatalf("threshold boundary used long tier: %f", boundary)
	}
	long := CostUSD("gpt-5.4", 272_001, 72_001, 10_000)
	wantLong := 200_000.0/1e6*5 + 72_001.0/1e6*0.5 + 10_000.0/1e6*22.5
	if math.Abs(long-wantLong) > 1e-9 {
		t.Fatalf("long cost = %f, want %f", long, wantLong)
	}
}

func TestUnknownModelFallsBackToSol(t *testing.T) {
	unknown := CostUSD("future-model", 100_000, 0, 100_000)
	if unknown != 3.5 {
		t.Fatalf("fallback cost = %f, want 3.5", unknown)
	}
}

func TestRequestLevelPricingDoesNotPromoteAggregatedShortRequests(t *testing.T) {
	price, ok := LookupModelPrice("gpt-5.4")
	if !ok {
		t.Fatal("gpt-5.4 price missing")
	}
	requestCost := CostUSDForPrice(price, 200_000, 0, 1_000)
	summed := requestCost + requestCost
	aggregatedWrong := CostUSDForPrice(price, 400_000, 0, 2_000)
	if summed >= aggregatedWrong {
		t.Fatalf("test fixture did not exercise long-context overcharge: per-request=%f aggregated=%f", summed, aggregatedWrong)
	}
	want := 400_000.0/1e6*2.5 + 2_000.0/1e6*15
	if math.Abs(summed-want) > 1e-9 {
		t.Fatalf("request-level sum=%f, want=%f", summed, want)
	}
}
