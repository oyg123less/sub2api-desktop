package codexcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCodexCatalogIncludesStandardAndActiveModels(t *testing.T) {
	active := []string{"kimi-k2.6", "deepseek-chat"}
	models := BuildCodexCatalog(active)

	foundKimi := false
	foundDeepSeek := false
	foundAstra := false

	for _, m := range models {
		switch m.Slug {
		case "kimi-k2.6":
			foundKimi = true
			if m.DisplayName == "" {
				t.Fatalf("expected non-empty display name for kimi")
			}
		case "deepseek-chat":
			foundDeepSeek = true
		case "gpt-6-astra":
			foundAstra = true
		}
	}

	if !foundKimi || !foundDeepSeek || !foundAstra {
		t.Fatalf("catalog missing models: kimi=%v deepseek=%v astra=%v", foundKimi, foundDeepSeek, foundAstra)
	}
}

func TestSyncModelCatalogWritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	active := []string{"kimi-k2.6"}
	if err := SyncModelCatalog(dir, active); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, CatalogFileName)
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Models []CodexCatalogModel `json:"models"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid json generated: %v", err)
	}
	if len(parsed.Models) == 0 {
		t.Fatalf("expected models in catalog, got 0")
	}
}

func TestMergeConfigPreservesCustomSections(t *testing.T) {
	existing := `model_provider = "openai"
model = "gpt-4o"

notify = ["some-hook", "turn-ended"]

[windows]
sandbox = "elevated"

[projects.'d:\study\test']
trust_level = "trusted"

[mcp_servers.my_server]
command = "node"
args = ["server.js"]
`
	catalogPath := "C:/Users/test/.codex/amber_models.json"
	merged := mergeConfig(existing, "http://127.0.0.1:8080/v1", "gpt-6-astra", catalogPath)

	if !strings.Contains(merged, `model_provider = "sub2api"`) {
		t.Fatalf("expected model_provider to be sub2api in:\n%s", merged)
	}
	if !strings.Contains(merged, `model = "gpt-6-astra"`) {
		t.Fatalf("expected model to be updated in:\n%s", merged)
	}
	if !strings.Contains(merged, `model_catalog_json = "C:/Users/test/.codex/amber_models.json"`) {
		t.Fatalf("expected model_catalog_json to be set in:\n%s", merged)
	}
	if !strings.Contains(merged, `[model_providers.sub2api]`) {
		t.Fatalf("expected [model_providers.sub2api] table in:\n%s", merged)
	}
	// Verify other sections are preserved
	if !strings.Contains(merged, `notify = ["some-hook", "turn-ended"]`) {
		t.Fatalf("notify array lost in:\n%s", merged)
	}
	if !strings.Contains(merged, `[windows]`) || !strings.Contains(merged, `sandbox = "elevated"`) {
		t.Fatalf("[windows] section lost in:\n%s", merged)
	}
	if !strings.Contains(merged, `[projects.'d:\study\test']`) || !strings.Contains(merged, `trust_level = "trusted"`) {
		t.Fatalf("[projects] section lost in:\n%s", merged)
	}
	if !strings.Contains(merged, `[mcp_servers.my_server]`) {
		t.Fatalf("[mcp_servers] section lost in:\n%s", merged)
	}
}
