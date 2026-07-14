package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sub2api-desktop/core/internal/codexcfg"
	"sub2api-desktop/core/internal/codexremote"
)

func (c *Control) codexRemoteTest(w http.ResponseWriter, r *http.Request) {
	if c.remoteCodex == nil {
		writeControlError(w, http.StatusServiceUnavailable, "remote_unavailable", "remote Codex integration is unavailable", true, nil)
		return
	}
	var request codexremote.ProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeControlError(w, http.StatusBadRequest, "invalid_request", err.Error(), false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	probe, err := c.remoteCodex.Probe(ctx, request)
	if err != nil {
		writeCodexRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, probe)
}

func (c *Control) codexRemoteInject(w http.ResponseWriter, r *http.Request) {
	if c.remoteCodex == nil {
		writeControlError(w, http.StatusServiceUnavailable, "remote_unavailable", "remote Codex integration is unavailable", true, nil)
		return
	}
	var request codexremote.InjectRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeControlError(w, http.StatusBadRequest, "invalid_request", err.Error(), false, nil)
		return
	}
	if request.Model == "" {
		request.Model = c.codexModel()
	}
	if !validCodexModel(request.Model) {
		writeControlError(w, http.StatusBadRequest, "invalid_model", "model must belong to the gpt-5 or codex family", false, nil)
		return
	}
	if request.RemotePort == 0 {
		request.RemotePort = 8080
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", request.RemotePort)
	request.Config = codexcfg.RenderConfig(baseURL, request.Model)
	request.Auth = codexcfg.RenderAuth(c.settings.Get().LocalAPIKey)
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	target, err := c.remoteCodex.Inject(ctx, request)
	if err != nil {
		writeCodexRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (c *Control) codexRemoteTargets(w http.ResponseWriter, _ *http.Request) {
	if c.remoteCodex == nil {
		writeControlError(w, http.StatusServiceUnavailable, "remote_unavailable", "remote Codex integration is unavailable", true, nil)
		return
	}
	targets, err := c.remoteCodex.Targets()
	if err != nil {
		writeCodexRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": targets})
}

func (c *Control) codexRemoteTunnel(w http.ResponseWriter, r *http.Request) {
	if c.remoteCodex == nil {
		writeControlError(w, http.StatusServiceUnavailable, "remote_unavailable", "remote Codex integration is unavailable", true, nil)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Enabled == nil {
		writeControlError(w, http.StatusBadRequest, "invalid_request", "enabled must be a boolean", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	target, err := c.remoteCodex.SetTunnel(ctx, id, *body.Enabled)
	if err != nil {
		writeCodexRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (c *Control) codexRemoteRestore(w http.ResponseWriter, r *http.Request) {
	if c.remoteCodex == nil {
		writeControlError(w, http.StatusServiceUnavailable, "remote_unavailable", "remote Codex integration is unavailable", true, nil)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	target, err := c.remoteCodex.Restore(ctx, id)
	if err != nil {
		writeCodexRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (c *Control) codexRemoteDelete(w http.ResponseWriter, r *http.Request) {
	if c.remoteCodex == nil {
		writeControlError(w, http.StatusServiceUnavailable, "remote_unavailable", "remote Codex integration is unavailable", true, nil)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := c.remoteCodex.Delete(id); err != nil {
		writeCodexRemoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeCodexRemoteError(w http.ResponseWriter, err error) {
	var remoteError *codexremote.Error
	if !errors.As(err, &remoteError) {
		writeControlError(w, http.StatusInternalServerError, "remote_failed", "remote Codex operation failed", true, nil)
		return
	}
	status := http.StatusBadGateway
	switch remoteError.Code {
	case "invalid_target", "unsupported_os":
		status = http.StatusBadRequest
	case "auth_failed":
		status = http.StatusUnauthorized
	case "host_key_unknown", "host_key_mismatch":
		status = http.StatusConflict
	case "target_not_found":
		status = http.StatusNotFound
	}
	details := map[string]any(nil)
	if fingerprint := strings.TrimSpace(remoteError.Fingerprint); fingerprint != "" {
		details = map[string]any{"host_key_fingerprint": fingerprint}
	}
	writeControlError(w, status, remoteError.Code, remoteError.Error(), remoteError.Code == "connection_failed", details)
}
