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
	"strings"
)

const (
	configName   = "config.toml"
	authName     = "auth.json"
	backupSuffix = ".sub2api-bak"
	absentSuffix = ".sub2api-absent"
	providerID   = "sub2api"
	providerName = "Amber"
	// DefaultModel is the Codex model used when applying the config.
	DefaultModel = "gpt-5.5"
)

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
	BackupExists bool   `json:"backup_exists"`
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
		st.Applied = strings.Contains(string(data), `model_provider = "`+providerID+`"`)
	case os.IsNotExist(err):
	default:
		return st, err
	}
	st.BackupExists = fileExists(m.configPath()+backupSuffix) || fileExists(m.configPath()+absentSuffix)
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
	if err := os.WriteFile(m.configPath(), []byte(renderConfig(baseURL, model)), 0o600); err != nil {
		return err
	}
	auth, err := m.mergedAuth(apiKey)
	if err != nil {
		return err
	}
	return os.WriteFile(m.authPath(), auth, 0o600)
}

// Restore reverts config.toml and auth.json to their pre-apply state.
func (m *Manager) Restore() error {
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
		return os.WriteFile(path+absentSuffix, []byte{}, 0o600)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path+backupSuffix, data, 0o600)
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
		if err := os.WriteFile(path, data, 0o600); err != nil {
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
		_ = json.Unmarshal(data, &obj)
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
wire_api = "chat"
requires_openai_auth = true
`, providerID, model, providerID, providerName, baseURL)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
