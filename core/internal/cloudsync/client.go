package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxCloudResponseBytes = 4 * 1024 * 1024

type CloudError struct {
	Status    int
	Code      string
	Message   string
	Retryable bool
}

func (e *CloudError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "Amber Cloud request failed"
}

type cloudClient struct {
	baseURL string
	http    *http.Client
}

func newCloudClient(baseURL string, httpClient *http.Client) (*cloudClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return &cloudClient{http: httpClient}, nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("invalid Amber Cloud URL")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost")) {
		return nil, errors.New("Amber Cloud URL must use HTTPS")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &cloudClient{baseURL: baseURL, http: httpClient}, nil
}

func (c *cloudClient) configured() bool { return c != nil && c.baseURL != "" }

type registerRequest struct {
	Email           string `json:"email"`
	TurnstileToken  string `json:"turnstile_token"`
	AuthHash        string `json:"auth_hash"`
	SaltKDF         string `json:"salt_kdf"`
	SaltAuth        string `json:"salt_auth"`
	WrappedVaultKey string `json:"wrapped_vault_key"`
}

type loginParameters struct {
	SaltKDF         string `json:"salt_kdf"`
	SaltAuth        string `json:"salt_auth"`
	WrappedVaultKey string `json:"wrapped_vault_key"`
}

type cloudUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type loginResponse struct {
	AccessToken     string    `json:"access_token"`
	AccessExpiresIn int       `json:"access_expires_in"`
	RefreshToken    string    `json:"refresh_token"`
	User            cloudUser `json:"user"`
	SaltKDF         string    `json:"salt_kdf"`
	SaltAuth        string    `json:"salt_auth"`
	WrappedVaultKey string    `json:"wrapped_vault_key"`
}

type refreshResponse struct {
	AccessToken     string `json:"access_token"`
	AccessExpiresIn int    `json:"access_expires_in"`
	RefreshToken    string `json:"refresh_token"`
}

type remoteVaultItem struct {
	Kind       string `json:"kind"`
	ClientUID  string `json:"client_uid"`
	Ciphertext string `json:"ciphertext"`
	Version    int    `json:"version"`
	Deleted    bool   `json:"deleted"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type pullResponse struct {
	Items  []remoteVaultItem `json:"items"`
	Cursor string            `json:"cursor"`
}

type pushResponse struct {
	Items     []remoteVaultItem `json:"items"`
	Cursor    string            `json:"cursor"`
	Conflicts []remoteVaultItem `json:"conflicts"`
}

type cloudErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Conflicts []remoteVaultItem `json:"conflicts"`
}

func (c *cloudClient) register(ctx context.Context, request registerRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/register", "", request, nil)
}

func (c *cloudClient) verifyEmail(ctx context.Context, email, code string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/verify-email", "", map[string]string{"email": email, "code": code}, nil)
}

func (c *cloudClient) parameters(ctx context.Context, email string) (loginParameters, error) {
	var response loginParameters
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/parameters", "", map[string]string{"email": email}, &response)
	return response, err
}

func (c *cloudClient) login(ctx context.Context, email, authHash string) (loginResponse, error) {
	var response loginResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/login", "", map[string]string{"email": email, "auth_hash": authHash}, &response)
	return response, err
}

func (c *cloudClient) refresh(ctx context.Context, token string) (refreshResponse, error) {
	var response refreshResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/refresh", "", map[string]string{"refresh_token": token}, &response)
	return response, err
}

func (c *cloudClient) logout(ctx context.Context, token string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/logout", "", map[string]string{"refresh_token": token}, nil)
}

func (c *cloudClient) changePassword(ctx context.Context, accessToken, currentAuthHash string, request registerRequest) error {
	body := map[string]string{
		"current_auth_hash": currentAuthHash,
		"auth_hash":         request.AuthHash,
		"salt_kdf":          request.SaltKDF,
		"salt_auth":         request.SaltAuth,
		"wrapped_vault_key": request.WrappedVaultKey,
	}
	return c.doJSON(ctx, http.MethodPut, "/v1/auth/master-password", accessToken, body, nil)
}

func (c *cloudClient) pull(ctx context.Context, accessToken, cursor string) (pullResponse, error) {
	path := "/v1/vault"
	if cursor != "" {
		path += "?since=" + url.QueryEscape(cursor)
	}
	var response pullResponse
	err := c.doJSON(ctx, http.MethodGet, path, accessToken, nil, &response)
	return response, err
}

func (c *cloudClient) push(ctx context.Context, accessToken string, items []remoteVaultItem) (pushResponse, error) {
	var response pushResponse
	err := c.doJSON(ctx, http.MethodPut, "/v1/vault/batch", accessToken, map[string]any{"items": items}, &response)
	var cloudErr *CloudError
	if errors.As(err, &cloudErr) && cloudErr.Status == http.StatusConflict {
		return response, err
	}
	return response, err
}

func (c *cloudClient) doJSON(ctx context.Context, method, path, accessToken string, body, output any) error {
	if !c.configured() {
		return &CloudError{Status: http.StatusServiceUnavailable, Code: "cloud_not_configured", Message: "Amber Cloud is not configured"}
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Amber/0.3.0")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return &CloudError{Code: "cloud_unreachable", Message: "Amber Cloud is unreachable", Retryable: true}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCloudResponseBytes+1))
	if err != nil {
		return &CloudError{Status: response.StatusCode, Code: "cloud_response_failed", Message: "Amber Cloud returned an unreadable response", Retryable: true}
	}
	if len(data) > maxCloudResponseBytes {
		return &CloudError{Status: response.StatusCode, Code: "cloud_response_too_large", Message: "Amber Cloud returned an oversized response"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload cloudErrorResponse
		_ = json.Unmarshal(data, &payload)
		if output != nil && response.StatusCode == http.StatusConflict {
			_ = json.Unmarshal(data, output)
		}
		code := payload.Error.Code
		if code == "" {
			code = fmt.Sprintf("cloud_http_%d", response.StatusCode)
		}
		message := payload.Error.Message
		if message == "" {
			message = "Amber Cloud request failed"
		}
		return &CloudError{Status: response.StatusCode, Code: code, Message: message, Retryable: response.StatusCode >= 500 || response.StatusCode == 429}
	}
	if output != nil && len(data) != 0 {
		if err := json.Unmarshal(data, output); err != nil {
			return &CloudError{Status: response.StatusCode, Code: "cloud_invalid_response", Message: "Amber Cloud returned an invalid response"}
		}
	}
	return nil
}
