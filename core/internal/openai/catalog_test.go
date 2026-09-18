package openai

import (
	"strings"
	"testing"
)

func TestCatalogIncludesGPT56Everywhere(t *testing.T) {
	wanted := []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"}
	models := GatewayModels()
	options := ModelOptions()
	for _, id := range wanted {
		if !containsModel(models, id) {
			t.Fatalf("gateway catalog missing %q", id)
		}
		if !containsString(options, id) {
			t.Fatalf("model options missing %q", id)
		}
		mapped, _ := MapCodexModel(id)
		if mapped != id {
			t.Fatalf("MapCodexModel(%q) = %q", id, mapped)
		}
	}
	for name, value := range map[string]string{
		"gateway": DefaultGatewayModel,
		"codex":   DefaultCodexModel,
		"test":    DefaultTestModel,
	} {
		if value != wanted[0] {
			t.Fatalf("%s default = %q, want %q", name, value, wanted[0])
		}
	}
}

func TestCatalogReturnsCopiesWithoutDuplicateOptions(t *testing.T) {
	models := GatewayModels()
	models[0].ID = "changed"
	if GatewayModels()[0].ID == "changed" {
		t.Fatal("GatewayModels exposed mutable catalog storage")
	}

	seen := make(map[string]struct{})
	for _, option := range ModelOptions() {
		if _, exists := seen[option]; exists {
			t.Fatalf("duplicate model option %q", option)
		}
		seen[option] = struct{}{}
	}
}

func containsModel(models []Model, id string) bool {
	for _, model := range models {
		if model.ID == id {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestGPT6CatalogAndInstructions(t *testing.T) {
	models := GatewayModels()
	if !containsModel(models, "gpt-6") {
		t.Fatal("gateway catalog missing gpt-6")
	}
	if !containsModel(models, "gpt-6-astra") {
		t.Fatal("gateway catalog missing gpt-6-astra")
	}

	instr := CodexBaseInstructionsForModel("gpt-6")
	if len(instr) == 0 {
		t.Fatal("CodexBaseInstructionsForModel(gpt-6) is empty")
	}
	if !strings.Contains(instr, "GPT-6") {
		t.Fatal("CodexBaseInstructionsForModel(gpt-6) does not contain GPT-6 prompt")
	}

	mapped, effort := MapCodexModel("gpt-6-high")
	if mapped != "gpt-6" || effort != "high" {
		t.Fatalf("MapCodexModel(gpt-6-high) = (%q, %q), want (gpt-6, high)", mapped, effort)
	}
}

func TestActiveGatewayModelsDynamicFiltering(t *testing.T) {
	// When empty, returns default catalog
	defaults := ActiveGatewayModels(nil)
	if len(defaults) == 0 {
		t.Fatal("ActiveGatewayModels(nil) returned empty")
	}

	// When active accounts supply models (e.g. DeepSeek only)
	active := []string{"deepseek-chat", "deepseek-reasoner"}
	filtered := ActiveGatewayModels(active)
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].ID != "deepseek-chat" || filtered[0].OwnedBy != "deepseek" {
		t.Fatalf("unexpected filtered[0]: %#v", filtered[0])
	}
	if filtered[1].ID != "deepseek-reasoner" || filtered[1].OwnedBy != "deepseek" {
		t.Fatalf("unexpected filtered[1]: %#v", filtered[1])
	}

	// Active options include deepseek models
	opts := ActiveModelOptions(active)
	if !containsString(opts, "deepseek-chat") || !containsString(opts, "deepseek-reasoner") {
		t.Fatalf("ActiveModelOptions missing deepseek models: %v", opts)
	}
}
