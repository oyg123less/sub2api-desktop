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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

type CloudProfile struct {
	DisplayName          string `json:"display_name"`
	FriendCode           string `json:"friend_code"`
	EncryptionPublicKey  string `json:"encryption_public_key"`
	EncryptionKeyVersion int    `json:"encryption_key_version"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

type profileWire struct {
	CloudProfile
	EncryptionPrivateCipher string `json:"encryption_private_cipher"`
}

type profileResponse struct {
	Profile    *profileWire `json:"profile"`
	NeedsSetup bool         `json:"needs_setup"`
}

type CloudFriend struct {
	PublicID             string `json:"public_id"`
	DisplayName          string `json:"display_name"`
	FriendCode           string `json:"friend_code"`
	EncryptionPublicKey  string `json:"encryption_public_key"`
	EncryptionKeyVersion int    `json:"encryption_key_version"`
	Alias                string `json:"alias"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

type CloudFriendRequest struct {
	PublicID    string `json:"public_id"`
	Status      string `json:"status"`
	Direction   string `json:"direction"`
	DisplayName string `json:"display_name"`
	FriendCode  string `json:"friend_code"`
	CreatedAt   string `json:"created_at"`
	RespondedAt string `json:"responded_at,omitempty"`
	ExpiresAt   string `json:"expires_at"`
}

type CloudFriendsResponse struct {
	Friends []CloudFriend `json:"friends"`
}

type CloudFriendRequestsResponse struct {
	Requests []CloudFriendRequest `json:"requests"`
}

type CloudDocument map[string]any

type ShareGroupAccountSelection struct {
	AccountID int64  `json:"account_id"`
	RelayMode string `json:"relay_mode"`
	Priority  int    `json:"priority,omitempty"`
	Weight    int    `json:"weight,omitempty"`
}

type ShareGroupRecipientSelection struct {
	FriendshipID     string `json:"friendship_id"`
	RPMLimit         int    `json:"rpm_limit,omitempty"`
	ConcurrencyLimit int    `json:"concurrency_limit,omitempty"`
	QuotaRequests    int    `json:"quota_requests,omitempty"`
	ExpiresAt        string `json:"expires_at,omitempty"`
}

type CreateShareGroupInput struct {
	IdempotencyKey       string                         `json:"idempotency_key"`
	Name                 string                         `json:"name"`
	Description          string                         `json:"description"`
	RoutePolicy          string                         `json:"route_policy"`
	DefaultRPM           int                            `json:"default_rpm"`
	DefaultConcurrency   int                            `json:"default_concurrency"`
	DefaultQuotaRequests int                            `json:"default_quota_requests"`
	DefaultExpiresAt     string                         `json:"default_expires_at"`
	Accounts             []ShareGroupAccountSelection   `json:"accounts"`
	Recipients           []ShareGroupRecipientSelection `json:"recipients"`
}

type CloudReceivedKeyInfo struct {
	PublicID            string `json:"public_id"`
	KeyPrefix           string `json:"key_prefix"`
	KeyVersion          int    `json:"key_version"`
	KeyEnvelope         string `json:"key_envelope,omitempty"`
	EnvelopeContext     string `json:"envelope_context,omitempty"`
	RecipientKeyVersion int    `json:"recipient_key_version,omitempty"`
	Status              string `json:"status"`
}

type CloudReceivedShare struct {
	PublicID string `json:"public_id"`
	Status   string `json:"status"`
	Group    struct {
		PublicID            string `json:"public_id"`
		Name                string `json:"name"`
		Description         string `json:"description"`
		Status              string `json:"status"`
		RoutePolicy         string `json:"route_policy"`
		AccountCount        int    `json:"account_count"`
		OwnerDeviceRequired bool   `json:"owner_device_required"`
	} `json:"group"`
	Owner struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	RPMLimit         int                       `json:"rpm_limit"`
	ConcurrencyLimit int                       `json:"concurrency_limit"`
	QuotaRequests    int                       `json:"quota_requests"`
	UsedRequests     int                       `json:"used_requests"`
	ExpiresAt        string                    `json:"expires_at,omitempty"`
	CreatedAt        string                    `json:"created_at"`
	AcceptedAt       string                    `json:"accepted_at,omitempty"`
	BaseURL          string                    `json:"base_url"`
	Key              *CloudReceivedKeyInfo     `json:"key,omitempty"`
	APIKey           string                    `json:"api_key,omitempty"`
	LocalEnabled     bool                      `json:"local_enabled"`
	ProxyID          *int64                    `json:"proxy_id,omitempty"`
	HealthStatus     string                    `json:"health_status"`
	HealthMessage    string                    `json:"health_message,omitempty"`
	LastCheckedAt    string                    `json:"last_checked_at,omitempty"`
	ConnectionTest   *CloudShareConnectionTest `json:"connection_test,omitempty"`
}

type CloudReceivedSharesResponse struct {
	Shares []CloudReceivedShare `json:"shares"`
}

type CloudShareConnectionTest struct {
	OK      bool   `json:"ok"`
	Status  int    `json:"status"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type CloudDevicesResponse struct {
	Devices      []CloudDevice `json:"devices"`
	RelayEnabled bool          `json:"relay_enabled"`
}

type CloudWorkspaceResponse struct {
	Profile        CloudProfile                `json:"profile"`
	Friends        CloudFriendsResponse        `json:"friends"`
	FriendRequests CloudFriendRequestsResponse `json:"friend_requests"`
	ShareGroups    CloudDocument               `json:"share_groups"`
	ReceivedShares CloudReceivedSharesResponse `json:"received_shares"`
	Devices        CloudDevicesResponse        `json:"devices"`
	ConnectHost    CloudDocument               `json:"connect_host"`
}

type CloudDevice struct {
	PublicID     string         `json:"public_id"`
	Name         string         `json:"name"`
	Capabilities []string       `json:"capabilities"`
	IsPrimary    bool           `json:"is_primary"`
	Revoked      bool           `json:"revoked"`
	Online       bool           `json:"online"`
	LastSeenAt   string         `json:"last_seen_at,omitempty"`
	Relay        map[string]any `json:"relay,omitempty"`
}

func (c *cloudClient) getProfile(ctx context.Context, accessToken string) (profileResponse, error) {
	var response profileResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/profile", accessToken, nil, &response)
	return response, err
}

func (c *cloudClient) putProfile(ctx context.Context, accessToken string, body any) (profileResponse, error) {
	var response profileResponse
	err := c.doJSON(ctx, http.MethodPut, "/v1/profile", accessToken, body, &response)
	return response, err
}

func (c *cloudClient) patchProfile(ctx context.Context, accessToken, displayName string) (profileResponse, error) {
	var response profileResponse
	err := c.doJSON(ctx, http.MethodPatch, "/v1/profile", accessToken, map[string]string{"display_name": displayName}, &response)
	return response, err
}

func (c *cloudClient) friends(ctx context.Context, accessToken string) (CloudFriendsResponse, error) {
	var response CloudFriendsResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/friends", accessToken, nil, &response)
	return response, err
}

func (c *cloudClient) friendRequests(ctx context.Context, accessToken string) (CloudFriendRequestsResponse, error) {
	var response CloudFriendRequestsResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/friend-requests", accessToken, nil, &response)
	return response, err
}

func (c *cloudClient) document(ctx context.Context, method, path, accessToken string, body any, headers map[string]string) (CloudDocument, error) {
	var response CloudDocument
	err := c.doJSONWithHeaders(ctx, method, path, accessToken, body, &response, headers)
	return response, err
}

func (c *cloudClient) receivedShares(ctx context.Context, accessToken string) (CloudReceivedSharesResponse, error) {
	var response CloudReceivedSharesResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/received-shares", accessToken, nil, &response)
	return response, err
}

func (c *cloudClient) receivedAction(ctx context.Context, accessToken, shareID, action string) (CloudDocument, error) {
	return c.document(ctx, http.MethodPost, fmt.Sprintf("/v1/received-shares/%s/%s", shareID, action), accessToken, nil, nil)
}

func (c *cloudClient) receivedEnvelope(ctx context.Context, accessToken, shareID string) (CloudReceivedKeyInfo, error) {
	var response struct {
		Key CloudReceivedKeyInfo `json:"key"`
	}
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/received-shares/%s/key-envelope", shareID), accessToken, nil, &response)
	return response.Key, err
}

func (m *Manager) cloudV2Access(ctx context.Context) (string, int64, []byte, error) {
	if err := m.ensureAccess(ctx); err != nil {
		return "", 0, nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil || len(m.vaultKey) != keySize {
		return "", 0, nil, errors.New("cloud login is required")
	}
	return m.accessToken, m.session.UserID, append([]byte(nil), m.vaultKey...), nil
}

func safeDeviceName() string {
	name, _ := os.Hostname()
	name = strings.TrimSpace(name)
	if len(name) < 2 {
		return "Amber device"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}

func safeDisplayName(email string) string {
	name := strings.TrimSpace(strings.SplitN(email, "@", 2)[0])
	if len(name) < 2 {
		name = "Amber user"
	}
	if len(name) > 40 {
		name = name[:40]
	}
	return name
}

func (m *Manager) ensureProfileLocked(ctx context.Context, displayName string) (CloudProfile, error) {
	accessToken, userID, vaultKey, err := m.cloudV2Access(ctx)
	if err != nil {
		return CloudProfile{}, err
	}
	defer wipe(vaultKey)
	remote, err := m.client.getProfile(ctx, accessToken)
	if err != nil {
		return CloudProfile{}, err
	}
	local, localErr := m.store.LoadCloudIdentity(userID)
	if localErr != nil && !errors.Is(localErr, store.ErrNotFound) {
		return CloudProfile{}, localErr
	}
	if remote.Profile != nil {
		plaintext, err := decryptVaultItem(vaultKey, remote.Profile.EncryptionPrivateCipher)
		if err != nil {
			return CloudProfile{}, errors.New("cloud identity key could not be decrypted")
		}
		privateKey := string(plaintext)
		wipe(plaintext)
		publicKey, err := identityPublicKey(privateKey)
		if err != nil || publicKey != remote.Profile.EncryptionPublicKey {
			return CloudProfile{}, errors.New("cloud identity key does not match the public profile")
		}
		if local == nil {
			devicePrivate, devicePublic, err := generateDeviceKeyPair()
			if err != nil {
				return CloudProfile{}, err
			}
			local = &store.CloudIdentity{UserID: userID, DeviceName: safeDeviceName(), DevicePrivateKey: devicePrivate, DevicePublicKey: devicePublic}
		}
		local.X25519PrivateKey = privateKey
		local.X25519PublicKey = publicKey
		if err := m.store.SaveCloudIdentity(*local); err != nil {
			return CloudProfile{}, err
		}
		return remote.Profile.CloudProfile, nil
	}
	if local == nil {
		privateKey, publicKey, err := generateIdentityKeyPair()
		if err != nil {
			return CloudProfile{}, err
		}
		devicePrivate, devicePublic, err := generateDeviceKeyPair()
		if err != nil {
			return CloudProfile{}, err
		}
		local = &store.CloudIdentity{
			UserID: userID, X25519PrivateKey: privateKey, X25519PublicKey: publicKey,
			DevicePrivateKey: devicePrivate, DevicePublicKey: devicePublic, DeviceName: safeDeviceName(),
		}
		if err := m.store.SaveCloudIdentity(*local); err != nil {
			return CloudProfile{}, err
		}
	}
	if displayName = strings.TrimSpace(displayName); displayName == "" {
		m.mu.RLock()
		email := m.session.Email
		m.mu.RUnlock()
		displayName = safeDisplayName(email)
	}
	privateCipher, err := encryptVaultItem(vaultKey, []byte(local.X25519PrivateKey))
	if err != nil {
		return CloudProfile{}, err
	}
	created, err := m.client.putProfile(ctx, accessToken, map[string]any{
		"display_name": displayName, "encryption_public_key": local.X25519PublicKey,
		"encryption_private_cipher": privateCipher,
	})
	if err != nil {
		return CloudProfile{}, err
	}
	if created.Profile == nil {
		return CloudProfile{}, errors.New("Amber Cloud returned an invalid profile")
	}
	return created.Profile.CloudProfile, nil
}

func (m *Manager) EnsureProfile(ctx context.Context, displayName string) (CloudProfile, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	profile, err := m.ensureProfileLocked(ctx, displayName)
	if err != nil {
		m.setError(err)
		return CloudProfile{}, err
	}
	m.clearError()
	return profile, nil
}

// LoadWorkspace gets one authenticated snapshot for the Cloud workspace. It
// holds the operation lock once, then runs independent remote reads in
// parallel so the UI does not pay one network round trip per resource.
func (m *Manager) LoadWorkspace(ctx context.Context) (CloudWorkspaceResponse, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	profile, err := m.ensureProfileLocked(ctx, "")
	if err != nil {
		return CloudWorkspaceResponse{}, err
	}
	accessToken, userID, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return CloudWorkspaceResponse{}, err
	}

	result := CloudWorkspaceResponse{Profile: profile}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var group sync.WaitGroup
	var errorMu sync.Mutex
	var firstErr error
	run := func(read func() error) {
		group.Add(1)
		go func() {
			defer group.Done()
			if readErr := read(); readErr != nil {
				errorMu.Lock()
				if firstErr == nil {
					firstErr = readErr
					cancel()
				}
				errorMu.Unlock()
			}
		}()
	}
	// v0.4.4 keeps legacy friend/share APIs server-side for compatibility, but
	// the desktop workspace no longer loads them into the primary experience.
	result.Friends = CloudFriendsResponse{Friends: []CloudFriend{}}
	result.FriendRequests = CloudFriendRequestsResponse{Requests: []CloudFriendRequest{}}
	result.ShareGroups = CloudDocument{"groups": []any{}}
	run(func() error {
		var readErr error
		result.ReceivedShares, readErr = m.client.receivedShares(workCtx, accessToken)
		return readErr
	})
	run(func() error {
		return m.client.doJSON(workCtx, http.MethodGet, "/v1/devices", accessToken, nil, &result.Devices)
	})
	run(func() error {
		var readErr error
		result.ConnectHost, readErr = m.getConnectHostLocked(workCtx, accessToken, userID)
		return readErr
	})
	group.Wait()
	if firstErr != nil {
		return CloudWorkspaceResponse{}, firstErr
	}
	if identity, identityErr := m.store.LoadCloudIdentity(userID); identityErr == nil {
		result.Devices.RelayEnabled = identity.RelayEnabled
	}
	if err := m.hydrateReceivedShares(ctx, accessToken, userID, &result.ReceivedShares); err != nil {
		return CloudWorkspaceResponse{}, err
	}
	return result, nil
}

func (m *Manager) UpdateProfile(ctx context.Context, displayName string) (CloudProfile, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, _, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return CloudProfile{}, err
	}
	response, err := m.client.patchProfile(ctx, accessToken, strings.TrimSpace(displayName))
	if err != nil || response.Profile == nil {
		if err == nil {
			err = errors.New("Amber Cloud returned an invalid profile")
		}
		return CloudProfile{}, err
	}
	return response.Profile.CloudProfile, nil
}

func (m *Manager) ListFriends(ctx context.Context) (CloudFriendsResponse, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, _, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return CloudFriendsResponse{}, err
	}
	return m.client.friends(ctx, accessToken)
}

func (m *Manager) ListFriendRequests(ctx context.Context) (CloudFriendRequestsResponse, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, _, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return CloudFriendRequestsResponse{}, err
	}
	return m.client.friendRequests(ctx, accessToken)
}

func (m *Manager) FriendAction(ctx context.Context, method, path string, body any) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, _, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return nil, err
	}
	return m.client.document(ctx, method, path, accessToken, body, nil)
}

func (m *Manager) CreateShareGroup(ctx context.Context, input CreateShareGroupInput) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if _, err := m.ensureProfileLocked(ctx, ""); err != nil {
		return nil, err
	}
	accessToken, _, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return nil, err
	}
	friends, err := m.client.friends(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	friendByID := make(map[string]CloudFriend, len(friends.Friends))
	for _, friend := range friends.Friends {
		friendByID[friend.PublicID] = friend
	}
	accounts := make([]map[string]any, 0, len(input.Accounts))
	for _, selected := range input.Accounts {
		account, err := m.store.GetAccount(selected.AccountID)
		if err != nil {
			return nil, err
		}
		if account.ClientUID == "" {
			return nil, errors.New("sync every selected account before sharing")
		}
		relayMode := selected.RelayMode
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
	recipients := make([]map[string]any, 0, len(input.Recipients))
	for _, selected := range input.Recipients {
		friend, ok := friendByID[selected.FriendshipID]
		if !ok {
			return nil, errors.New("a selected friend is no longer available")
		}
		material, _, err := createGuestKeyMaterial(friend.EncryptionPublicKey, friend.EncryptionKeyVersion)
		if err != nil {
			return nil, err
		}
		entry := map[string]any{"friendship_id": selected.FriendshipID, "key_material": material}
		if selected.RPMLimit > 0 {
			entry["rpm_limit"] = selected.RPMLimit
		}
		if selected.ConcurrencyLimit > 0 {
			entry["concurrency_limit"] = selected.ConcurrencyLimit
		}
		if selected.QuotaRequests > 0 {
			entry["quota_requests"] = selected.QuotaRequests
		}
		if selected.ExpiresAt != "" {
			entry["expires_at"] = selected.ExpiresAt
		}
		recipients = append(recipients, entry)
	}
	body := map[string]any{
		"name": input.Name, "description": input.Description, "route_policy": input.RoutePolicy,
		"default_rpm": input.DefaultRPM, "default_concurrency": input.DefaultConcurrency,
		"default_quota_requests": input.DefaultQuotaRequests, "accounts": accounts, "recipients": recipients,
	}
	if input.DefaultExpiresAt != "" {
		body["default_expires_at"] = input.DefaultExpiresAt
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = uuid.NewString()
	}
	return m.client.document(ctx, http.MethodPost, "/v1/share-groups", accessToken, body,
		map[string]string{"Idempotency-Key": input.IdempotencyKey})
}

func (m *Manager) ShareGroupRequest(ctx context.Context, method, path string, body any, idempotencyKey string) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, _, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return nil, err
	}
	headers := map[string]string(nil)
	if idempotencyKey != "" {
		headers = map[string]string{"Idempotency-Key": idempotencyKey}
	}
	return m.client.document(ctx, method, path, accessToken, body, headers)
}

func (m *Manager) ListShareGroups(ctx context.Context) (CloudDocument, error) {
	return m.ShareGroupRequest(ctx, http.MethodGet, "/v1/share-groups", nil, "")
}

func (m *Manager) ListReceivedShares(ctx context.Context) (CloudReceivedSharesResponse, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if _, err := m.ensureProfileLocked(ctx, ""); err != nil {
		return CloudReceivedSharesResponse{}, err
	}
	accessToken, userID, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return CloudReceivedSharesResponse{}, err
	}
	response, err := m.client.receivedShares(ctx, accessToken)
	if err != nil {
		return CloudReceivedSharesResponse{}, err
	}
	if err := m.hydrateReceivedShares(ctx, accessToken, userID, &response); err != nil {
		return CloudReceivedSharesResponse{}, err
	}
	return response, nil
}

func (m *Manager) hydrateReceivedShares(ctx context.Context, accessToken string, userID int64, response *CloudReceivedSharesResponse) error {
	identity, err := m.store.LoadCloudIdentity(userID)
	if err != nil {
		return err
	}
	for index := range response.Shares {
		share := &response.Shares[index]
		if share.Status != "active" && share.Status != "paused" {
			_ = m.store.DeleteCloudReceivedAccountLink(userID, share.PublicID)
			_ = m.store.DeleteCloudReceivedKey(userID, share.PublicID)
			continue
		}
		local, localErr := m.store.LoadCloudReceivedKey(userID, share.PublicID)
		if localErr == nil && (share.Key == nil || local.KeyVersion >= share.Key.KeyVersion) {
			share.APIKey = local.GuestKey
			continue
		}
		key, envelopeErr := m.client.receivedEnvelope(ctx, accessToken, share.PublicID)
		if envelopeErr != nil {
			continue
		}
		guestKey, openErr := openGuestKeyEnvelope(identity.X25519PrivateKey, key.KeyEnvelope, key.EnvelopeContext, key.RecipientKeyVersion)
		if openErr != nil {
			continue
		}
		if err := m.store.SaveCloudReceivedKey(store.CloudReceivedKey{
			UserID: userID, GrantPublicID: share.PublicID, KeyVersion: key.KeyVersion, KeyPrefix: key.KeyPrefix,
			BaseURL: share.BaseURL, GuestKey: guestKey,
		}); err != nil {
			return err
		}
		share.APIKey = guestKey
	}
	links, err := m.store.ListCloudReceivedAccountLinks(userID)
	if err != nil {
		return err
	}
	linkByGrant := make(map[string]store.CloudReceivedAccountLink, len(links))
	for _, link := range links {
		linkByGrant[link.GrantPublicID] = link
	}
	for index := range response.Shares {
		share := &response.Shares[index]
		if share.Status == "active" || share.Status == "paused" {
			if err := m.saveReceivedLink(userID, *share, true); err != nil {
				return err
			}
		}
		if link, ok := linkByGrant[share.PublicID]; ok {
			share.LocalEnabled = link.Enabled
			share.ProxyID = link.ProxyID
			share.HealthStatus = link.HealthStatus
			share.HealthMessage = link.HealthMessage
			if !link.LastCheckedAt.IsZero() {
				share.LastCheckedAt = link.LastCheckedAt.Format(time.RFC3339)
			}
		} else if share.Status == "active" || share.Status == "paused" {
			share.LocalEnabled = true
			share.HealthStatus = "unchecked"
		}
	}
	return nil
}

func (m *Manager) AcceptReceivedShare(ctx context.Context, shareID string) (CloudReceivedShare, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if _, err := m.ensureProfileLocked(ctx, ""); err != nil {
		return CloudReceivedShare{}, err
	}
	accessToken, userID, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return CloudReceivedShare{}, err
	}
	response, err := m.client.receivedAction(ctx, accessToken, shareID, "accept")
	if err != nil {
		return CloudReceivedShare{}, err
	}
	raw, err := json.Marshal(response["share"])
	if err != nil {
		return CloudReceivedShare{}, err
	}
	var share CloudReceivedShare
	if err := json.Unmarshal(raw, &share); err != nil || share.Key == nil {
		return CloudReceivedShare{}, errors.New("Amber Cloud returned an invalid share")
	}
	identity, err := m.store.LoadCloudIdentity(userID)
	if err != nil {
		return CloudReceivedShare{}, err
	}
	guestKey, err := openGuestKeyEnvelope(identity.X25519PrivateKey, share.Key.KeyEnvelope, share.Key.EnvelopeContext, share.Key.RecipientKeyVersion)
	if err != nil {
		return CloudReceivedShare{}, err
	}
	if err := m.store.SaveCloudReceivedKey(store.CloudReceivedKey{
		UserID: userID, GrantPublicID: share.PublicID, KeyVersion: share.Key.KeyVersion, KeyPrefix: share.Key.KeyPrefix,
		BaseURL: share.BaseURL, GuestKey: guestKey,
	}); err != nil {
		return CloudReceivedShare{}, err
	}
	share.APIKey = guestKey
	if err := m.saveReceivedLink(userID, share, true); err != nil {
		return CloudReceivedShare{}, err
	}
	share.LocalEnabled = true
	return share, nil
}

func (m *Manager) ReceivedShareAction(ctx context.Context, shareID, action string) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, userID, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return nil, err
	}
	response, err := m.client.receivedAction(ctx, accessToken, shareID, action)
	if err == nil && (action == "leave" || action == "decline") {
		_ = m.store.DeleteCloudReceivedAccountLink(userID, shareID)
		_ = m.store.DeleteCloudReceivedKey(userID, shareID)
	}
	return response, err
}

// testReceivedKey performs one explicit, low-output request through a received
// Guest Key. It never retries because the upstream may already have started.
func (m *Manager) testReceivedKey(ctx context.Context, key *store.CloudReceivedKey, proxyID *int64, model string) (CloudShareConnectionTest, error) {
	target, err := url.Parse(strings.TrimRight(key.BaseURL, "/") + "/responses")
	if err != nil || target.User != nil || !m.client.trustedEndpoint(target) {
		return CloudShareConnectionTest{}, errors.New("the shared Base URL is not trusted")
	}
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 128 || strings.ContainsAny(model, "\r\n\t") {
		model = openai.DefaultTestModel
	}
	payload, _ := json.Marshal(map[string]any{
		"model": model,
		"input": []map[string]any{{
			"type": "message",
			"role": "user",
			"content": []map[string]string{{
				"type": "input_text",
				"text": "Reply with OK.",
			}},
		}},
		"stream": true,
		"store":  false,
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(payload))
	if err != nil {
		return CloudShareConnectionTest{}, err
	}
	request.Header.Set("Authorization", "Bearer "+key.GuestKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("User-Agent", amberUserAgent)
	request.Header.Set("X-Amber-Client-Version", amberClientVersion)
	var response *http.Response
	if proxyID == nil {
		response, err = m.client.do(request)
	} else {
		selected, proxyErr := m.store.GetProxy(*proxyID)
		if proxyErr != nil {
			return CloudShareConnectionTest{OK: false, Code: "proxy_unavailable", Message: "The selected proxy is unavailable."}, nil
		}
		client, _, proxyErr := newConfiguredCloudHTTPClient(m.client.endpoint(), store.CloudConnectionSettings{
			Mode: store.CloudConnectionProxy, ProxyID: proxyID,
		}, selected)
		if proxyErr != nil {
			return CloudShareConnectionTest{OK: false, Code: "proxy_unavailable", Message: "The selected proxy is unavailable."}, nil
		}
		response, err = client.Do(request)
	}
	if err != nil {
		return CloudShareConnectionTest{OK: false, Code: "share_test_unreachable", Message: "The shared gateway is unreachable."}, nil
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 256*1024))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return CloudShareConnectionTest{OK: true, Status: response.StatusCode, Message: "The shared connection is available."}, nil
	}
	var failure struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &failure)
	if failure.Error.Code == "" {
		failure.Error.Code = "share_test_failed"
	}
	if failure.Error.Message == "" {
		failure.Error.Message = http.StatusText(response.StatusCode)
	}
	return CloudShareConnectionTest{OK: false, Status: response.StatusCode, Code: failure.Error.Code, Message: failure.Error.Message}, nil
}

func (m *Manager) TestReceivedShare(ctx context.Context, shareID, model string) (CloudShareConnectionTest, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	_, userID, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err != nil {
		return CloudShareConnectionTest{}, err
	}
	shareID = strings.TrimSpace(shareID)
	key, err := m.store.LoadCloudReceivedKey(userID, shareID)
	if err != nil {
		return CloudShareConnectionTest{}, errors.New("sync and accept this shared access before testing it")
	}
	account, err := m.store.GetCloudReceivedAccountByGrant(userID, shareID)
	if err != nil {
		return CloudShareConnectionTest{}, errors.New("sync and accept this shared access before testing it")
	}
	result, testErr := m.testReceivedKey(ctx, key, account.ProxyID, model)
	if testErr != nil {
		result = CloudShareConnectionTest{OK: false, Code: "share_test_failed", Message: testErr.Error()}
	}
	if err := m.store.UpdateCloudReceivedAccountHealth(userID, shareID, result.OK, result.Message); err != nil && !errors.Is(err, store.ErrNotFound) {
		return CloudShareConnectionTest{}, err
	}
	return result, nil
}

func (m *Manager) ListDevices(ctx context.Context) (CloudDevicesResponse, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, userID, vaultKey, err := m.cloudV2Access(ctx)
	if err != nil {
		return CloudDevicesResponse{}, err
	}
	wipe(vaultKey)
	var response CloudDevicesResponse
	err = m.client.doJSON(ctx, http.MethodGet, "/v1/devices", accessToken, nil, &response)
	if err == nil {
		if identity, identityErr := m.store.LoadCloudIdentity(userID); identityErr == nil {
			response.RelayEnabled = identity.RelayEnabled
		}
	}
	return response, err
}

func (m *Manager) EnsureDevice(ctx context.Context) (CloudDevice, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.ensureDeviceLocked(ctx)
}

func (m *Manager) ensureDeviceLocked(ctx context.Context) (CloudDevice, error) {
	if _, err := m.ensureProfileLocked(ctx, ""); err != nil {
		return CloudDevice{}, err
	}
	accessToken, userID, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return CloudDevice{}, err
	}
	identity, err := m.store.LoadCloudIdentity(userID)
	if err != nil {
		return CloudDevice{}, err
	}
	if identity.DevicePublicID != "" {
		devices, err := m.client.document(ctx, http.MethodGet, "/v1/devices", accessToken, nil, nil)
		if err == nil {
			raw, _ := json.Marshal(devices)
			var list CloudDevicesResponse
			if json.Unmarshal(raw, &list) == nil {
				for _, device := range list.Devices {
					if device.PublicID == identity.DevicePublicID && !device.Revoked {
						return device, nil
					}
				}
			}
		}
	}
	response, err := m.client.document(ctx, http.MethodPost, "/v1/devices", accessToken, map[string]any{
		"name": identity.DeviceName, "device_public_key": identity.DevicePublicKey,
		"capabilities": []string{"oauth", "api_key", "proxy", "streaming"},
	}, nil)
	if err != nil {
		return CloudDevice{}, err
	}
	raw, _ := json.Marshal(response["device"])
	var device CloudDevice
	if err := json.Unmarshal(raw, &device); err != nil || device.PublicID == "" {
		return CloudDevice{}, errors.New("Amber Cloud returned an invalid device")
	}
	if err := m.store.UpdateCloudDevice(userID, device.PublicID, identity.RelayEnabled); err != nil {
		return CloudDevice{}, err
	}
	return device, nil
}

func (m *Manager) SetRelayEnabled(ctx context.Context, enabled bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	_, userID, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return err
	}
	identity, err := m.store.LoadCloudIdentity(userID)
	if err != nil {
		return err
	}
	if identity.DevicePublicID == "" && enabled {
		return errors.New("register this device before enabling owner relay")
	}
	if err := m.store.UpdateCloudDevice(userID, identity.DevicePublicID, enabled); err != nil {
		return err
	}
	if !enabled {
		m.closeRelaySession()
	}
	return nil
}

func (m *Manager) RotateRecipientKey(ctx context.Context, groupID, recipientID, friendshipID, idempotencyKey string) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, _, _, err := m.cloudV2Access(ctx)
	if err != nil {
		return nil, err
	}
	friends, err := m.client.friends(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	var target *CloudFriend
	for index := range friends.Friends {
		if friends.Friends[index].PublicID == friendshipID {
			target = &friends.Friends[index]
			break
		}
	}
	if target == nil {
		return nil, errors.New("the selected friend is no longer available")
	}
	material, _, err := createGuestKeyMaterial(target.EncryptionPublicKey, target.EncryptionKeyVersion)
	if err != nil {
		return nil, err
	}
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	return m.client.document(ctx, http.MethodPost,
		fmt.Sprintf("/v1/share-groups/%s/recipients/%s/keys/rotate", groupID, recipientID), accessToken,
		map[string]any{"key_material": material}, map[string]string{"Idempotency-Key": idempotencyKey})
}

func (m *Manager) AddShareGroupAccount(ctx context.Context, groupID string, selected ShareGroupAccountSelection) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, _, vaultKey, err := m.cloudV2Access(ctx)
	if err != nil {
		return nil, err
	}
	wipe(vaultKey)
	account, err := m.store.GetAccount(selected.AccountID)
	if err != nil {
		return nil, err
	}
	if account.ClientUID == "" {
		return nil, errors.New("sync the selected account before sharing")
	}
	relayMode := selected.RelayMode
	if relayMode == "" {
		relayMode = "owner_device"
	}
	body := map[string]any{
		"account_uid": account.ClientUID, "account_type": string(account.AccountType), "relay_mode": relayMode,
		"priority": selected.Priority, "weight": selected.Weight,
	}
	if selected.Priority == 0 {
		body["priority"] = 100
	}
	if selected.Weight == 0 {
		body["weight"] = 100
	}
	if relayMode == "worker_direct" {
		if account.AccountType != store.AccountTypeAPIKey || strings.TrimSpace(account.APIKey) == "" {
			return nil, errors.New("Worker-direct sharing requires an API-key account")
		}
		body["credential"] = shareCredential{Token: account.APIKey, AccountType: string(account.AccountType), UpstreamURL: account.BaseURL}
	}
	return m.client.document(ctx, http.MethodPost, fmt.Sprintf("/v1/share-groups/%s/accounts", groupID), accessToken, body, nil)
}

func (m *Manager) InviteShareGroupRecipients(ctx context.Context, groupID string, selections []ShareGroupRecipientSelection, idempotencyKey string) (CloudDocument, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, _, vaultKey, err := m.cloudV2Access(ctx)
	if err != nil {
		return nil, err
	}
	wipe(vaultKey)
	friends, err := m.client.friends(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	friendByID := make(map[string]CloudFriend, len(friends.Friends))
	for _, friend := range friends.Friends {
		friendByID[friend.PublicID] = friend
	}
	recipients := make([]map[string]any, 0, len(selections))
	for _, selected := range selections {
		friend, ok := friendByID[selected.FriendshipID]
		if !ok {
			return nil, errors.New("a selected friend is no longer available")
		}
		material, _, err := createGuestKeyMaterial(friend.EncryptionPublicKey, friend.EncryptionKeyVersion)
		if err != nil {
			return nil, err
		}
		entry := map[string]any{"friendship_id": selected.FriendshipID, "key_material": material}
		if selected.RPMLimit > 0 {
			entry["rpm_limit"] = selected.RPMLimit
		}
		if selected.ConcurrencyLimit > 0 {
			entry["concurrency_limit"] = selected.ConcurrencyLimit
		}
		if selected.QuotaRequests > 0 {
			entry["quota_requests"] = selected.QuotaRequests
		}
		if selected.ExpiresAt != "" {
			entry["expires_at"] = selected.ExpiresAt
		}
		recipients = append(recipients, entry)
	}
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	return m.client.document(ctx, http.MethodPost, fmt.Sprintf("/v1/share-groups/%s/recipients", groupID), accessToken,
		map[string]any{"recipients": recipients}, map[string]string{"Idempotency-Key": idempotencyKey})
}

func validateCloudV2Path(path string) error {
	if !strings.HasPrefix(path, "/v1/") || strings.Contains(path, "..") || strings.ContainsAny(path, "?#") {
		return errors.New("invalid Amber Cloud path")
	}
	return nil
}

func (m *Manager) CloudV2Request(ctx context.Context, method, path string, body any, idempotencyKey string) (CloudDocument, error) {
	if err := validateCloudV2Path(path); err != nil {
		return nil, err
	}
	return m.ShareGroupRequest(ctx, method, path, body, idempotencyKey)
}
