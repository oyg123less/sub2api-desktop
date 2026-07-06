package control

import (
	"net/http"
	"strconv"

	"sub2api-desktop/core/internal/codexcfg"
)

// codexBaseURL builds the gateway base URL clients should target. Codex always
// connects over loopback regardless of the LAN listen setting.
func (c *Control) codexBaseURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(c.server.Port()) + "/v1"
}

func (c *Control) codexStatus(w http.ResponseWriter, r *http.Request) {
	mgr, err := codexcfg.New("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	st, err := mgr.Status()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	base := c.codexBaseURL()
	cfg := c.settings.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"config_path":    st.ConfigPath,
		"auth_path":      st.AuthPath,
		"applied":        st.Applied,
		"config_exists":  st.ConfigExists,
		"backup_exists":  st.BackupExists,
		"base_url":       base,
		"model":          codexcfg.DefaultModel,
		"config_preview": codexcfg.RenderConfig(base, codexcfg.DefaultModel),
		"auth_preview":   codexcfg.RenderAuth(cfg.LocalAPIKey),
	})
}

func (c *Control) codexApply(w http.ResponseWriter, r *http.Request) {
	mgr, err := codexcfg.New("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	cfg := c.settings.Get()
	if err := mgr.Apply(c.codexBaseURL(), cfg.LocalAPIKey, codexcfg.DefaultModel); err != nil {
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
