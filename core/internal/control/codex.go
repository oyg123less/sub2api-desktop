package control

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"sub2api-desktop/core/internal/codexcfg"
	"sub2api-desktop/core/internal/openai"
)

// codexBaseURL builds the gateway base URL clients should target. Codex always
// connects over loopback regardless of the LAN listen setting.
func (c *Control) codexBaseURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(c.server.Port()) + "/v1"
}

// codexModel returns the model configured for the Codex CLI integration.
func (c *Control) codexModel() string {
	if m := strings.TrimSpace(c.settings.Get().CodexModel); m != "" {
		return m
	}
	return codexcfg.DefaultModel
}

// validCodexModel reports whether a model name belongs to the gpt-5*/codex
// families accepted by the gateway.
func validCodexModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "gpt-5") || strings.Contains(m, "codex")
}

func (c *Control) codexStatus(w http.ResponseWriter, r *http.Request) {
	mgr, err := codexcfg.New("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	base := c.codexBaseURL()
	cfg := c.settings.Get()
	st, err := mgr.Status(base, cfg.LocalAPIKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	model := c.codexModel()
	if st.Applied && validCodexModel(st.Model) {
		model = strings.TrimSpace(st.Model)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"config_path":    st.ConfigPath,
		"auth_path":      st.AuthPath,
		"applied":        st.Applied,
		"config_exists":  st.ConfigExists,
		"backup_exists":  st.BackupExists,
		"backup_at":      st.BackupAt,
		"backup_source":  st.BackupSource,
		"stale":          st.Stale,
		"stale_reason":   st.StaleReason,
		"base_url":       base,
		"model":          model,
		"models":         openai.ModelOptions(),
		"config_preview": codexcfg.RenderConfig(base, model),
		"auth_preview":   codexcfg.RenderAuth(cfg.LocalAPIKey),
	})
}

func (c *Control) codexApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	model := strings.TrimSpace(body.Model)
	if model != "" {
		if !validCodexModel(model) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "模型名无效：仅支持 gpt-5*/codex 系列（如 gpt-5.6-sol、gpt-5.4-high、gpt-5.3-codex）"})
			return
		}
		cur := c.settings.Get()
		if cur.CodexModel != model {
			cur.CodexModel = model
			if err := c.settings.Save(cur); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		}
	}
	mgr, err := codexcfg.New("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	cfg := c.settings.Get()
	if err := mgr.Apply(c.codexBaseURL(), cfg.LocalAPIKey, c.codexModel()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	c.codexStatus(w, r)
}

func (c *Control) codexRestore(w http.ResponseWriter, r *http.Request) {
	mgr, err := codexcfg.New("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := mgr.Restore(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	c.codexStatus(w, r)
}

// codexFiles returns the on-disk contents of ~/.codex/config.toml and
// auth.json alongside the defaults the app would write, so the UI can offer
// manual editing with a known-good starting point.
func (c *Control) codexFiles(w http.ResponseWriter, r *http.Request) {
	mgr, err := codexcfg.New("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	config, auth, err := mgr.ReadFiles()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	st, _ := mgr.Status("", "")
	cfg := c.settings.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"config_path":    st.ConfigPath,
		"auth_path":      st.AuthPath,
		"config_content": config,
		"auth_content":   auth,
		"config_default": codexcfg.RenderConfig(c.codexBaseURL(), c.codexModel()),
		"auth_default":   codexcfg.RenderAuth(cfg.LocalAPIKey),
	})
}

// codexWriteFiles writes user-edited config.toml / auth.json contents after
// validating them (auth.json must be a JSON object; config.toml must declare
// a model_provider). Empty fields are left untouched.
func (c *Control) codexWriteFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Config string `json:"config"`
		Auth   string `json:"auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.Config) == "" && strings.TrimSpace(body.Auth) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "config 和 auth 至少需要提供一项"})
		return
	}
	if strings.TrimSpace(body.Auth) != "" {
		if err := codexcfg.ValidateAuth(body.Auth); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "auth.json 不是合法的 JSON 对象: " + err.Error()})
			return
		}
	}
	if strings.TrimSpace(body.Config) != "" {
		if err := codexcfg.ValidateConfig(body.Config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
	}
	mgr, err := codexcfg.New("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := mgr.WriteFiles(body.Config, body.Auth); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	c.codexFiles(w, r)
}
