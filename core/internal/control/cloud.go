package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sub2api-desktop/core/internal/cloudsync"
)

const maxCloudControlBody = 64 * 1024

func (c *Control) cloudStatus(w http.ResponseWriter, _ *http.Request) {
	if c.cloud == nil {
		writeJSON(w, http.StatusOK, cloudsync.Status{Configured: false})
		return
	}
	writeJSON(w, http.StatusOK, c.cloud.Status())
}

func (c *Control) cloudRegister(w http.ResponseWriter, r *http.Request) {
	if c.cloud == nil {
		writeControlError(w, http.StatusServiceUnavailable, "cloud_unavailable", "Amber Cloud is unavailable", true, nil)
		return
	}
	var request struct {
		Email                string `json:"email"`
		Password             string `json:"password"`
		TurnstileToken       string `json:"turnstile_token"`
		RecoveryAcknowledged bool   `json:"recovery_acknowledged"`
	}
	if !decodeCloudRequest(w, r, &request) {
		return
	}
	if !request.RecoveryAcknowledged {
		writeControlError(w, http.StatusBadRequest, "recovery_acknowledgement_required", "Confirm that a forgotten master password cannot recover cloud data", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := c.cloud.Register(ctx, cloudsync.RegisterInput{
		Email: request.Email, Password: request.Password, TurnstileToken: request.TurnstileToken,
	}); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "verification_required": true})
}

func (c *Control) cloudVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if c.cloud == nil || !decodeCloudRequest(w, r, &request) {
		if c.cloud == nil {
			writeControlError(w, http.StatusServiceUnavailable, "cloud_unavailable", "Amber Cloud is unavailable", true, nil)
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := c.cloud.VerifyEmail(ctx, request.Email, request.Code); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.cloud.Status())
}

func (c *Control) cloudLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if c.cloud == nil || !decodeCloudRequest(w, r, &request) {
		if c.cloud == nil {
			writeControlError(w, http.StatusServiceUnavailable, "cloud_unavailable", "Amber Cloud is unavailable", true, nil)
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := c.cloud.Login(ctx, request.Email, request.Password); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.cloud.Status())
}

func (c *Control) cloudLogout(w http.ResponseWriter, r *http.Request) {
	if c.cloud == nil {
		writeControlError(w, http.StatusServiceUnavailable, "cloud_unavailable", "Amber Cloud is unavailable", true, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := c.cloud.Logout(ctx); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.cloud.Status())
}

func (c *Control) cloudSync(w http.ResponseWriter, r *http.Request) {
	if c.cloud == nil {
		writeControlError(w, http.StatusServiceUnavailable, "cloud_unavailable", "Amber Cloud is unavailable", true, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := c.cloud.Sync(ctx); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.cloud.Status())
}

func (c *Control) cloudChangePassword(w http.ResponseWriter, r *http.Request) {
	var request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if c.cloud == nil || !decodeCloudRequest(w, r, &request) {
		if c.cloud == nil {
			writeControlError(w, http.StatusServiceUnavailable, "cloud_unavailable", "Amber Cloud is unavailable", true, nil)
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := c.cloud.ChangePassword(ctx, request.CurrentPassword, request.NewPassword); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c.cloud.Status())
}

func (c *Control) cloudAdminOverview(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AdminKey string `json:"admin_key"`
	}
	if !c.decodeCloudAdminRequest(w, r, &request) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	overview, err := c.cloud.AdminOverview(ctx, request.AdminKey)
	if err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (c *Control) cloudAdminSetUserBanned(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AdminKey string `json:"admin_key"`
		Banned   *bool  `json:"banned"`
	}
	if !c.decodeCloudAdminRequest(w, r, &request) {
		return
	}
	userID, ok := cloudAdminUserID(w, r)
	if !ok {
		return
	}
	if request.Banned == nil {
		writeControlError(w, http.StatusBadRequest, "invalid_admin_action", "The banned field is required", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := c.cloud.AdminSetUserBanned(ctx, request.AdminKey, userID, *request.Banned); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (c *Control) cloudAdminLogoutUser(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AdminKey string `json:"admin_key"`
	}
	if !c.decodeCloudAdminRequest(w, r, &request) {
		return
	}
	userID, ok := cloudAdminUserID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := c.cloud.AdminLogoutUser(ctx, request.AdminKey, userID); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (c *Control) cloudAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AdminKey string `json:"admin_key"`
		Confirm  string `json:"confirm"`
	}
	if !c.decodeCloudAdminRequest(w, r, &request) {
		return
	}
	if request.Confirm != "DELETE" {
		writeControlError(w, http.StatusBadRequest, "delete_confirmation_required", "Type DELETE to confirm user deletion", false, nil)
		return
	}
	userID, ok := cloudAdminUserID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := c.cloud.AdminDeleteUser(ctx, request.AdminKey, userID); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (c *Control) cloudAdminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AdminKey     string `json:"admin_key"`
		Registration *bool  `json:"registration_enabled"`
		InviteMode   *bool  `json:"invite_mode"`
	}
	if !c.decodeCloudAdminRequest(w, r, &request) {
		return
	}
	settings := make(map[string]bool, 2)
	if request.Registration != nil {
		settings["registration_enabled"] = *request.Registration
	}
	if request.InviteMode != nil {
		settings["invite_mode"] = *request.InviteMode
	}
	if len(settings) == 0 {
		writeControlError(w, http.StatusBadRequest, "invalid_platform_settings", "No supported settings were provided", false, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := c.cloud.AdminUpdateSettings(ctx, request.AdminKey, settings); err != nil {
		writeCloudControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (c *Control) decodeCloudAdminRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if c.cloud == nil {
		writeControlError(w, http.StatusServiceUnavailable, "cloud_unavailable", "Amber Cloud is unavailable", true, nil)
		return false
	}
	if !decodeCloudRequest(w, r, target) {
		return false
	}
	return true
}

func cloudAdminUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeControlError(w, http.StatusBadRequest, "invalid_user_id", "The user ID is invalid", false, nil)
		return 0, false
	}
	return id, true
}

func decodeCloudRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxCloudControlBody+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeControlError(w, http.StatusBadRequest, "invalid_request", "Invalid cloud request", false, nil)
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeControlError(w, http.StatusBadRequest, "invalid_request", "Invalid cloud request", false, nil)
		return false
	}
	return true
}

func writeCloudControlError(w http.ResponseWriter, err error) {
	var cloudErr *cloudsync.CloudError
	if errors.As(err, &cloudErr) {
		status := cloudErr.Status
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		writeControlError(w, status, cloudErr.Code, cloudErr.Message, cloudErr.Retryable, nil)
		return
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "password"), strings.Contains(message, "registration session"), strings.Contains(message, "login is required"), strings.Contains(message, "administrator"):
		writeControlError(w, http.StatusBadRequest, "cloud_validation_failed", err.Error(), false, nil)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeControlError(w, http.StatusGatewayTimeout, "cloud_timeout", "Amber Cloud request timed out", true, nil)
	default:
		writeControlError(w, http.StatusInternalServerError, "cloud_operation_failed", "The cloud operation could not be completed", true, nil)
	}
}
