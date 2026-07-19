package cloudsync

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxCloudResponseBytes   = 4 * 1024 * 1024
	amberClientVersion      = "0.4.4"
	amberUserAgent          = "Amber/" + amberClientVersion
	defaultCloudPrimaryURL  = "https://api.amberapp.asia"
	defaultCloudFallbackURL = "https://amber-cloud-api.484486528.workers.dev"
)

type CloudError struct {
	Status         int
	Code           string
	Message        string
	Stage          string
	Retryable      bool
	Attempt        int
	MinimumVersion string
	LatestVersion  string
	UpdateURL      string
	Details        map[string]any
}

func (e *CloudError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "Amber Cloud request failed"
}

type cloudClient struct {
	baseURL     string
	baseURLs    []string
	mu          sync.RWMutex
	http        *http.Client
	configErr   error
	retryDelays []time.Duration
}

func newCloudClient(baseURL string, httpClient *http.Client) (*cloudClient, error) {
	baseURLs, err := parseCloudBaseURLs(baseURL)
	if err != nil {
		return nil, err
	}
	if len(baseURLs) == 0 {
		return &cloudClient{http: httpClient}, nil
	}
	if httpClient == nil {
		httpClient = newDefaultCloudHTTPClient()
	}
	return &cloudClient{
		baseURL:     baseURLs[0],
		baseURLs:    baseURLs,
		http:        httpClient,
		retryDelays: []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond},
	}, nil
}

func parseCloudBaseURLs(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' })
	candidates := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		candidate := strings.TrimRight(strings.TrimSpace(part), "/")
		if candidate == "" {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Host == "" || parsed.User != nil {
			return nil, errors.New("invalid Amber Cloud URL")
		}
		if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost")) {
			return nil, errors.New("Amber Cloud URL must use HTTPS")
		}
		candidates = appendUniqueEndpoint(candidates, candidate)
	}
	if len(candidates) == 1 && (strings.EqualFold(candidates[0], defaultCloudPrimaryURL) || strings.EqualFold(candidates[0], defaultCloudFallbackURL)) {
		return []string{defaultCloudPrimaryURL, defaultCloudFallbackURL}, nil
	}
	return candidates, nil
}

func appendUniqueEndpoint(values []string, candidate string) []string {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return values
		}
	}
	return append(values, candidate)
}

func (c *cloudClient) configured() bool { return c != nil && c.endpoint() != "" }

func (c *cloudClient) endpoint() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

func (c *cloudClient) endpoints() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.baseURLs...)
}

func (c *cloudClient) useEndpoint(endpoint string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, candidate := range c.baseURLs {
		if strings.EqualFold(candidate, endpoint) {
			changed := !strings.EqualFold(c.baseURL, candidate)
			c.baseURL = candidate
			return changed
		}
	}
	return false
}

func (c *cloudClient) rotateEndpoint() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.baseURLs) < 2 {
		return false
	}
	for index, candidate := range c.baseURLs {
		if strings.EqualFold(candidate, c.baseURL) {
			next := c.baseURLs[(index+1)%len(c.baseURLs)]
			changed := !strings.EqualFold(next, c.baseURL)
			c.baseURL = next
			return changed
		}
	}
	c.baseURL = c.baseURLs[0]
	return true
}

func (c *cloudClient) usingFallback() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.baseURLs) > 1 && !strings.EqualFold(c.baseURL, c.baseURLs[0])
}

func (c *cloudClient) trustedEndpoint(target *url.URL) bool {
	if target == nil {
		return false
	}
	for _, candidate := range c.endpoints() {
		parsed, err := url.Parse(candidate)
		if err == nil && target.Scheme == parsed.Scheme && strings.EqualFold(target.Host, parsed.Host) {
			return true
		}
	}
	return false
}

func (c *cloudClient) setHTTPClient(httpClient *http.Client, configErr error) {
	c.mu.Lock()
	c.http = httpClient
	c.configErr = configErr
	c.mu.Unlock()
}

func (c *cloudClient) httpClient() (*http.Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.configErr != nil {
		return nil, c.configErr
	}
	if c.http == nil {
		return nil, errors.New("Amber Cloud HTTP client is unavailable")
	}
	return c.http, nil
}

func (c *cloudClient) do(request *http.Request) (*http.Response, error) {
	httpClient, err := c.httpClient()
	if err != nil {
		return nil, err
	}
	return httpClient.Do(request)
}

type registerRequest struct {
	Email           string `json:"email"`
	TurnstileToken  string `json:"turnstile_token"`
	AuthHash        string `json:"auth_hash"`
	SaltKDF         string `json:"salt_kdf"`
	SaltAuth        string `json:"salt_auth"`
	WrappedVaultKey string `json:"wrapped_vault_key"`
}

type loginParameters struct {
	SaltKDF  string `json:"salt_kdf"`
	SaltAuth string `json:"salt_auth"`
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
		Code           string `json:"code"`
		Message        string `json:"message"`
		MinimumVersion string `json:"minimum_version"`
		LatestVersion  string `json:"latest_version"`
		UpdateURL      string `json:"update_url"`
	} `json:"error"`
	Conflicts []remoteVaultItem `json:"conflicts"`
}

type AdminUser struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	EmailVerified int    `json:"email_verified"`
	Banned        int    `json:"banned"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	LastActiveAt  string `json:"last_active_at"`
	VaultCount    int    `json:"vault_count"`
}

type AdminSetting struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	UpdatedAt string `json:"updated_at"`
}

type AdminAudit struct {
	ID          int64  `json:"id"`
	ActorUserID int64  `json:"actor_user_id"`
	Action      string `json:"action"`
	TargetType  string `json:"target_type"`
	TargetID    string `json:"target_id"`
	Details     string `json:"details"`
	CreatedAt   string `json:"created_at"`
}

type AdminStats struct {
	Users            int     `json:"users"`
	DailyActiveUsers int     `json:"daily_active_users"`
	VaultItems       int     `json:"vault_items"`
	ActiveShares     int     `json:"active_shares"`
	ShareRequests    int     `json:"share_requests"`
	ShareErrorRate   float64 `json:"share_error_rate"`
}

type AdminShare struct {
	ID            int64  `json:"id"`
	OwnerID       int64  `json:"owner_id"`
	OwnerEmail    string `json:"owner_email"`
	AccountUID    string `json:"account_uid"`
	ShareCode     string `json:"share_code"`
	QuotaRequests int    `json:"quota_requests"`
	UsedRequests  int    `json:"used_requests"`
	ExpiresAt     string `json:"expires_at"`
	Revoked       int    `json:"revoked"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type AdminConnectEndpoint struct {
	PublicID       string `json:"public_id"`
	OwnerID        int64  `json:"owner_id"`
	OwnerEmail     string `json:"owner_email"`
	Status         string `json:"status"`
	GroupStatus    string `json:"group_status"`
	AccountCount   int    `json:"account_count"`
	RecipientCount int    `json:"recipient_count"`
	WindowStatus   string `json:"window_status"`
	MaxClaims      int    `json:"max_claims"`
	ClaimedCount   int    `json:"claimed_count"`
	ExpiresAt      string `json:"expires_at"`
	UpdatedAt      string `json:"updated_at"`
}

type Share struct {
	ID            int64  `json:"id"`
	AccountUID    string `json:"account_uid"`
	ShareCode     string `json:"share_code"`
	QuotaRequests int    `json:"quota_requests"`
	UsedRequests  int    `json:"used_requests"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	Revoked       bool   `json:"revoked"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	BaseURL       string `json:"base_url"`
}

type ShareUsage struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"ts"`
	Model     string `json:"model"`
	Status    int    `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
}

type shareCredential struct {
	Token            string `json:"token"`
	AccountType      string `json:"account_type"`
	UpstreamURL      string `json:"upstream_url"`
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
}

type createShareRequest struct {
	AccountUID    string          `json:"account_uid"`
	Credential    shareCredential `json:"credential"`
	QuotaRequests int             `json:"quota_requests"`
	ExpiresAt     string          `json:"expires_at,omitempty"`
	Consent       bool            `json:"consent"`
}

type createShareResponse struct {
	Share    Share  `json:"share"`
	GuestKey string `json:"guest_key"`
}

type AdminOverview struct {
	Users            []AdminUser            `json:"users"`
	Shares           []AdminShare           `json:"shares"`
	ConnectEndpoints []AdminConnectEndpoint `json:"connect_endpoints"`
	Settings         []AdminSetting         `json:"settings"`
	Audit            []AdminAudit           `json:"audit"`
	Stats            AdminStats             `json:"stats"`
}

func (c *cloudClient) register(ctx context.Context, request registerRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/register", "", request, nil)
}

func (c *cloudClient) verifyEmail(ctx context.Context, email, code string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/verify-email", "", map[string]string{"email": email, "code": code}, nil)
}

func (c *cloudClient) resendVerification(ctx context.Context, email string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/resend-verification", "", map[string]string{"email": email}, nil)
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

func (c *cloudClient) push(ctx context.Context, accessToken, idempotencyKey string, items []remoteVaultItem) (pushResponse, error) {
	var response pushResponse
	err := c.doJSONWithHeaders(ctx, http.MethodPut, "/v1/vault/batch", accessToken, map[string]any{"items": items}, &response,
		map[string]string{"Idempotency-Key": idempotencyKey})
	var cloudErr *CloudError
	if errors.As(err, &cloudErr) && cloudErr.Status == http.StatusConflict {
		return response, err
	}
	return response, err
}

func (c *cloudClient) listShares(ctx context.Context, accessToken string) ([]Share, error) {
	var response struct {
		Shares []Share `json:"shares"`
	}
	err := c.doJSON(ctx, http.MethodGet, "/v1/shares", accessToken, nil, &response)
	return response.Shares, err
}

func (c *cloudClient) createShare(ctx context.Context, accessToken string, request createShareRequest) (createShareResponse, error) {
	var response createShareResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/shares", accessToken, request, &response)
	return response, err
}

func (c *cloudClient) updateShare(ctx context.Context, accessToken string, shareID int64, updates map[string]any) (Share, error) {
	var response struct {
		Share Share `json:"share"`
	}
	err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/v1/shares/%d", shareID), accessToken, updates, &response)
	return response.Share, err
}

func (c *cloudClient) shareUsage(ctx context.Context, accessToken string, shareID int64) ([]ShareUsage, error) {
	var response struct {
		Usage []ShareUsage `json:"usage"`
	}
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/shares/%d/usage", shareID), accessToken, nil, &response)
	return response.Usage, err
}

func (c *cloudClient) doJSON(ctx context.Context, method, path, accessToken string, body, output any) error {
	return c.doJSONWithHeaders(ctx, method, path, accessToken, body, output, nil)
}

func (c *cloudClient) doAdminJSON(ctx context.Context, method, path, accessToken, adminKey string, body, output any) error {
	return c.doJSONWithHeaders(ctx, method, path, accessToken, body, output, map[string]string{"X-Admin-Key": adminKey})
}

func (c *cloudClient) doJSONWithHeaders(ctx context.Context, method, path, accessToken string, body, output any, extraHeaders map[string]string) error {
	if !c.configured() {
		return &CloudError{Status: http.StatusServiceUnavailable, Code: "cloud_not_configured", Message: "Amber Cloud is not configured", Stage: "local"}
	}
	if _, err := c.httpClient(); err != nil {
		return &CloudError{Status: http.StatusServiceUnavailable, Code: "cloud_proxy_missing", Message: err.Error(), Stage: "local", Retryable: false}
	}
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	attempts := 1
	if method == http.MethodGet || method == http.MethodHead || (method == http.MethodPut && extraHeaders["Idempotency-Key"] != "") {
		attempts += len(c.retryDelays)
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(encoded)
		}
		request, err := http.NewRequestWithContext(ctx, method, c.endpoint()+path, reader)
		if err != nil {
			return err
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", amberUserAgent)
		request.Header.Set("X-Amber-Client-Version", amberClientVersion)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if accessToken != "" {
			request.Header.Set("Authorization", "Bearer "+accessToken)
		}
		for key, value := range extraHeaders {
			request.Header.Set(key, value)
		}
		response, err := c.do(request)
		if err != nil {
			cloudErr := cloudTransportError(err, attempt)
			c.rotateEndpoint()
			if attempt < attempts && sleepContext(ctx, c.retryDelay(attempt, 0)) == nil {
				continue
			}
			return cloudErr
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, maxCloudResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			cloudErr := &CloudError{Status: response.StatusCode, Code: "cloud_response_failed", Message: "Amber Cloud returned an unreadable response", Stage: "response", Retryable: true, Attempt: attempt}
			if attempt < attempts && sleepContext(ctx, c.retryDelay(attempt, 0)) == nil {
				continue
			}
			return cloudErr
		}
		if len(data) > maxCloudResponseBytes {
			return &CloudError{Status: response.StatusCode, Code: "cloud_response_too_large", Message: "Amber Cloud returned an oversized response", Stage: "response", Attempt: attempt}
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
			cloudErr := &CloudError{Status: response.StatusCode, Code: code, Message: message, Stage: "http", Retryable: response.StatusCode >= 500 || response.StatusCode == 429, Attempt: attempt,
				MinimumVersion: payload.Error.MinimumVersion, LatestVersion: payload.Error.LatestVersion, UpdateURL: payload.Error.UpdateURL}
			if response.StatusCode >= 500 {
				c.rotateEndpoint()
			}
			if cloudErr.Retryable && attempt < attempts && sleepContext(ctx, c.retryDelay(attempt, retryAfter(response.Header.Get("Retry-After")))) == nil {
				continue
			}
			return cloudErr
		}
		if output != nil && len(data) != 0 {
			if err := json.Unmarshal(data, output); err != nil {
				return &CloudError{Status: response.StatusCode, Code: "cloud_invalid_response", Message: "Amber Cloud returned an invalid response", Stage: "response", Attempt: attempt}
			}
		}
		return nil
	}
	return &CloudError{Code: "cloud_unreachable", Message: "Amber Cloud is unreachable", Stage: "network", Retryable: true, Attempt: attempts}
}

func (c *cloudClient) retryDelay(attempt int, serverDelay time.Duration) time.Duration {
	if serverDelay > 0 {
		return min(serverDelay, 30*time.Second)
	}
	index := attempt - 1
	if index < 0 || index >= len(c.retryDelays) {
		return 0
	}
	base := c.retryDelays[index]
	if base <= 0 {
		return 0
	}
	jitter := rand.Int64N(max(1, int64(base/5)))
	return base + time.Duration(jitter)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(date))
	}
	return 0
}

func cloudTransportError(err error, attempt int) *CloudError {
	stage := cloudTransportStage(err)
	code, message := "cloud_unreachable", "Amber Cloud is unreachable"
	switch stage {
	case "dns":
		code, message = "cloud_dns_failed", "Amber Cloud DNS lookup failed"
	case "connect":
		code, message = "cloud_connect_failed", "Amber Cloud connection failed"
	case "tls":
		code, message = "cloud_tls_failed", "Amber Cloud secure connection failed"
	case "timeout":
		code, message = "cloud_timeout", "Amber Cloud connection timed out"
	}
	return &CloudError{Code: code, Message: message, Stage: stage, Retryable: true, Attempt: attempt}
}

func cloudTransportStage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	var certificateErr x509.UnknownAuthorityError
	if errors.As(err, &certificateErr) {
		return "tls"
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return "tls"
	}
	var recordErr tls.RecordHeaderError
	if errors.As(err, &recordErr) {
		return "tls"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "tls") || strings.Contains(message, "certificate") || strings.Contains(message, "handshake") {
		return "tls"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && (opErr.Op == "dial" || opErr.Op == "connect") {
		return "connect"
	}
	return "network"
}

func (c *cloudClient) adminOverview(ctx context.Context, accessToken, adminKey string) (AdminOverview, error) {
	var users struct {
		Users []AdminUser `json:"users"`
	}
	var settings struct {
		Settings []AdminSetting `json:"settings"`
	}
	var audit struct {
		Audit []AdminAudit `json:"audit"`
	}
	var shares struct {
		Shares []AdminShare `json:"shares"`
	}
	var connectEndpoints struct {
		ConnectEndpoints []AdminConnectEndpoint `json:"connect_endpoints"`
	}
	var stats AdminStats
	for _, request := range []struct {
		path   string
		output any
	}{
		{"/v1/admin/users?limit=100", &users},
		{"/v1/admin/shares?limit=100", &shares},
		{"/v1/admin/connect-endpoints?limit=100", &connectEndpoints},
		{"/v1/admin/settings", &settings},
		{"/v1/admin/stats", &stats},
		{"/v1/admin/audit?limit=50", &audit},
	} {
		if err := c.doAdminJSON(ctx, http.MethodGet, request.path, accessToken, adminKey, nil, request.output); err != nil {
			return AdminOverview{}, err
		}
	}
	return AdminOverview{Users: users.Users, Shares: shares.Shares, ConnectEndpoints: connectEndpoints.ConnectEndpoints,
		Settings: settings.Settings, Audit: audit.Audit, Stats: stats}, nil
}

func (c *cloudClient) adminSetUserBanned(ctx context.Context, accessToken, adminKey string, userID int64, banned bool) error {
	return c.doAdminJSON(ctx, http.MethodPatch, fmt.Sprintf("/v1/admin/users/%d", userID), accessToken, adminKey, map[string]bool{"banned": banned}, nil)
}

func (c *cloudClient) adminLogoutUser(ctx context.Context, accessToken, adminKey string, userID int64) error {
	return c.doAdminJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/admin/users/%d/logout-all", userID), accessToken, adminKey, map[string]any{}, nil)
}

func (c *cloudClient) adminDeleteUser(ctx context.Context, accessToken, adminKey string, userID int64) error {
	return c.doAdminJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/admin/users/%d", userID), accessToken, adminKey, map[string]string{"confirm": "DELETE"}, nil)
}

func (c *cloudClient) adminUpdateSettings(ctx context.Context, accessToken, adminKey string, settings map[string]bool) error {
	return c.doAdminJSON(ctx, http.MethodPatch, "/v1/admin/settings", accessToken, adminKey, settings, nil)
}

func (c *cloudClient) adminSetShareRevoked(ctx context.Context, accessToken, adminKey string, shareID int64, revoked bool) error {
	return c.doAdminJSON(ctx, http.MethodPatch, fmt.Sprintf("/v1/admin/shares/%d", shareID), accessToken, adminKey, map[string]bool{"revoked": revoked}, nil)
}
