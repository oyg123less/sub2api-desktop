// Package codexcfg writes and restores the Codex CLI configuration so the
// desktop app can point Codex at the local Sub2API gateway with one click,
// replacing the need for an external config switcher. It manages
// ~/.codex/config.toml and ~/.codex/auth.json, backing up any existing files
// before applying so the original setup can be restored.
package codexcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"sub2api-desktop/core/internal/openai"
)

const (
	configName   = "config.toml"
	authName     = "auth.json"
	backupSuffix = ".sub2api-bak"
	absentSuffix = ".sub2api-absent"
	providerID   = "sub2api"
	providerName = "Amber"
	// DefaultModel is the Codex model used when applying the config.
	DefaultModel = openai.DefaultCodexModel
)

var reasoningEffortAssignment = regexp.MustCompile(`(?m)^([ \t]*model_reasoning_effort[ \t]*=[ \t]*)(?:"[^"\r\n]*"|'[^'\r\n]*')`)

// Manager applies and restores Codex configuration under a base directory.
type Manager struct {
	dir string
}

// New returns a Manager rooted at dir. When dir is empty it defaults to
// ~/.codex.
func New(dir string) (*Manager, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".codex")
	}
	return &Manager{dir: dir}, nil
}

// Dir returns the Codex config directory in use.
func (m *Manager) Dir() string { return m.dir }

// Status reports whether the Sub2API provider is currently active.
type Status struct {
	ConfigPath   string `json:"config_path"`
	AuthPath     string `json:"auth_path"`
	Applied      bool   `json:"applied"`
	ConfigExists bool   `json:"config_exists"`
	Model        string `json:"model,omitempty"`
	BackupExists bool   `json:"backup_exists"`
	BackupAt     string `json:"backup_at,omitempty"`
	BackupSource string `json:"backup_source,omitempty"`
}

func (m *Manager) configPath() string { return filepath.Join(m.dir, configName) }
func (m *Manager) authPath() string   { return filepath.Join(m.dir, authName) }

// Status inspects the current Codex config.
func (m *Manager) Status() (Status, error) {
	st := Status{ConfigPath: m.configPath(), AuthPath: m.authPath()}
	data, err := os.ReadFile(m.configPath())
	switch {
	case err == nil:
		st.ConfigExists = true
		var config map[string]any
		if toml.Unmarshal(data, &config) == nil {
			provider, _ := config["model_provider"].(string)
			st.Applied = provider == providerID
			st.Model, _ = config["model"].(string)
		}
	case os.IsNotExist(err):
	default:
		return st, err
	}
	st.BackupExists = fileExists(m.configPath()+backupSuffix) || fileExists(m.configPath()+absentSuffix)
	if st.BackupExists {
		path := m.configPath() + backupSuffix
		if !fileExists(path) {
			path = m.configPath() + absentSuffix
		}
		if info, infoErr := os.Stat(path); infoErr == nil {
			st.BackupAt = info.ModTime().UTC().Format(time.RFC3339)
			st.BackupSource = "configuration before Amber integration"
		}
	}
	return st, nil
}

// Apply writes a Codex config pointing at the given gateway base URL and API
// key, backing up any existing files on first apply.
func (m *Manager) Apply(baseURL, apiKey, model string) error {
	if model == "" {
		model = DefaultModel
	}
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return err
	}
	if err := m.backupOnce(m.configPath()); err != nil {
		return err
	}
	if err := m.backupOnce(m.authPath()); err != nil {
		return err
	}
	if err := atomicWriteFile(m.configPath(), []byte(renderConfig(baseURL, model)), 0o600); err != nil {
		return err
	}
	auth, err := m.mergedAuth(apiKey)
	if err != nil {
		return err
	}
	return atomicWriteFile(m.authPath(), auth, 0o600)
}

// ReadFiles returns the current on-disk contents of config.toml and
// auth.json (empty strings when a file does not exist).
func (m *Manager) ReadFiles() (config, auth string, err error) {
	c, err := os.ReadFile(m.configPath())
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	a, err := os.ReadFile(m.authPath())
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	return string(c), string(a), nil
}

// WriteFiles writes user-edited config.toml / auth.json contents, backing up
// the originals on first write. An empty string leaves that file untouched.
func (m *Manager) WriteFiles(config, auth string) error {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return err
	}
	if config != "" {
		normalizedConfig, err := NormalizeConfig(config)
		if err != nil {
			return err
		}
		if err := m.backupOnce(m.configPath()); err != nil {
			return err
		}
		if err := atomicWriteFile(m.configPath(), []byte(normalizedConfig), 0o600); err != nil {
			return err
		}
	}
	if auth != "" {
		if err := ValidateAuth(auth); err != nil {
			return err
		}
		if err := m.backupOnce(m.authPath()); err != nil {
			return err
		}
		if err := atomicWriteFile(m.authPath(), []byte(auth), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Restore reverts config.toml and auth.json to their pre-apply state.
func (m *Manager) Restore() error {
	if err := snapshotBeforeRestore(m.configPath()); err != nil {
		return err
	}
	if err := snapshotBeforeRestore(m.authPath()); err != nil {
		return err
	}
	if err := restoreOne(m.configPath()); err != nil {
		return err
	}
	return restoreOne(m.authPath())
}

// backupOnce records the original file the first time Apply runs: either a
// copy of the existing file, or a marker noting the file was absent.
func (m *Manager) backupOnce(path string) error {
	if fileExists(path+backupSuffix) || fileExists(path+absentSuffix) {
		return nil // already captured the original
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return atomicWriteFile(path+absentSuffix, []byte{}, 0o600)
	}
	if err != nil {
		return err
	}
	return atomicWriteFile(path+backupSuffix, data, 0o600)
}

func restoreOne(path string) error {
	bak := path + backupSuffix
	absent := path + absentSuffix
	switch {
	case fileExists(bak):
		data, err := os.ReadFile(bak)
		if err != nil {
			return err
		}
		if err := atomicWriteFile(path, data, 0o600); err != nil {
			return err
		}
		return os.Remove(bak)
	case fileExists(absent):
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Remove(absent)
	default:
		return nil // nothing was applied
	}
}

// mergedAuth sets OPENAI_API_KEY while preserving any other keys already in
// auth.json (e.g. an existing ChatGPT login).
func (m *Manager) mergedAuth(apiKey string) ([]byte, error) {
	obj := map[string]any{}
	if data, err := os.ReadFile(m.authPath()); err == nil {
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, fmt.Errorf("existing auth.json is invalid: %w", err)
		}
		if obj == nil {
			return nil, fmt.Errorf("existing auth.json must be a JSON object")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	obj["OPENAI_API_KEY"] = apiKey
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// ValidateConfig parses TOML and verifies the fields required for a usable
// Codex model provider configuration.
func ValidateConfig(config string) error {
	var value map[string]any
	if err := toml.Unmarshal([]byte(config), &value); err != nil {
		return fmt.Errorf("config.toml is invalid: %w", err)
	}
	provider, ok := value["model_provider"].(string)
	if !ok || strings.TrimSpace(provider) == "" {
		return fmt.Errorf("config.toml must define a non-empty model_provider")
	}
	if effortValue, exists := value["model_reasoning_effort"]; exists {
		effort, ok := effortValue.(string)
		if !ok || !validReasoningEffort(effort) {
			return fmt.Errorf("model_reasoning_effort must be one of none, minimal, low, medium, high, xhigh")
		}
	}
	providers, ok := value["model_providers"].(map[string]any)
	if !ok {
		return fmt.Errorf("config.toml must define [model_providers.%s]", provider)
	}
	entry, ok := providers[provider].(map[string]any)
	if !ok {
		return fmt.Errorf("config.toml is missing [model_providers.%s]", provider)
	}
	for _, field := range []string{"base_url", "wire_api"} {
		if text, ok := entry[field].(string); !ok || strings.TrimSpace(text) == "" {
			return fmt.Errorf("model provider %s must define %s", provider, field)
		}
	}
	return nil
}

// NormalizeConfig converts accepted aliases to values understood by Codex,
// then validates the exact content that will be written.
func NormalizeConfig(config string) (string, error) {
	var value map[string]any
	if err := toml.Unmarshal([]byte(config), &value); err != nil {
		return "", fmt.Errorf("config.toml is invalid: %w", err)
	}
	if rawValue, exists := value["model_reasoning_effort"]; exists {
		raw, ok := rawValue.(string)
		if !ok {
			return "", fmt.Errorf("model_reasoning_effort must be a string")
		}
		normalized := openai.NormalizeReasoningEffort(raw)
		if normalized == "" {
			return "", fmt.Errorf("model_reasoning_effort must be one of none, minimal, low, medium, high, xhigh")
		}
		if normalized != raw {
			location := reasoningEffortAssignment.FindStringSubmatchIndex(config)
			if location == nil {
				return "", fmt.Errorf("unable to normalize model_reasoning_effort assignment")
			}
			config = config[:location[3]] + `"` + normalized + `"` + config[location[1]:]
		}
	}
	if err := ValidateConfig(config); err != nil {
		return "", err
	}
	return config, nil
}

func validReasoningEffort(raw string) bool {
	switch raw {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

func ValidateAuth(auth string) error {
	var value map[string]any
	if err := json.Unmarshal([]byte(auth), &value); err != nil {
		return fmt.Errorf("auth.json is invalid: %w", err)
	}
	if value == nil {
		return fmt.Errorf("auth.json must be a JSON object")
	}
	return nil
}

func snapshotBeforeRestore(path string) error {
	if !fileExists(path) || (!fileExists(path+backupSuffix) && !fileExists(path+absentSuffix)) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	snapshot := fmt.Sprintf("%s.sub2api-pre-restore-%s.bak", path, time.Now().UTC().Format("20060102T150405.000000000Z"))
	return atomicWriteFile(snapshot, data, 0o600)
}

// RenderConfig returns the config.toml content the tool writes for the given
// gateway base URL and model. Exposed so the UI can show a copyable preview
// (e.g. for pasting into a remote server's ~/.codex).
func RenderConfig(baseURL, model string) string {
	if model == "" {
		model = DefaultModel
	}
	return renderConfig(baseURL, model)
}

// RenderAuth returns the auth.json content (containing only OPENAI_API_KEY)
// for the given API key.
func RenderAuth(apiKey string) string {
	out, _ := json.MarshalIndent(map[string]any{"OPENAI_API_KEY": apiKey}, "", "  ")
	return string(out) + "\n"
}

func renderConfig(baseURL, model string) string {
	return fmt.Sprintf(`model_provider = %q
model = %q
model_reasoning_effort = "high"
disable_response_storage = true

[model_providers.%s]
name = %q
base_url = %q
wire_api = "responses"
requires_openai_auth = true
`, providerID, model, providerID, providerName, baseURL)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
