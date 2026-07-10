package openai

import "testing"

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
