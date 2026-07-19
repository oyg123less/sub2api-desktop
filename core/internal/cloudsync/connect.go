package cloudsync

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"sub2api-desktop/core/internal/store"
)

const connectPasswordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type ConnectHostStartInput struct {
	MaxClaims       int    `json:"max_claims"`
	DurationMinutes int    `json:"duration_minutes"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type ConnectClaimInput struct {
	ConnectionCode string `json:"connection_code"`
	Password       string `json:"password"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ConnectReceivedUpdate struct {
	Enabled  *bool  `json:"enabled"`
	ProxyID  *int64 `json:"proxy_id"`
	SetProxy bool   `json:"set_proxy"`
}

type CloudUserEvent struct {
	ID             int64          `json:"id"`
	EventType      string         `json:"event_type"`
	EntityType     string         `json:"entity_type"`
	EntityPublicID string         `json:"entity_public_id"`
	Payload        map[string]any `json:"payload"`
	CreatedAt      string         `json:"created_at"`
}

type CloudUserEventsResponse struct {
	Events  []CloudUserEvent `json:"events"`
	Cursor  int64            `json:"cursor"`
	HasMore bool             `json:"has_more"`
}

func newConnectPassword() (string, error) {
	result := make([]byte, 6)
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for index, value := range random {
		result[index] = connectPasswordAlphabet[int(value)%len(connectPasswordAlphabet)]
	}
	return string(result), nil
}

func documentValue[T any](document CloudDocument, key string) (T, error) {
	var result T
	raw, err := json.Marshal(document[key])
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (m *Manager) connectAccountBody(selections []ShareGroupAccountSelection) (map[string]any, error) {
	if len(selections) < 1 || len(selections) > 20 {
		return nil, errors.New("select between 1 and 20 accounts")
	}
	accounts := make([]map[string]any, 0, len(selections))
	seen := make(map[int64]bool, len(selections))
	for _, selected := range selections {
		if seen[selected.AccountID] {
			return nil, errors.New("each account can be selected only once")
		}
		seen[selected.AccountID] = true
		account, err := m.store.GetAccount(selected.AccountID)
		if err != nil {
			return nil, err
		}
		if account.ClientUID == "" {
			return nil, errors.New("sync every selected account before sharing")
		}
		relayMode := strings.TrimSpace(selected.RelayMode)
		if relayMode == "" {
			relayMode = "owner_device"
		}
		entry := map[string]any{
			"account_uid": account.ClientUID, "account_type": string(account.AccountType), "relay_mode": relayMode,
			"priority": selected.Priority, "weight": selected.Weight,
		}
		if selected.Priority == 0 {
			entry["priority"] = 100
		}
		if selected.Weight == 0 {
			entry["weight"] = 100
		}
		if relayMode == "worker_direct" {
			if account.AccountType != store.AccountTypeAPIKey || strings.TrimSpace(account.APIKey) == "" {
				return nil, errors.New("Worker-direct sharing requires an API-key account")
			}
			entry["credential"] = shareCredential{Token: account.APIKey, AccountType: string(account.AccountType), UpstreamURL: account.BaseURL}
		}
		accounts = append(accounts, entry)
	}
	return map[string]any{"accounts": accounts}, nil
}

func (m *Manager) getConnectHostLocked(ctx context.Context, accessToken string, userID int64) (CloudDocument, error) {
	document, err := m.client.document(ctx, http.MethodGet, "/v1/connect/host", accessToken, nil, nil)
	if err != nil {
		var cloudErr *CloudError
		if errors.As(err, &cloudErr) && cloudErr.Status == http.StatusNotFound {
			return CloudDocument{"configured": false, "accounts": []any{}, "recipients": []any{}}, nil
		}
		return nil, err
	}
	state, stateErr := m.store.LoadCloudConnectHostState(userID)
	if stateErr == nil && time.Now().Before(state.ExpiresAt) {
		endpoint, _ := documentValue[struct {
			ConnectionCode string `json:"connection_code"`
		}](document, "endpoint")
		window, _ := documentValue[struct {
			PasswordVersion int `json:"password_version"`
		}](document, "window")
		if endpoint.ConnectionCode == state.ConnectionCode && window.PasswordVersion == state.PasswordVersion {
			document["temporary_password"] = state.Password
		}
	}
	return document, nil
}

func (m *Manager) GetConnectHost(ctx context.Context) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, userID, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return nil, err
	}
	return m.getConnectHostLocked(ctx, accessToken, userID)
}

func (m *Manager) ConfigureConnectHostAccounts(ctx context.Context, selections []ShareGroupAccountSelection) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if _, err := m.ensureProfileLocked(ctx, ""); err != nil {
		return nil, err
	}
	accessToken, _, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return nil, err
	}
	body, err := m.connectAccountBody(selections)
	if err != nil {
		return nil, err
	}
	return m.client.document(ctx, http.MethodPut, "/v1/connect/host/accounts", accessToken, body, nil)
}

func (m *Manager) StartConnectHost(ctx context.Context, input ConnectHostStartInput, rotate bool) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, userID, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return nil, err
	}
	password, err := newConnectPassword()
	if err != nil {
		return nil, err
	}
	path := "/v1/connect/host/start"
	if rotate {
		path = "/v1/connect/host/rotate-password"
	}
	body := map[string]any{"password": password, "max_claims": input.MaxClaims, "duration_minutes": input.DurationMinutes}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	document, err := m.client.document(ctx, http.MethodPost, path, accessToken, body,
		map[string]string{"Idempotency-Key": idempotencyKey})
	if err != nil {
		return nil, err
	}
	host, err := documentValue[CloudDocument](document, "host")
	if err != nil {
		return nil, errors.New("Amber Cloud returned an invalid sharing entry")
	}
	endpoint, err := documentValue[struct {
		ConnectionCode string `json:"connection_code"`
	}](host, "endpoint")
	if err != nil || endpoint.ConnectionCode == "" {
		return nil, errors.New("Amber Cloud returned an invalid connection code")
	}
	version, _ := document["password_version"].(float64)
	expiresText, _ := document["expires_at"].(string)
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresText)
	if err != nil {
		return nil, errors.New("Amber Cloud returned an invalid temporary-password expiry")
	}
	if err := m.store.SaveCloudConnectHostState(store.CloudConnectHostState{
		UserID: userID, ConnectionCode: endpoint.ConnectionCode, PasswordVersion: int(version), Password: password, ExpiresAt: expiresAt,
	}); err != nil {
		return nil, err
	}
	host["temporary_password"] = password
	document["host"] = host
	document["temporary_password"] = password
	return document, nil
}

func (m *Manager) ConnectHostAction(ctx context.Context, action string) (CloudDocument, error) {
	if action != "pause" && action != "resume" && action != "reset-code" {
		return nil, errors.New("invalid connection sharing action")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, userID, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return nil, err
	}
	document, err := m.client.document(ctx, http.MethodPost, "/v1/connect/host/"+action, accessToken, nil, nil)
	if err == nil && action == "reset-code" {
		_ = m.store.DeleteCloudConnectHostState(userID)
	}
	return document, err
}

func (m *Manager) ConnectRecipientRequest(ctx context.Context, method, recipientID string, body any) (CloudDocument, error) {
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" || strings.ContainsAny(recipientID, "/?#") {
		return nil, errors.New("invalid connected user")
	}
	path := fmt.Sprintf("/v1/connect/host/recipients/%s", recipientID)
	return m.ShareGroupRequest(ctx, method, path, body, "")
}

func (m *Manager) ListConnectEvents(ctx context.Context, cursor int64) (CloudUserEventsResponse, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if cursor < 0 {
		return CloudUserEventsResponse{}, errors.New("invalid event cursor")
	}
	accessToken, _, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return CloudUserEventsResponse{}, err
	}
	document, err := m.client.document(ctx, http.MethodGet, fmt.Sprintf("/v1/events?cursor=%d&limit=100", cursor), accessToken, nil, nil)
	if err != nil {
		return CloudUserEventsResponse{}, err
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return CloudUserEventsResponse{}, err
	}
	var response CloudUserEventsResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return CloudUserEventsResponse{}, errors.New("Amber Cloud returned invalid events")
	}
	return response, nil
}

func (m *Manager) ClaimConnectAndUse(ctx context.Context, input ConnectClaimInput) (CloudReceivedShare, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	profile, err := m.ensureProfileLocked(ctx, "")
	if err != nil {
		return CloudReceivedShare{}, err
	}
	accessToken, userID, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return CloudReceivedShare{}, err
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = uuid.NewString()
	}
	var material cloudKeyMaterial
	var guestKey string
	attempt, attemptErr := m.store.LoadCloudConnectClaimAttempt(userID, input.IdempotencyKey)
	if attemptErr == nil {
		if attempt.ConnectionCode != strings.TrimSpace(input.ConnectionCode) || attempt.Password != strings.TrimSpace(input.Password) {
			return CloudReceivedShare{}, errors.New("the idempotency key belongs to different connection details")
		}
		if err := json.Unmarshal([]byte(attempt.KeyMaterialJSON), &material); err != nil {
			return CloudReceivedShare{}, errors.New("saved connection attempt is invalid")
		}
		guestKey = attempt.GuestKey
	} else if !errors.Is(attemptErr, store.ErrNotFound) {
		return CloudReceivedShare{}, attemptErr
	} else {
		material, guestKey, err = createGuestKeyMaterial(profile.EncryptionPublicKey, profile.EncryptionKeyVersion)
		if err != nil {
			return CloudReceivedShare{}, err
		}
		materialJSON, _ := json.Marshal(material)
		if err := m.store.SaveCloudConnectClaimAttempt(store.CloudConnectClaimAttempt{
			UserID: userID, IdempotencyKey: input.IdempotencyKey, ConnectionCode: strings.TrimSpace(input.ConnectionCode),
			Password: strings.TrimSpace(input.Password), KeyMaterialJSON: string(materialJSON), GuestKey: guestKey,
		}); err != nil {
			return CloudReceivedShare{}, err
		}
	}
	document, err := m.client.document(ctx, http.MethodPost, "/v1/connect/claim", accessToken, map[string]any{
		"connection_code": input.ConnectionCode, "password": input.Password, "key_material": material,
	}, map[string]string{"Idempotency-Key": input.IdempotencyKey})
	if err != nil {
		return CloudReceivedShare{}, err
	}
	share, err := documentValue[CloudReceivedShare](document, "share")
	if err != nil || share.PublicID == "" || share.Key == nil {
		return CloudReceivedShare{}, errors.New("Amber Cloud returned an invalid connected share")
	}
	if err := m.store.SaveCloudReceivedKey(store.CloudReceivedKey{
		UserID: userID, GrantPublicID: share.PublicID, KeyVersion: share.Key.KeyVersion,
		KeyPrefix: share.Key.KeyPrefix, BaseURL: share.BaseURL, GuestKey: guestKey,
	}); err != nil {
		return CloudReceivedShare{}, err
	}
	if err := m.saveReceivedLink(userID, share, false); err != nil {
		return CloudReceivedShare{}, err
	}
	key, err := m.store.LoadCloudReceivedKey(userID, share.PublicID)
	if err != nil {
		return CloudReceivedShare{}, err
	}
	testResult, testErr := m.testReceivedKey(ctx, key, nil, "")
	if testErr != nil {
		testResult = CloudShareConnectionTest{OK: false, Code: "share_test_failed", Message: testErr.Error()}
	}
	if err := m.store.SetCloudReceivedAccountHealth(userID, share.PublicID, testResult.OK, testResult.OK, testResult.Message); err != nil {
		return CloudReceivedShare{}, err
	}
	_ = m.store.DeleteCloudConnectClaimAttempt(userID, input.IdempotencyKey)
	share.APIKey = guestKey
	share.LocalEnabled = testResult.OK
	share.ConnectionTest = &testResult
	share.HealthMessage = testResult.Message
	share.LastCheckedAt = time.Now().Format(time.RFC3339)
	if testResult.OK {
		share.HealthStatus = "healthy"
	} else {
		share.HealthStatus = "needs_attention"
	}
	return share, nil
}

func (m *Manager) saveReceivedLink(userID int64, share CloudReceivedShare, defaultEnabled bool) error {
	enabled := defaultEnabled
	var proxyID *int64
	if links, err := m.store.ListCloudReceivedAccountLinks(userID); err == nil {
		for _, link := range links {
			if link.GrantPublicID == share.PublicID {
				enabled = link.Enabled
				proxyID = link.ProxyID
				break
			}
		}
	}
	return m.store.SaveCloudReceivedAccountLink(store.CloudReceivedAccountLink{
		UserID: userID, GrantPublicID: share.PublicID, OwnerName: share.Owner.DisplayName,
		GroupName: share.Group.Name, RemoteStatus: share.Status, Enabled: enabled,
		RPMLimit: share.RPMLimit, ConcurrencyLimit: share.ConcurrencyLimit, QuotaRequests: share.QuotaRequests,
		UsedRequests: share.UsedRequests, ProxyID: proxyID,
	})
}

func (m *Manager) UpdateConnectReceived(ctx context.Context, grantID string, update ConnectReceivedUpdate) (store.CloudReceivedAccountLink, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	_, userID, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return store.CloudReceivedAccountLink{}, err
	}
	if err := m.store.SetCloudReceivedAccountLink(userID, strings.TrimSpace(grantID), update.Enabled, update.ProxyID, update.SetProxy); err != nil {
		return store.CloudReceivedAccountLink{}, err
	}
	links, err := m.store.ListCloudReceivedAccountLinks(userID)
	if err != nil {
		return store.CloudReceivedAccountLink{}, err
	}
	for _, link := range links {
		if link.GrantPublicID == grantID {
			return link, nil
		}
	}
	return store.CloudReceivedAccountLink{}, store.ErrNotFound
}
