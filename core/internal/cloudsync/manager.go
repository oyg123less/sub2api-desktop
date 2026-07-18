package cloudsync

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"sub2api-desktop/core/internal/store"
)

type SettingsAccess interface {
	Get() store.Settings
	Save(store.Settings) error
}

const (
	cloudPullPageSize = 1000
	cloudPullMaxPages = 20
	cloudPushMaxItems = 200
	cloudPushMaxBytes = 1024 * 1024
)

type syncOutboxPayload struct {
	Items []remoteVaultItem `json:"items"`
}

type RegisterInput struct {
	Email          string
	Password       string
	TurnstileToken string
}

type CreateShareInput struct {
	AccountID     int64
	QuotaRequests int
	ExpiresAt     string
	Consent       bool
}

type CreatedShare struct {
	Share    Share  `json:"share"`
	GuestKey string `json:"guest_key"`
}

type Status struct {
	Configured          bool                  `json:"configured"`
	Authenticated       bool                  `json:"authenticated"`
	PendingVerification bool                  `json:"pending_verification"`
	Email               string                `json:"email,omitempty"`
	Role                string                `json:"role,omitempty"`
	TurnstileSiteKey    string                `json:"turnstile_site_key,omitempty"`
	LastSyncAt          *time.Time            `json:"last_sync_at,omitempty"`
	LastAttemptAt       *time.Time            `json:"last_attempt_at,omitempty"`
	PendingItems        int                   `json:"pending_items"`
	Syncing             bool                  `json:"syncing"`
	LastError           string                `json:"last_error,omitempty"`
	LastErrorCode       string                `json:"last_error_code,omitempty"`
	LastErrorStage      string                `json:"last_error_stage,omitempty"`
	ConsecutiveFailures int                   `json:"consecutive_failures"`
	NextRetryAt         *time.Time            `json:"next_retry_at,omitempty"`
	Conflicts           []store.CloudConflict `json:"conflicts"`
}

type pendingRegistration struct {
	email    string
	material *authMaterial
}

type pendingRegistrationPayload struct {
	Email    string `json:"email"`
	AuthHash string `json:"auth_hash"`
	VaultKey string `json:"vault_key"`
}

type Manager struct {
	store        *store.Store
	settings     SettingsAccess
	client       *cloudClient
	siteKey      string
	logger       *slog.Logger
	httpOverride *http.Client

	opMu         sync.Mutex
	mu           sync.RWMutex
	relayMu      sync.Mutex
	relayCloser  interface{ Close() error }
	relayHandler RelayHandler

	session             *store.CloudSession
	vaultKey            []byte
	accessToken         string
	accessExpires       time.Time
	pending             *pendingRegistration
	syncing             bool
	lastError           string
	lastErrorCode       string
	lastErrorStage      string
	lastAttemptAt       time.Time
	nextRetryAt         time.Time
	consecutiveFailures int
	appliedHook         func(context.Context) error
	networkInfo         NetworkInfo
}

func (m *Manager) SetAppliedHook(hook func(context.Context) error) {
	m.mu.Lock()
	m.appliedHook = hook
	m.mu.Unlock()
}

func NewManager(st *store.Store, settings SettingsAccess, baseURL, siteKey string, httpClient *http.Client, logger *slog.Logger) *Manager {
	client, err := newCloudClient(baseURL, httpClient)
	manager := &Manager{store: st, settings: settings, client: client, siteKey: strings.TrimSpace(siteKey), logger: logger, httpOverride: httpClient}
	if manager.logger == nil {
		manager.logger = slog.Default()
	}
	manager.restorePendingRegistration()
	if err != nil {
		manager.lastError = err.Error()
		manager.client, _ = newCloudClient("", httpClient)
		return manager
	}
	if networkErr := manager.restoreNetworkClient(); networkErr != nil {
		manager.lastError = networkErr.Error()
		manager.lastErrorCode = "cloud_proxy_missing"
		manager.lastErrorStage = "local"
	}
	session, err := st.LoadCloudSession()
	if errors.Is(err, store.ErrNotFound) {
		return manager
	}
	if err != nil {
		manager.lastError = "saved cloud session could not be loaded"
		manager.logger.Warn("load cloud session failed", "error_type", fmt.Sprintf("%T", err))
		return manager
	}
	vaultKey, err := decodeBytes(session.VaultKey, keySize)
	if err != nil {
		manager.lastError = "saved cloud vault could not be unlocked"
		manager.logger.Warn("decode saved cloud vault key failed", "error_type", fmt.Sprintf("%T", err))
		return manager
	}
	manager.session = session
	manager.vaultKey = vaultKey
	if manager.pending != nil {
		manager.pending.material.clear()
		manager.pending = nil
		_ = manager.store.DeleteCloudPendingRegistration()
	}
	return manager
}

func (m *Manager) restorePendingRegistration() {
	payload, err := m.store.LoadCloudPendingRegistration()
	if errors.Is(err, store.ErrNotFound) {
		return
	}
	if err != nil {
		m.lastError = "saved cloud registration could not be loaded"
		m.logger.Warn("load pending cloud registration failed", "error_type", fmt.Sprintf("%T", err))
		return
	}
	var saved pendingRegistrationPayload
	if err := json.Unmarshal(payload, &saved); err != nil {
		_ = m.store.DeleteCloudPendingRegistration()
		m.lastError = "saved cloud registration was invalid and has been cleared"
		return
	}
	authHash, authErr := decodeBytes(saved.AuthHash, keySize)
	vaultKey, vaultErr := decodeBytes(saved.VaultKey, keySize)
	email := strings.ToLower(strings.TrimSpace(saved.Email))
	if authErr != nil || vaultErr != nil || email == "" {
		wipe(authHash)
		wipe(vaultKey)
		_ = m.store.DeleteCloudPendingRegistration()
		m.lastError = "saved cloud registration was invalid and has been cleared"
		return
	}
	m.pending = &pendingRegistration{email: email, material: &authMaterial{AuthHash: authHash, VaultKey: vaultKey}}
}

func (m *Manager) persistPendingRegistration(pending *pendingRegistration) error {
	payload, err := json.Marshal(pendingRegistrationPayload{
		Email: pending.email, AuthHash: encodeBytes(pending.material.AuthHash), VaultKey: encodeBytes(pending.material.VaultKey),
	})
	if err != nil {
		return err
	}
	return m.store.SaveCloudPendingRegistration(payload)
}

func (m *Manager) Configured() bool {
	return m.client != nil && m.client.configured()
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	status := Status{
		Configured:          m.Configured(),
		Authenticated:       m.session != nil,
		PendingVerification: m.pending != nil,
		TurnstileSiteKey:    m.siteKey,
		Syncing:             m.syncing,
		LastError:           m.lastError,
		LastErrorCode:       m.lastErrorCode,
		LastErrorStage:      m.lastErrorStage,
		ConsecutiveFailures: m.consecutiveFailures,
	}
	if !m.lastAttemptAt.IsZero() {
		lastAttempt := m.lastAttemptAt
		status.LastAttemptAt = &lastAttempt
	}
	if !m.nextRetryAt.IsZero() {
		nextRetry := m.nextRetryAt
		status.NextRetryAt = &nextRetry
	}
	if m.session != nil {
		status.Email = m.session.Email
		status.Role = m.session.Role
		if !m.session.LastSyncAt.IsZero() {
			lastSync := m.session.LastSyncAt
			status.LastSyncAt = &lastSync
		}
	} else if m.pending != nil {
		status.Email = m.pending.email
	}
	m.mu.RUnlock()
	status.PendingItems, _ = m.store.PendingCloudCount()
	status.Conflicts, _ = m.store.ListCloudConflicts(50)
	return status
}

func (m *Manager) Register(ctx context.Context, input RegisterInput) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if !m.Configured() {
		return errors.New("Amber Cloud is not configured")
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	material, err := newRegistrationMaterial(input.Password)
	if err != nil {
		return err
	}
	request := registerRequest{
		Email:           email,
		TurnstileToken:  input.TurnstileToken,
		AuthHash:        encodeBytes(material.AuthHash),
		SaltKDF:         encodeBytes(material.SaltKDF),
		SaltAuth:        encodeBytes(material.SaltAuth),
		WrappedVaultKey: material.WrappedVaultKey,
	}
	if err := m.client.register(ctx, request); err != nil {
		material.clear()
		m.setError(err)
		return err
	}
	pending := &pendingRegistration{email: email, material: material}
	if err := m.persistPendingRegistration(pending); err != nil {
		material.clear()
		m.setError(err)
		return err
	}
	m.mu.Lock()
	if m.pending != nil {
		m.pending.material.clear()
	}
	m.pending = pending
	m.lastError = ""
	m.mu.Unlock()
	return nil
}

func (m *Manager) VerifyEmail(ctx context.Context, email, code string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	m.mu.RLock()
	pending := m.pending
	m.mu.RUnlock()
	if pending == nil || pending.email != email {
		return errors.New("registration session is no longer available; register again")
	}
	if err := m.client.verifyEmail(ctx, email, strings.TrimSpace(code)); err != nil {
		m.setError(err)
		return err
	}
	login, err := m.client.login(ctx, email, encodeBytes(pending.material.AuthHash))
	if err != nil {
		m.setError(err)
		return err
	}
	vaultKey := append([]byte(nil), pending.material.VaultKey...)
	if err := m.installSession(login, vaultKey); err != nil {
		wipe(vaultKey)
		return err
	}
	cleanupErr := m.store.DeleteCloudPendingRegistration()
	m.mu.Lock()
	pending.material.clear()
	m.pending = nil
	m.lastError = ""
	m.mu.Unlock()
	return cleanupErr
}

func (m *Manager) ResendVerification(ctx context.Context, email string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	m.mu.RLock()
	pending := m.pending
	m.mu.RUnlock()
	if pending == nil || pending.email != email {
		return errors.New("registration session is no longer available; register again")
	}
	if err := m.client.resendVerification(ctx, email); err != nil {
		m.setError(err)
		return err
	}
	m.clearError()
	return nil
}

func (m *Manager) CancelRegistration() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	deleteErr := m.store.DeleteCloudPendingRegistration()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending != nil {
		m.pending.material.clear()
	}
	m.pending = nil
	m.lastError = ""
	return deleteErr
}

func (m *Manager) Login(ctx context.Context, email, password string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if !m.Configured() {
		return errors.New("Amber Cloud is not configured")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	parameters, err := m.client.parameters(ctx, email)
	if err != nil {
		m.setError(err)
		return err
	}
	saltKDF, err := decodeBytes(parameters.SaltKDF, 16)
	if err != nil {
		return errors.New("Amber Cloud returned invalid login parameters")
	}
	saltAuth, err := decodeBytes(parameters.SaltAuth, 16)
	if err != nil {
		return errors.New("Amber Cloud returned invalid login parameters")
	}
	material := deriveAuthMaterial(password, saltKDF, saltAuth)
	defer material.clear()
	login, err := m.client.login(ctx, email, encodeBytes(material.AuthHash))
	if err != nil {
		m.setError(err)
		return err
	}
	if login.SaltKDF != parameters.SaltKDF || login.SaltAuth != parameters.SaltAuth {
		return errors.New("Amber Cloud login parameters changed during authentication")
	}
	vaultKey, err := unwrapVaultKey(material.MasterKey, login.WrappedVaultKey)
	if err != nil {
		return err
	}
	defer wipe(vaultKey)
	if err := m.installSession(login, vaultKey); err != nil {
		return err
	}
	if err := m.store.DeleteCloudPendingRegistration(); err != nil {
		return err
	}
	m.mu.Lock()
	if m.pending != nil {
		m.pending.material.clear()
		m.pending = nil
	}
	m.mu.Unlock()
	m.clearError()
	return nil
}

func (m *Manager) Logout(ctx context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.RLock()
	session := m.session
	m.mu.RUnlock()
	var remoteErr error
	if session != nil && m.Configured() {
		remoteErr = m.client.logout(ctx, session.RefreshToken)
	}
	if remoteErr != nil {
		m.setError(remoteErr)
		return remoteErr
	}
	if err := m.store.DeleteCloudSession(); err != nil {
		return err
	}
	m.mu.Lock()
	wipe(m.vaultKey)
	m.vaultKey = nil
	m.session = nil
	m.accessToken = ""
	m.accessExpires = time.Time{}
	m.lastError = ""
	m.mu.Unlock()
	m.closeRelaySession()
	return nil
}

func (m *Manager) ChangePassword(ctx context.Context, currentPassword, newPassword string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if err := m.ensureAccess(ctx); err != nil {
		return err
	}
	m.mu.RLock()
	if m.session == nil {
		m.mu.RUnlock()
		return errors.New("cloud login is required")
	}
	session := *m.session
	vaultKey := append([]byte(nil), m.vaultKey...)
	accessToken := m.accessToken
	m.mu.RUnlock()
	defer wipe(vaultKey)
	saltKDF, err := decodeBytes(session.SaltKDF, 16)
	if err != nil {
		return errors.New("saved cloud login parameters are invalid")
	}
	saltAuth, err := decodeBytes(session.SaltAuth, 16)
	if err != nil {
		return errors.New("saved cloud login parameters are invalid")
	}
	current := deriveAuthMaterial(currentPassword, saltKDF, saltAuth)
	defer current.clear()
	unwrapped, err := unwrapVaultKey(current.MasterKey, session.WrappedVaultKey)
	if err != nil {
		return errors.New("current master password is incorrect")
	}
	matches := subtle.ConstantTimeCompare(unwrapped, vaultKey) == 1
	wipe(unwrapped)
	if !matches {
		return errors.New("current master password is incorrect")
	}
	next, err := newRewrapMaterial(newPassword, vaultKey)
	if err != nil {
		return err
	}
	defer next.clear()
	request := registerRequest{
		AuthHash: encodeBytes(next.AuthHash), SaltKDF: encodeBytes(next.SaltKDF), SaltAuth: encodeBytes(next.SaltAuth),
		WrappedVaultKey: next.WrappedVaultKey,
	}
	if err := m.client.changePassword(ctx, accessToken, encodeBytes(current.AuthHash), request); err != nil {
		m.setError(err)
		return err
	}
	login, err := m.client.login(ctx, session.Email, encodeBytes(next.AuthHash))
	if err != nil {
		m.setError(err)
		return err
	}
	if err := m.installSession(login, vaultKey); err != nil {
		return err
	}
	m.clearError()
	return nil
}

func (m *Manager) ListShares(ctx context.Context) ([]Share, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if err := m.ensureAccess(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	accessToken := m.accessToken
	m.mu.RUnlock()
	shares, err := m.client.listShares(ctx, accessToken)
	if err != nil {
		m.setError(err)
		return nil, err
	}
	m.clearError()
	return shares, nil
}

func (m *Manager) CreateShare(ctx context.Context, input CreateShareInput) (CreatedShare, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if !input.Consent {
		return CreatedShare{}, errors.New("cloud custody consent is required")
	}
	if input.QuotaRequests < 0 || input.QuotaRequests > 1_000_000 {
		return CreatedShare{}, errors.New("share request quota is invalid")
	}
	if err := m.ensureAccess(ctx); err != nil {
		return CreatedShare{}, err
	}
	account, err := m.store.GetAccount(input.AccountID)
	if err != nil {
		return CreatedShare{}, err
	}
	credential := shareCredential{AccountType: string(account.AccountType), ChatGPTAccountID: account.ChatGPTAccountID}
	switch account.AccountType {
	case store.AccountTypeOAuth:
		return CreatedShare{}, errors.New("OAuth sharing requires the owner-device relay planned for v0.4.0")
	case store.AccountTypeAPIKey:
		if strings.TrimSpace(account.APIKey) == "" {
			return CreatedShare{}, errors.New("the API-key account has no credential")
		}
		credential.Token = account.APIKey
		credential.UpstreamURL = account.BaseURL
	default:
		return CreatedShare{}, errors.New("the account type cannot be shared")
	}
	if strings.TrimSpace(account.ClientUID) == "" {
		return CreatedShare{}, errors.New("sync this account before sharing it")
	}
	m.mu.RLock()
	accessToken := m.accessToken
	m.mu.RUnlock()
	response, err := m.client.createShare(ctx, accessToken, createShareRequest{
		AccountUID: account.ClientUID, Credential: credential, QuotaRequests: input.QuotaRequests,
		ExpiresAt: strings.TrimSpace(input.ExpiresAt), Consent: true,
	})
	if err != nil {
		m.setError(err)
		return CreatedShare{}, err
	}
	m.clearError()
	return CreatedShare{Share: response.Share, GuestKey: response.GuestKey}, nil
}

func (m *Manager) UpdateShare(ctx context.Context, shareID int64, updates map[string]any) (Share, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if shareID <= 0 {
		return Share{}, errors.New("share ID is invalid")
	}
	if err := m.ensureAccess(ctx); err != nil {
		return Share{}, err
	}
	m.mu.RLock()
	accessToken := m.accessToken
	m.mu.RUnlock()
	share, err := m.client.updateShare(ctx, accessToken, shareID, updates)
	if err != nil {
		m.setError(err)
		return Share{}, err
	}
	m.clearError()
	return share, nil
}

func (m *Manager) ShareUsage(ctx context.Context, shareID int64) ([]ShareUsage, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if shareID <= 0 {
		return nil, errors.New("share ID is invalid")
	}
	if err := m.ensureAccess(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	accessToken := m.accessToken
	m.mu.RUnlock()
	usage, err := m.client.shareUsage(ctx, accessToken, shareID)
	if err != nil {
		m.setError(err)
		return nil, err
	}
	m.clearError()
	return usage, nil
}

func (m *Manager) AdminOverview(ctx context.Context, adminKey string) (AdminOverview, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, err := m.adminAccess(ctx, adminKey)
	if err != nil {
		return AdminOverview{}, err
	}
	overview, err := m.client.adminOverview(ctx, accessToken, strings.TrimSpace(adminKey))
	if err != nil {
		m.setError(err)
		return AdminOverview{}, err
	}
	m.clearError()
	return overview, nil
}

func (m *Manager) AdminSetUserBanned(ctx context.Context, adminKey string, userID int64, banned bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, err := m.adminAccess(ctx, adminKey)
	if err != nil {
		return err
	}
	if err := m.client.adminSetUserBanned(ctx, accessToken, strings.TrimSpace(adminKey), userID, banned); err != nil {
		m.setError(err)
		return err
	}
	m.clearError()
	return nil
}

func (m *Manager) AdminLogoutUser(ctx context.Context, adminKey string, userID int64) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, err := m.adminAccess(ctx, adminKey)
	if err != nil {
		return err
	}
	if err := m.client.adminLogoutUser(ctx, accessToken, strings.TrimSpace(adminKey), userID); err != nil {
		m.setError(err)
		return err
	}
	m.clearError()
	return nil
}

func (m *Manager) AdminDeleteUser(ctx context.Context, adminKey string, userID int64) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, err := m.adminAccess(ctx, adminKey)
	if err != nil {
		return err
	}
	if err := m.client.adminDeleteUser(ctx, accessToken, strings.TrimSpace(adminKey), userID); err != nil {
		m.setError(err)
		return err
	}
	m.clearError()
	return nil
}

func (m *Manager) AdminUpdateSettings(ctx context.Context, adminKey string, settings map[string]bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, err := m.adminAccess(ctx, adminKey)
	if err != nil {
		return err
	}
	if err := m.client.adminUpdateSettings(ctx, accessToken, strings.TrimSpace(adminKey), settings); err != nil {
		m.setError(err)
		return err
	}
	m.clearError()
	return nil
}

func (m *Manager) AdminSetShareRevoked(ctx context.Context, adminKey string, shareID int64, revoked bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	accessToken, err := m.adminAccess(ctx, adminKey)
	if err != nil {
		return err
	}
	if err := m.client.adminSetShareRevoked(ctx, accessToken, strings.TrimSpace(adminKey), shareID, revoked); err != nil {
		m.setError(err)
		return err
	}
	m.clearError()
	return nil
}

func (m *Manager) adminAccess(ctx context.Context, adminKey string) (string, error) {
	if strings.TrimSpace(adminKey) == "" {
		return "", errors.New("administrator second factor is required")
	}
	if err := m.ensureAccess(ctx); err != nil {
		return "", err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return "", errors.New("cloud login is required")
	}
	if m.session.Role != "admin" {
		return "", errors.New("administrator role is required")
	}
	return m.accessToken, nil
}

func (m *Manager) Run(ctx context.Context) {
	go m.runRelay(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	nextPeriodic := time.Now().Add(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.mu.RLock()
			authenticated := m.session != nil
			syncing := m.syncing
			nextRetryAt := m.nextRetryAt
			lastAttemptAt := m.lastAttemptAt
			failures := m.consecutiveFailures
			m.mu.RUnlock()
			if !authenticated || syncing {
				continue
			}
			pendingItems, _ := m.store.PendingCloudCount()
			due := now.After(nextPeriodic) || now.Equal(nextPeriodic)
			if !nextRetryAt.IsZero() {
				due = !now.Before(nextRetryAt)
			} else if pendingItems > 0 && failures == 0 && !lastAttemptAt.IsZero() {
				due = true
			}
			if !due {
				continue
			}
			if err := m.Sync(ctx); err != nil {
				if ctx.Err() == nil {
					m.logger.Warn("cloud sync failed", "error_type", fmt.Sprintf("%T", err))
				}
				m.mu.RLock()
				retryScheduled := !m.nextRetryAt.IsZero()
				m.mu.RUnlock()
				if !retryScheduled {
					nextPeriodic = time.Now().Add(5 * time.Minute)
				}
			} else {
				nextPeriodic = time.Now().Add(5 * time.Minute)
			}
		}
	}
}

func (m *Manager) Sync(ctx context.Context) (syncErr error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.beginSync()
	defer func() { m.finishSync(syncErr) }()
	if err := m.ensureAccess(ctx); err != nil {
		return err
	}
	m.mu.RLock()
	if m.session == nil {
		m.mu.RUnlock()
		return errors.New("cloud login is required")
	}
	accessToken := m.accessToken
	cursor := m.session.SyncCursor
	vaultKey := append([]byte(nil), m.vaultKey...)
	m.mu.RUnlock()
	defer wipe(vaultKey)

	pulledItems := false
	for page := 0; page < cloudPullMaxPages; page++ {
		pull, err := m.client.pull(ctx, accessToken, cursor)
		if err != nil {
			return err
		}
		if err := m.applyRemoteItems(pull.Items, vaultKey); err != nil {
			return err
		}
		pulledItems = pulledItems || len(pull.Items) > 0
		previousCursor := cursor
		if pull.Cursor != "" {
			cursor = pull.Cursor
		}
		if cursor != previousCursor {
			if err := m.checkpointCursor(cursor); err != nil {
				return err
			}
		}
		if len(pull.Items) < cloudPullPageSize || cursor == previousCursor {
			break
		}
	}
	if pulledItems {
		m.mu.RLock()
		hook := m.appliedHook
		m.mu.RUnlock()
		if hook != nil {
			if err := hook(ctx); err != nil {
				return err
			}
		}
	}
	conflicted := false
	for attempt := 0; attempt < 2; attempt++ {
		conflicted = false
		pending, err := m.store.ListCloudOutboxBatches()
		if err != nil {
			return err
		}
		for _, batch := range pending {
			batchConflict, nextCursor, pushErr := m.pushOutboxBatch(ctx, accessToken, vaultKey, batch)
			if pushErr != nil {
				return pushErr
			}
			if nextCursor != "" {
				cursor = nextCursor
				if err := m.checkpointCursor(cursor); err != nil {
					return err
				}
			}
			if batchConflict {
				conflicted = true
				break
			}
		}
		if conflicted {
			continue
		}
		items, err := m.collectDirty(vaultKey)
		if err != nil {
			return err
		}
		itemBatches, err := splitSyncItems(items, cloudPushMaxItems, cloudPushMaxBytes)
		if err != nil {
			return err
		}
		for _, itemBatch := range itemBatches {
			key, err := newSyncIdempotencyKey()
			if err != nil {
				return err
			}
			payload, err := json.Marshal(syncOutboxPayload{Items: itemBatch})
			if err != nil {
				return err
			}
			if err := m.store.SaveCloudOutboxBatch(key, payload); err != nil {
				return err
			}
			batch := store.CloudOutboxBatch{IdempotencyKey: key, PayloadJSON: payload, CreatedAt: time.Now()}
			batchConflict, nextCursor, pushErr := m.pushOutboxBatch(ctx, accessToken, vaultKey, batch)
			if pushErr != nil {
				return pushErr
			}
			if nextCursor != "" {
				cursor = nextCursor
				if err := m.checkpointCursor(cursor); err != nil {
					return err
				}
			}
			if batchConflict {
				conflicted = true
				break
			}
		}
		if !conflicted {
			break
		}
	}
	if conflicted {
		return errors.New("cloud sync conflict could not be resolved automatically")
	}
	syncedAt := time.Now()
	m.mu.RLock()
	refreshToken := ""
	if m.session != nil {
		refreshToken = m.session.RefreshToken
	}
	m.mu.RUnlock()
	if err := m.store.UpdateCloudSessionProgress(refreshToken, cursor, syncedAt); err != nil {
		return err
	}
	m.mu.Lock()
	if m.session != nil {
		m.session.SyncCursor = cursor
		m.session.LastSyncAt = syncedAt
	}
	m.mu.Unlock()
	return nil
}

func newSyncIdempotencyKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate cloud sync idempotency key: %w", err)
	}
	return "amber-sync-" + hex.EncodeToString(value[:]), nil
}

func splitSyncItems(items []remoteVaultItem, maxItems, maxBytes int) ([][]remoteVaultItem, error) {
	if maxItems <= 0 || maxBytes <= 0 {
		return nil, errors.New("invalid cloud sync batch limits")
	}
	var batches [][]remoteVaultItem
	current := make([]remoteVaultItem, 0, min(maxItems, len(items)))
	for _, item := range items {
		candidate := append(append([]remoteVaultItem(nil), current...), item)
		encoded, err := json.Marshal(syncOutboxPayload{Items: candidate})
		if err != nil {
			return nil, err
		}
		if len(candidate) <= maxItems && len(encoded) <= maxBytes {
			current = candidate
			continue
		}
		if len(current) == 0 {
			return nil, fmt.Errorf("cloud sync item %s:%s exceeds the upload limit", item.Kind, item.ClientUID)
		}
		batches = append(batches, current)
		current = []remoteVaultItem{item}
		encoded, err = json.Marshal(syncOutboxPayload{Items: current})
		if err != nil {
			return nil, err
		}
		if len(encoded) > maxBytes {
			return nil, fmt.Errorf("cloud sync item %s:%s exceeds the upload limit", item.Kind, item.ClientUID)
		}
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches, nil
}

func splitSyncIdempotencyKey(base string, index, total int) string {
	if total <= 1 {
		return base
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", base, index, total)))
	return "amber-sync-split-" + hex.EncodeToString(digest[:16])
}

func (m *Manager) checkpointCursor(cursor string) error {
	if cursor == "" {
		return nil
	}
	if err := m.store.UpdateCloudSessionCursor(cursor); err != nil {
		return err
	}
	m.mu.Lock()
	if m.session != nil {
		m.session.SyncCursor = cursor
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) pushOutboxBatch(ctx context.Context, accessToken string, vaultKey []byte, batch store.CloudOutboxBatch) (bool, string, error) {
	var payload syncOutboxPayload
	if err := json.Unmarshal(batch.PayloadJSON, &payload); err != nil || len(payload.Items) == 0 || len(payload.Items) > cloudPushMaxItems {
		return false, "", errors.New("saved cloud sync batch is invalid")
	}
	itemBatches, err := splitSyncItems(payload.Items, cloudPushMaxItems, cloudPushMaxBytes)
	if err != nil {
		return false, "", err
	}
	lastCursor := ""
	for index, itemBatch := range itemBatches {
		key := splitSyncIdempotencyKey(batch.IdempotencyKey, index, len(itemBatches))
		response, pushErr := m.client.push(ctx, accessToken, key, itemBatch)
		if pushErr != nil {
			var cloudErr *CloudError
			if errors.As(pushErr, &cloudErr) && cloudErr.Status == http.StatusConflict && cloudErr.Code == "vault_conflict" && len(response.Conflicts) > 0 {
				if err := m.applyRemoteItems(response.Conflicts, vaultKey); err != nil {
					return false, "", err
				}
				if err := m.store.DeleteCloudOutboxBatch(batch.IdempotencyKey); err != nil {
					return false, "", err
				}
				return true, response.Cursor, nil
			}
			return false, "", pushErr
		}
		if len(response.Items) != len(itemBatch) {
			return false, "", errors.New("Amber Cloud returned an incomplete batch acknowledgement")
		}
		if err := m.acknowledge(response.Items, vaultKey); err != nil {
			return false, "", err
		}
		if response.Cursor != "" {
			lastCursor = response.Cursor
		}
	}
	if err := m.store.DeleteCloudOutboxBatch(batch.IdempotencyKey); err != nil {
		return false, "", err
	}
	return false, lastCursor, nil
}

func (m *Manager) ensureAccess(ctx context.Context) error {
	m.mu.RLock()
	if m.session == nil {
		m.mu.RUnlock()
		return errors.New("cloud login is required")
	}
	if m.accessToken != "" && time.Until(m.accessExpires) > time.Minute {
		m.mu.RUnlock()
		return nil
	}
	refreshToken := m.session.RefreshToken
	m.mu.RUnlock()
	response, err := m.client.refresh(ctx, refreshToken)
	if err != nil {
		if isTerminalCloudSessionError(err) {
			m.invalidateCloudSession(err)
		}
		return err
	}
	if response.AccessToken == "" || response.RefreshToken == "" {
		return errors.New("Amber Cloud returned an invalid session")
	}
	m.mu.Lock()
	m.accessToken = response.AccessToken
	m.accessExpires = time.Now().Add(time.Duration(response.AccessExpiresIn) * time.Second)
	m.session.RefreshToken = response.RefreshToken
	session := *m.session
	m.mu.Unlock()
	return m.store.UpdateCloudSessionProgress(session.RefreshToken, session.SyncCursor, session.LastSyncAt)
}

func isTerminalCloudSessionError(err error) bool {
	var cloudErr *CloudError
	if !errors.As(err, &cloudErr) || cloudErr.Status != http.StatusUnauthorized {
		return false
	}
	switch cloudErr.Code {
	case "invalid_refresh_token", "session_expired", "account_disabled":
		return true
	default:
		// ensureAccess only sends a refresh request, so any other 401 also means
		// the persisted refresh credential cannot establish a new session.
		return true
	}
}

func (m *Manager) invalidateCloudSession(reason error) {
	if err := m.store.DeleteCloudSession(); err != nil {
		m.logger.Warn("delete expired cloud session failed", "error_type", fmt.Sprintf("%T", err))
	}
	code, stage := cloudErrorMetadata(reason)
	m.mu.Lock()
	wipe(m.vaultKey)
	m.vaultKey = nil
	m.session = nil
	m.accessToken = ""
	m.accessExpires = time.Time{}
	m.lastError = reason.Error()
	m.lastErrorCode = code
	m.lastErrorStage = stage
	m.nextRetryAt = time.Time{}
	m.mu.Unlock()
}

func (m *Manager) installSession(login loginResponse, vaultKey []byte) error {
	if login.AccessToken == "" || login.RefreshToken == "" || login.User.ID <= 0 || login.User.Email == "" || len(vaultKey) != keySize {
		return errors.New("Amber Cloud returned an invalid login session")
	}
	session := &store.CloudSession{
		UserID: login.User.ID, Email: login.User.Email, Role: login.User.Role,
		SaltKDF: login.SaltKDF, SaltAuth: login.SaltAuth, WrappedVaultKey: login.WrappedVaultKey,
		VaultKey: encodeBytes(vaultKey), RefreshToken: login.RefreshToken,
	}
	if err := m.store.SaveCloudSession(*session); err != nil {
		return err
	}
	m.mu.Lock()
	wipe(m.vaultKey)
	m.vaultKey = append([]byte(nil), vaultKey...)
	m.session = session
	m.accessToken = login.AccessToken
	m.accessExpires = time.Now().Add(time.Duration(login.AccessExpiresIn) * time.Second)
	m.lastError = ""
	m.mu.Unlock()
	return nil
}

func (m *Manager) collectDirty(vaultKey []byte) ([]remoteVaultItem, error) {
	var result []remoteVaultItem
	proxies, err := m.store.ListProxies()
	if err != nil {
		return nil, err
	}
	for _, proxy := range proxies {
		if !proxy.SyncDirty {
			continue
		}
		ciphertext, err := encryptEnvelope(vaultKey, proxy.UpdatedAt, proxyPayload{
			Name: proxy.Name, Type: proxy.Type, Host: proxy.Host, Port: proxy.Port,
			Username: proxy.Username, Password: proxy.Password, CreatedAt: proxy.CreatedAt,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, remoteVaultItem{Kind: store.CloudKindProxy, ClientUID: proxy.ClientUID, Ciphertext: ciphertext, Version: proxy.SyncVersion})
	}
	accounts, err := m.store.ListAccounts()
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		if !account.SyncDirty {
			continue
		}
		proxyUID := ""
		if account.ProxyID != nil {
			if proxy, proxyErr := m.store.GetProxy(*account.ProxyID); proxyErr == nil {
				proxyUID = proxy.ClientUID
			}
		}
		ciphertext, err := encryptEnvelope(vaultKey, account.UpdatedAt, accountPayload{
			AccountType: account.AccountType, BaseURL: account.BaseURL, APIKey: account.APIKey, Email: account.Email,
			ChatGPTAccountID: account.ChatGPTAccountID, PlanType: account.PlanType, AccessToken: account.AccessToken,
			RefreshToken: account.RefreshToken, IDToken: account.IDToken, ExpiresAt: account.ExpiresAt,
			Status: account.Status, StatusReason: account.StatusReason, MaxConcurrency: account.MaxConcurrency,
			QueueCapacity: &account.QueueCapacity, ProxyUID: proxyUID, CreatedAt: account.CreatedAt,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, remoteVaultItem{Kind: store.CloudKindAccount, ClientUID: account.ClientUID, Ciphertext: ciphertext, Version: account.SyncVersion})
	}
	targets, err := m.store.ListCodexRemoteTargets()
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		if !target.SyncDirty {
			continue
		}
		ciphertext, err := encryptEnvelope(vaultKey, target.UpdatedAt, codexRemotePayload{
			Name: target.Name, Host: target.Host, Port: target.Port, User: target.User, Password: target.Password,
			RemotePort: target.RemotePort, Model: target.Model, Mode: target.Mode, BaseURL: target.BaseURL,
			APIKey: target.APIKey, TunnelEnabled: target.TunnelEnabled, Injected: target.Injected, CreatedAt: target.CreatedAt,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, remoteVaultItem{Kind: store.CloudKindCodexRemote, ClientUID: target.ClientUID, Ciphertext: ciphertext, Version: target.SyncVersion})
	}
	settingsState, err := m.store.CloudSettingsState()
	if err != nil {
		return nil, err
	}
	if settingsState.SyncDirty {
		settings := m.settings.Get()
		ciphertext, err := encryptEnvelope(vaultKey, settingsState.UpdatedAt, settingsPayload{
			InjectInstr: settings.InjectInstr, DefaultModel: settings.DefaultModel, UserAgent: settings.UserAgent,
			Originator: settings.Originator, Language: settings.Language, AutoStartServer: settings.AutoStartServer,
			TLSFingerprint: settings.TLSFingerprint, CodexModel: settings.CodexModel, AccountStrategy: settings.AccountStrategy,
			LogRetentionDays: settings.LogRetentionDays, MaxLogRows: settings.MaxLogRows, AutoRecovery: settings.AutoRecovery,
			CompatProfile: settings.CompatProfile,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, remoteVaultItem{Kind: store.CloudKindSettings, ClientUID: settingsState.ClientUID, Ciphertext: ciphertext, Version: settingsState.SyncVersion})
	}
	tombstones, err := m.store.CloudTombstones()
	if err != nil {
		return nil, err
	}
	for _, tombstone := range tombstones {
		ciphertext, err := encryptEnvelope(vaultKey, tombstone.UpdatedAt, map[string]bool{"deleted": true})
		if err != nil {
			return nil, err
		}
		result = append(result, remoteVaultItem{
			Kind: tombstone.Kind, ClientUID: tombstone.ClientUID, Ciphertext: ciphertext,
			Version: tombstone.SyncVersion, Deleted: true,
		})
	}
	return result, nil
}

func (m *Manager) applyRemoteItems(items []remoteVaultItem, vaultKey []byte) (resultErr error) {
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(left, right int) bool {
		return remoteRank(items[left]) < remoteRank(items[right])
	})
	if err := m.store.SetCloudApplying(true); err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, m.store.SetCloudApplying(false))
	}()
	for _, item := range items {
		if item.Version <= 0 || item.ClientUID == "" {
			return errors.New("Amber Cloud returned an invalid vault item")
		}
		envelope, err := decryptEnvelope(vaultKey, item.Ciphertext)
		if err != nil {
			return err
		}
		meta, metaErr := m.store.CloudItemMeta(item.Kind, item.ClientUID)
		localExists := metaErr == nil
		if metaErr != nil && !errors.Is(metaErr, store.ErrNotFound) {
			return metaErr
		}
		if localExists && item.Version <= meta.SyncVersion {
			continue
		}
		remoteWon := false
		if localExists && meta.SyncDirty {
			if meta.UpdatedAt.After(envelope.UpdatedAt) {
				if err := m.store.RebaseCloudItem(item.Kind, item.ClientUID, item.Version); err != nil {
					return err
				}
				if err := m.store.AddCloudConflict(item.Kind, item.ClientUID, "local_won", "Local update was newer than the remote update."); err != nil {
					return err
				}
				continue
			}
			remoteWon = true
		}
		if item.Deleted {
			if item.Kind == store.CloudKindSettings {
				return errors.New("cloud settings tombstones are not supported")
			}
			if localExists {
				if err := m.store.DeleteCloudItem(item.Kind, item.ClientUID); err != nil {
					return err
				}
			}
		} else if err := m.applyRemotePayload(item, envelope); err != nil {
			return err
		}
		if remoteWon {
			if err := m.store.AddCloudConflict(item.Kind, item.ClientUID, "remote_won", "Remote update was newer than the local update."); err != nil {
				return err
			}
		}
	}
	return nil
}

func remoteRank(item remoteVaultItem) int {
	if item.Deleted {
		switch item.Kind {
		case store.CloudKindAccount:
			return 10
		case store.CloudKindCodexRemote:
			return 11
		case store.CloudKindProxy:
			return 12
		default:
			return 13
		}
	}
	switch item.Kind {
	case store.CloudKindProxy:
		return 0
	case store.CloudKindCodexRemote:
		return 1
	case store.CloudKindAccount:
		return 2
	case store.CloudKindSettings:
		return 3
	default:
		return 9
	}
}

func (m *Manager) applyRemotePayload(item remoteVaultItem, envelope vaultEnvelope) error {
	switch item.Kind {
	case store.CloudKindProxy:
		payload, err := decodePayload[proxyPayload](envelope)
		if err != nil || validateProxyPayload(payload) != nil {
			return errors.New("invalid encrypted proxy payload")
		}
		return m.store.ApplyCloudProxy(&store.Proxy{
			Name: payload.Name, Type: payload.Type, Host: payload.Host, Port: payload.Port,
			Username: payload.Username, Password: payload.Password, CreatedAt: payload.CreatedAt, ClientUID: item.ClientUID,
		}, item.Version, envelope.UpdatedAt)
	case store.CloudKindAccount:
		payload, err := decodePayload[accountPayload](envelope)
		if err != nil || validateAccountPayload(payload) != nil {
			return errors.New("invalid encrypted account payload")
		}
		maxConcurrency, queueCapacity := accountPayloadLimits(payload)
		return m.store.ApplyCloudAccount(&store.Account{
			AccountType: payload.AccountType, BaseURL: payload.BaseURL, APIKey: payload.APIKey, Email: payload.Email,
			ChatGPTAccountID: payload.ChatGPTAccountID, PlanType: payload.PlanType, AccessToken: payload.AccessToken,
			RefreshToken: payload.RefreshToken, IDToken: payload.IDToken, ExpiresAt: payload.ExpiresAt,
			Status: payload.Status, StatusReason: payload.StatusReason, MaxConcurrency: maxConcurrency,
			QueueCapacity: queueCapacity, CreatedAt: payload.CreatedAt, ClientUID: item.ClientUID,
		}, payload.ProxyUID, item.Version, envelope.UpdatedAt)
	case store.CloudKindCodexRemote:
		payload, err := decodePayload[codexRemotePayload](envelope)
		if err != nil || validateCodexRemotePayload(payload) != nil {
			return errors.New("invalid encrypted Codex remote payload")
		}
		return m.store.ApplyCloudCodexRemote(&store.CodexRemoteTarget{
			Name: payload.Name, Host: payload.Host, Port: payload.Port, User: payload.User, Password: payload.Password,
			RemotePort: payload.RemotePort, Model: payload.Model, Mode: payload.Mode, BaseURL: payload.BaseURL,
			APIKey: payload.APIKey, TunnelEnabled: payload.TunnelEnabled, Injected: payload.Injected,
			CreatedAt: payload.CreatedAt, ClientUID: item.ClientUID,
		}, item.Version, envelope.UpdatedAt)
	case store.CloudKindSettings:
		payload, err := decodePayload[settingsPayload](envelope)
		if err != nil {
			return errors.New("invalid encrypted settings payload")
		}
		current := m.settings.Get()
		current.InjectInstr = payload.InjectInstr
		current.DefaultModel = payload.DefaultModel
		current.UserAgent = payload.UserAgent
		current.Originator = payload.Originator
		current.Language = payload.Language
		current.AutoStartServer = payload.AutoStartServer
		current.TLSFingerprint = payload.TLSFingerprint
		current.CodexModel = payload.CodexModel
		current.AccountStrategy = payload.AccountStrategy
		current.LogRetentionDays = payload.LogRetentionDays
		current.MaxLogRows = payload.MaxLogRows
		current.AutoRecovery = payload.AutoRecovery
		current.CompatProfile = payload.CompatProfile
		if err := m.settings.Save(current); err != nil {
			return err
		}
		return m.store.ApplyCloudSettingsState(item.ClientUID, item.Version, envelope.UpdatedAt)
	default:
		return errors.New("unsupported cloud vault item kind")
	}
}

func (m *Manager) acknowledge(items []remoteVaultItem, vaultKey []byte) error {
	for _, item := range items {
		if item.Version <= 0 {
			return errors.New("Amber Cloud returned an invalid batch acknowledgement")
		}
		envelope, err := decryptEnvelope(vaultKey, item.Ciphertext)
		if err != nil {
			return err
		}
		acknowledged, err := m.store.AcknowledgeCloudItem(
			item.Kind, item.ClientUID, item.Version-1, item.Version, envelope.UpdatedAt, item.Deleted,
		)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) && !item.Deleted {
				if rebaseErr := m.store.RebaseCloudTombstone(item.Kind, item.ClientUID, item.Version); rebaseErr != nil {
					return rebaseErr
				}
				continue
			}
			return err
		}
		if acknowledged {
			continue
		}
		if item.Deleted {
			if err := m.store.RebaseCloudTombstone(item.Kind, item.ClientUID, item.Version); err != nil {
				return err
			}
		} else if err := m.store.RebaseCloudItem(item.Kind, item.ClientUID, item.Version); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) beginSync() {
	m.mu.Lock()
	m.syncing = true
	m.lastAttemptAt = time.Now()
	m.mu.Unlock()
}

func (m *Manager) finishSync(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncing = false
	if err == nil {
		m.lastError = ""
		m.lastErrorCode = ""
		m.lastErrorStage = ""
		m.consecutiveFailures = 0
		m.nextRetryAt = time.Time{}
		return
	}
	m.lastError = err.Error()
	m.lastErrorCode, m.lastErrorStage = cloudErrorMetadata(err)
	m.consecutiveFailures++
	var cloudErr *CloudError
	if errors.As(err, &cloudErr) && cloudErr.Retryable {
		m.nextRetryAt = time.Now().Add(syncRetryDelay(m.consecutiveFailures))
	} else {
		m.nextRetryAt = time.Time{}
	}
}

func syncRetryDelay(failures int) time.Duration {
	switch failures {
	case 1:
		return 5 * time.Second
	case 2:
		return 15 * time.Second
	case 3:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}

func cloudErrorMetadata(err error) (string, string) {
	var cloudErr *CloudError
	if errors.As(err, &cloudErr) {
		return cloudErr.Code, cloudErr.Stage
	}
	return "cloud_local_failed", "local"
}

func (m *Manager) setError(err error) {
	m.mu.Lock()
	m.lastError = err.Error()
	m.lastErrorCode, m.lastErrorStage = cloudErrorMetadata(err)
	m.mu.Unlock()
}

func (m *Manager) clearError() {
	m.mu.Lock()
	m.lastError = ""
	m.lastErrorCode = ""
	m.lastErrorStage = ""
	m.consecutiveFailures = 0
	m.nextRetryAt = time.Time{}
	m.mu.Unlock()
}

func (m *Manager) Close() {
	m.closeRelaySession()
	m.mu.Lock()
	wipe(m.vaultKey)
	if m.pending != nil {
		m.pending.material.clear()
	}
	m.vaultKey = nil
	m.pending = nil
	m.mu.Unlock()
}
