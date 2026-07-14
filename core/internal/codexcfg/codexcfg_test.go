package codexcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyThenRestoreExistingConfig(t *testing.T) {
	dir := t.TempDir()
	orig := "model = \"gpt-4o\"\n# my custom setup\n"
	origAuth := `{"tokens":{"id_token":"abc"}}`
	writeFile(t, filepath.Join(dir, configName), orig)
	writeFile(t, filepath.Join(dir, authName), origAuth)

	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Apply("http://127.0.0.1:8080/v1", "sk-local-xyz", ""); err != nil {
		t.Fatal(err)
	}

	cfg := readFile(t, filepath.Join(dir, configName))
	if !strings.Contains(cfg, `model_provider = "sub2api"`) {
		t.Fatalf("config not applied: %s", cfg)
	}
	if !strings.Contains(cfg, `base_url = "http://127.0.0.1:8080/v1"`) {
		t.Fatalf("base_url missing: %s", cfg)
	}
	if !strings.Contains(cfg, `model = "gpt-5.6-sol"`) {
		t.Fatalf("default model missing: %s", cfg)
	}

	// auth.json keeps the existing token and adds the API key.
	var auth map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, authName))), &auth); err != nil {
		t.Fatal(err)
	}
	if auth["OPENAI_API_KEY"] != "sk-local-xyz" {
		t.Fatalf("api key not written: %v", auth)
	}
	if _, ok := auth["tokens"]; !ok {
		t.Fatalf("existing auth key lost: %v", auth)
	}

	st, err := m.Status("http://127.0.0.1:8080/v1", "sk-local-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Applied || !st.BackupExists {
		t.Fatalf("unexpected status: %+v", st)
	}
	if st.Model != "gpt-5.6-sol" {
		t.Fatalf("status model = %q, want gpt-5.6-sol", st.Model)
	}

	if err := m.Restore(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, configName)); got != orig {
		t.Fatalf("restore mismatch: %q != %q", got, orig)
	}
	if got := readFile(t, filepath.Join(dir, authName)); got != origAuth {
		t.Fatalf("auth restore mismatch: %q", got)
	}
	if fileExists(filepath.Join(dir, configName) + backupSuffix) {
		t.Fatal("backup not cleaned up")
	}
}

func TestStatusDetectsConfigurationDrift(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Apply("http://127.0.0.1:8080/v1", "key-one", "gpt-5.6"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		baseURL    string
		apiKey     string
		wantStale  bool
		wantReason string
	}{
		{name: "current", baseURL: "http://127.0.0.1:8080/v1", apiKey: "key-one"},
		{name: "base URL changed", baseURL: "http://127.0.0.1:9090/v1", apiKey: "key-one", wantStale: true, wantReason: "base_url_mismatch"},
		{name: "API key changed", baseURL: "http://127.0.0.1:8080/v1", apiKey: "key-two", wantStale: true, wantReason: "api_key_mismatch"},
		{name: "both changed", baseURL: "http://127.0.0.1:9090/v1", apiKey: "key-two", wantStale: true, wantReason: "base_url_mismatch,api_key_mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := m.Status(tt.baseURL, tt.apiKey)
			if err != nil {
				t.Fatal(err)
			}
			if !status.Applied || status.Stale != tt.wantStale || status.StaleReason != tt.wantReason {
				t.Fatalf("status = %+v", status)
			}
		})
	}
}

func TestApplyThenRestoreNoPriorConfig(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Apply("http://127.0.0.1:8080/v1", "sk-local-xyz", ""); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(dir, configName)) {
		t.Fatal("config not created")
	}
	if err := m.Restore(); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(dir, configName)) {
		t.Fatal("config should be removed on restore when none existed before")
	}
	if fileExists(filepath.Join(dir, authName)) {
		t.Fatal("auth should be removed on restore when none existed before")
	}
}

func TestBackupPreservesTrueOriginalAcrossReapply(t *testing.T) {
	dir := t.TempDir()
	orig := "model = \"gpt-4o\"\n"
	writeFile(t, filepath.Join(dir, configName), orig)
	m, _ := New(dir)
	if err := m.Apply("http://127.0.0.1:8080/v1", "k1", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Apply("http://127.0.0.1:9090/v1", "k2", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Restore(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, configName)); got != orig {
		t.Fatalf("re-apply clobbered original backup: %q", got)
	}
}

func TestWriteFilesNormalizesMaxReasoningEffort(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	config := strings.Replace(RenderConfig("http://127.0.0.1:8080/v1", "gpt-5.6-sol"),
		`model_reasoning_effort = "high"`, `model_reasoning_effort = "max" # preserve`, 1)
	if err := m.WriteFiles(config, ""); err != nil {
		t.Fatal(err)
	}
	written := readFile(t, filepath.Join(dir, configName))
	if !strings.Contains(written, `model_reasoning_effort = "xhigh" # preserve`) {
		t.Fatalf("reasoning effort was not normalized: %s", written)
	}
	if err := ValidateConfig(written); err != nil {
		t.Fatalf("normalized config is invalid: %v", err)
	}
}

func TestValidateConfigRejectsUnsupportedReasoningEffort(t *testing.T) {
	config := strings.Replace(RenderConfig("http://127.0.0.1:8080/v1", "gpt-5.6-sol"),
		`model_reasoning_effort = "high"`, `model_reasoning_effort = "ultra"`, 1)
	if err := ValidateConfig(config); err == nil {
		t.Fatal("unsupported reasoning effort was accepted")
	}
	if _, err := NormalizeConfig(config); err == nil {
		t.Fatal("unsupported reasoning effort was normalized")
	}
}

func TestNormalizeConfigCanonicalizesReasoningEffortCase(t *testing.T) {
	config := strings.Replace(RenderConfig("http://127.0.0.1:8080/v1", "gpt-5.6-sol"),
		`model_reasoning_effort = "high"`, `model_reasoning_effort = "HIGH"`, 1)
	normalized, err := NormalizeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(normalized, `model_reasoning_effort = "high"`) {
		t.Fatalf("reasoning effort case was not normalized: %s", normalized)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
