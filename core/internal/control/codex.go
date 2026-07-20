package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	cfg := c.settings.Get()
	if strings.TrimSpace(cfg.LocalAPIKey) == "" {
		writeControlError(w, http.StatusServiceUnavailable, "gateway_api_key_missing", "Configure a local API key before connecting Codex.", false, nil)
		return
	}
	if !c.server.Running() {
		if err := c.server.Start(); err != nil {
			writeControlError(w, http.StatusConflict, "gateway_port_conflict", err.Error(), false,
				map[string]any{"port": c.server.Port()})
			return
		}
	}
	if err := c.verifyCodexGateway(r.Context(), cfg.LocalAPIKey, c.codexModel()); err != nil {
		var verification *codexGatewayVerificationError
		if errors.As(err, &verification) {
			writeControlError(w, verification.status, verification.code, verification.message, verification.retryable,
				map[string]any{"port": c.server.Port(), "base_url": c.codexBaseURL()})
			return
		}
		writeControlError(w, http.StatusBadGateway, "gateway_health_check_failed", err.Error(), true, nil)
		return
	}
	mgr, err := codexcfg.New("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := mgr.Apply(c.codexBaseURL(), cfg.LocalAPIKey, c.codexModel()); err != nil {
		writeControlError(w, http.StatusInternalServerError, "codex_config_write_failed", err.Error(), false, nil)
		return
	}
	applied, err := mgr.Status(c.codexBaseURL(), cfg.LocalAPIKey)
	if err != nil || !applied.Applied || applied.Stale || strings.TrimSpace(applied.Model) != c.codexModel() {
		writeControlError(w, http.StatusInternalServerError, "codex_config_verification_failed", "Codex configuration was written but could not be verified.", false, nil)
		return
	}
	c.codexStatus(w, r)
}

type codexGatewayVerificationError struct {
	status    int
	code      string
	message   string
	retryable bool
}

func (e *codexGatewayVerificationError) Error() string { return e.message }

func (c *Control) verifyCodexGateway(parent context.Context, apiKey, model string) error {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", c.server.Port())
	healthRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	healthResponse, err := client.Do(healthRequest)
	if err != nil {
		return &codexGatewayVerificationError{status: http.StatusBadGateway, code: "gateway_health_check_failed", message: "Amber gateway did not become ready.", retryable: true}
	}
	defer healthResponse.Body.Close()
	if healthResponse.StatusCode != http.StatusOK {
		return &codexGatewayVerificationError{status: http.StatusBadGateway, code: "gateway_health_check_failed", message: "The gateway health check returned an unexpected status.", retryable: true}
	}
	var health struct {
		Service    string `json:"service"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(io.LimitReader(healthResponse.Body, 64*1024)).Decode(&health); err != nil || health.Service != "amber-gateway" {
		return &codexGatewayVerificationError{status: http.StatusConflict, code: "gateway_port_conflict", message: "The configured port is occupied by a service that is not Amber."}
	}
	if identity, ok := c.server.(interface{ InstanceID() string }); ok && identity.InstanceID() != "" && health.InstanceID != identity.InstanceID() {
		return &codexGatewayVerificationError{status: http.StatusConflict, code: "gateway_port_conflict", message: "The configured port belongs to another Amber instance."}
	}
	modelsRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.codexBaseURL()+"/models", nil)
	modelsRequest.Header.Set("Authorization", "Bearer "+apiKey)
	modelsResponse, err := client.Do(modelsRequest)
	if err != nil {
		return &codexGatewayVerificationError{status: http.StatusBadGateway, code: "gateway_health_check_failed", message: "Amber gateway model verification failed.", retryable: true}
	}
	defer modelsResponse.Body.Close()
	if modelsResponse.StatusCode != http.StatusOK {
		return &codexGatewayVerificationError{status: http.StatusBadGateway, code: "gateway_health_check_failed", message: "The local API key could not access the Amber model catalog."}
	}
	var catalog struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(modelsResponse.Body, 2*1024*1024)).Decode(&catalog); err != nil || len(catalog.Data) == 0 {
		return &codexGatewayVerificationError{status: http.StatusBadGateway, code: "gateway_health_check_failed", message: "Amber returned an invalid model catalog.", retryable: true}
	}
	for _, item := range catalog.Data {
		if item.ID == model {
			return nil
		}
	}
	return &codexGatewayVerificationError{status: http.StatusBadRequest, code: "codex_model_unavailable", message: "The selected Codex model is not available in the local gateway catalog."}
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
