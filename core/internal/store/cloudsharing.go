package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CloudIdentity stores per-cloud-user envelope and device keys. Private key
// fields are encrypted by the installation cipher before they reach SQLite.
type CloudIdentity struct {
	UserID           int64
	X25519PublicKey  string
	X25519PrivateKey string
	DevicePublicKey  string
	DevicePrivateKey string
	DevicePublicID   string
	DeviceName       string
	RelayEnabled     bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CloudReceivedKey struct {
	UserID        int64
	GrantPublicID string
	KeyVersion    int
	KeyPrefix     string
	BaseURL       string
	GuestKey      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CloudConnectHostState struct {
	UserID          int64
	ConnectionCode  string
	PasswordVersion int
	Password        string
	ExpiresAt       time.Time
	UpdatedAt       time.Time
}

type CloudReceivedAccountLink struct {
	ID               int64     `json:"id"`
	UserID           int64     `json:"-"`
	GrantPublicID    string    `json:"grant_public_id"`
	OwnerName        string    `json:"owner_name"`
	GroupName        string    `json:"group_name"`
	RemoteStatus     string    `json:"remote_status"`
	Enabled          bool      `json:"enabled"`
	RPMLimit         int       `json:"rpm_limit"`
	ConcurrencyLimit int       `json:"concurrency_limit"`
	QuotaRequests    int       `json:"quota_requests"`
	UsedRequests     int       `json:"used_requests"`
	ProxyID          *int64    `json:"proxy_id,omitempty"`
	HealthStatus     string    `json:"health_status"`
	HealthMessage    string    `json:"health_message,omitempty"`
	LastCheckedAt    time.Time `json:"last_checked_at,omitempty"`
	BaseURL          string    `json:"base_url"`
	GuestKey         string    `json:"-"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CloudConnectClaimAttempt struct {
	UserID          int64
	IdempotencyKey  string
	ConnectionCode  string
	Password        string
	KeyMaterialJSON string
	GuestKey        string
	CreatedAt       time.Time
}

func (s *Store) SaveCloudIdentity(identity CloudIdentity) error {
	privateCipher, err := s.cipher.Encrypt(identity.X25519PrivateKey)
	if err != nil {
		return err
	}
	deviceCipher, err := s.cipher.Encrypt(identity.DevicePrivateKey)
	if err != nil {
		return err
	}
	now := time.Now()
	created := identity.CreatedAt
	if created.IsZero() {
		created = now
	}
	_, err = s.db.Exec(`INSERT INTO cloud_identities
		(user_id,x25519_public_key,x25519_private_cipher,device_public_key,device_private_cipher,device_public_id,
		 device_name,relay_enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET x25519_public_key=excluded.x25519_public_key,
		x25519_private_cipher=excluded.x25519_private_cipher,device_public_key=excluded.device_public_key,
		device_private_cipher=excluded.device_private_cipher,device_public_id=excluded.device_public_id,
		device_name=excluded.device_name,relay_enabled=excluded.relay_enabled,updated_at=excluded.updated_at`,
		identity.UserID, identity.X25519PublicKey, privateCipher, identity.DevicePublicKey, deviceCipher,
		identity.DevicePublicID, identity.DeviceName, boolInt(identity.RelayEnabled), created.Unix(), now.Unix())
	return err
}

func (s *Store) LoadCloudIdentity(userID int64) (*CloudIdentity, error) {
	var identity CloudIdentity
	var privateCipher, deviceCipher string
	var relayEnabled int
	var createdAt, updatedAt int64
	err := s.db.QueryRow(`SELECT user_id,x25519_public_key,x25519_private_cipher,device_public_key,device_private_cipher,
		device_public_id,device_name,relay_enabled,created_at,updated_at FROM cloud_identities WHERE user_id=?`, userID).Scan(
		&identity.UserID, &identity.X25519PublicKey, &privateCipher, &identity.DevicePublicKey, &deviceCipher,
		&identity.DevicePublicID, &identity.DeviceName, &relayEnabled, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if identity.X25519PrivateKey, err = s.cipher.Decrypt(privateCipher); err != nil {
		return nil, err
	}
	if identity.DevicePrivateKey, err = s.cipher.Decrypt(deviceCipher); err != nil {
		return nil, err
	}
	identity.RelayEnabled = relayEnabled != 0
	identity.CreatedAt = unixToTime(createdAt)
	identity.UpdatedAt = unixToTime(updatedAt)
	return &identity, nil
}

func (s *Store) UpdateCloudDevice(userID int64, publicID string, relayEnabled bool) error {
	result, err := s.db.Exec(`UPDATE cloud_identities SET device_public_id=?,relay_enabled=?,updated_at=? WHERE user_id=?`,
		publicID, boolInt(relayEnabled), time.Now().Unix(), userID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SaveCloudReceivedKey(key CloudReceivedKey) error {
	ciphertext, err := s.cipher.Encrypt(key.GuestKey)
	if err != nil {
		return err
	}
	now := time.Now()
	created := key.CreatedAt
	if created.IsZero() {
		created = now
	}
	_, err = s.db.Exec(`INSERT INTO cloud_received_keys
		(user_id,grant_public_id,key_version,key_prefix,base_url,guest_key_cipher,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(user_id,grant_public_id) DO UPDATE SET
		key_version=excluded.key_version,key_prefix=excluded.key_prefix,base_url=excluded.base_url,
		guest_key_cipher=excluded.guest_key_cipher,updated_at=excluded.updated_at`,
		key.UserID, key.GrantPublicID, key.KeyVersion, key.KeyPrefix, key.BaseURL, ciphertext, created.Unix(), now.Unix())
	return err
}

func (s *Store) LoadCloudReceivedKey(userID int64, grantPublicID string) (*CloudReceivedKey, error) {
	var key CloudReceivedKey
	var ciphertext string
	var createdAt, updatedAt int64
	err := s.db.QueryRow(`SELECT user_id,grant_public_id,key_version,key_prefix,base_url,guest_key_cipher,created_at,updated_at
		FROM cloud_received_keys WHERE user_id=? AND grant_public_id=?`, userID, grantPublicID).Scan(
		&key.UserID, &key.GrantPublicID, &key.KeyVersion, &key.KeyPrefix, &key.BaseURL, &ciphertext, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if key.GuestKey, err = s.cipher.Decrypt(ciphertext); err != nil {
		return nil, err
	}
	key.CreatedAt = unixToTime(createdAt)
	key.UpdatedAt = unixToTime(updatedAt)
	return &key, nil
}

func (s *Store) DeleteCloudReceivedKey(userID int64, grantPublicID string) error {
	_, err := s.db.Exec("DELETE FROM cloud_received_keys WHERE user_id=? AND grant_public_id=?", userID, grantPublicID)
	return err
}

func (s *Store) SaveCloudConnectHostState(state CloudConnectHostState) error {
	ciphertext, err := s.cipher.Encrypt(state.Password)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = s.db.Exec(`INSERT INTO cloud_connect_host_state
		(user_id,connection_code,password_version,password_cipher,expires_at,updated_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET connection_code=excluded.connection_code,
		password_version=excluded.password_version,password_cipher=excluded.password_cipher,
		expires_at=excluded.expires_at,updated_at=excluded.updated_at`, state.UserID, state.ConnectionCode,
		state.PasswordVersion, ciphertext, state.ExpiresAt.Unix(), now.Unix())
	return err
}

func (s *Store) LoadCloudConnectHostState(userID int64) (*CloudConnectHostState, error) {
	var state CloudConnectHostState
	var ciphertext string
	var expiresAt, updatedAt int64
	err := s.db.QueryRow(`SELECT user_id,connection_code,password_version,password_cipher,expires_at,updated_at
		FROM cloud_connect_host_state WHERE user_id=?`, userID).Scan(&state.UserID, &state.ConnectionCode,
		&state.PasswordVersion, &ciphertext, &expiresAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	state.Password, err = s.cipher.Decrypt(ciphertext)
	if err != nil {
		return nil, err
	}
	state.ExpiresAt = unixToTime(expiresAt)
	state.UpdatedAt = unixToTime(updatedAt)
	return &state, nil
}

func (s *Store) DeleteCloudConnectHostState(userID int64) error {
	_, err := s.db.Exec("DELETE FROM cloud_connect_host_state WHERE user_id=?", userID)
	return err
}

func (s *Store) SaveCloudConnectClaimAttempt(attempt CloudConnectClaimAttempt) error {
	passwordCipher, err := s.cipher.Encrypt(attempt.Password)
	if err != nil {
		return err
	}
	guestCipher, err := s.cipher.Encrypt(attempt.GuestKey)
	if err != nil {
		return err
	}
	created := attempt.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	_, err = s.db.Exec(`INSERT INTO cloud_connect_claim_attempts
		(user_id,idempotency_key,connection_code,password_cipher,key_material_json,guest_key_cipher,created_at)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(user_id,idempotency_key) DO NOTHING`, attempt.UserID,
		attempt.IdempotencyKey, attempt.ConnectionCode, passwordCipher, attempt.KeyMaterialJSON, guestCipher, created.Unix())
	return err
}

func (s *Store) LoadCloudConnectClaimAttempt(userID int64, idempotencyKey string) (*CloudConnectClaimAttempt, error) {
	var attempt CloudConnectClaimAttempt
	var passwordCipher, guestCipher string
	var createdAt int64
	err := s.db.QueryRow(`SELECT user_id,idempotency_key,connection_code,password_cipher,key_material_json,
		guest_key_cipher,created_at FROM cloud_connect_claim_attempts WHERE user_id=? AND idempotency_key=?`,
		userID, idempotencyKey).Scan(&attempt.UserID, &attempt.IdempotencyKey, &attempt.ConnectionCode,
		&passwordCipher, &attempt.KeyMaterialJSON, &guestCipher, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if attempt.Password, err = s.cipher.Decrypt(passwordCipher); err != nil {
		return nil, err
	}
	if attempt.GuestKey, err = s.cipher.Decrypt(guestCipher); err != nil {
		return nil, err
	}
	attempt.CreatedAt = unixToTime(createdAt)
	return &attempt, nil
}

func (s *Store) DeleteCloudConnectClaimAttempt(userID int64, idempotencyKey string) error {
	_, err := s.db.Exec("DELETE FROM cloud_connect_claim_attempts WHERE user_id=? AND idempotency_key=?", userID, idempotencyKey)
	return err
}

func (s *Store) SaveCloudReceivedAccountLink(link CloudReceivedAccountLink) error {
	now := time.Now()
	created := link.CreatedAt
	if created.IsZero() {
		created = now
	}
	healthStatus := link.HealthStatus
	if healthStatus == "" {
		healthStatus = "unchecked"
	}
	lastCheckedAt := link.LastCheckedAt.Unix()
	if link.LastCheckedAt.IsZero() {
		lastCheckedAt = 0
	}
	_, err := s.db.Exec(`INSERT INTO cloud_received_account_links
		(user_id,grant_public_id,owner_name,group_name,remote_status,enabled,rpm_limit,concurrency_limit,
		 quota_requests,used_requests,proxy_id,health_status,health_message,last_checked_at,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id,grant_public_id) DO UPDATE SET owner_name=excluded.owner_name,
		group_name=excluded.group_name,remote_status=excluded.remote_status,rpm_limit=excluded.rpm_limit,
		concurrency_limit=excluded.concurrency_limit,quota_requests=excluded.quota_requests,
		used_requests=excluded.used_requests,updated_at=excluded.updated_at`, link.UserID, link.GrantPublicID,
		link.OwnerName, link.GroupName, link.RemoteStatus, boolInt(link.Enabled), max(1, link.RPMLimit),
		max(1, link.ConcurrencyLimit), max(0, link.QuotaRequests), max(0, link.UsedRequests), link.ProxyID,
		healthStatus, link.HealthMessage, lastCheckedAt, created.Unix(), now.Unix())
	return err
}

func (s *Store) SetCloudReceivedAccountLink(userID int64, grantPublicID string, enabled *bool, proxyID *int64, setProxy bool) error {
	if enabled == nil && !setProxy {
		return errors.New("no received share changes")
	}
	if setProxy && proxyID != nil {
		if _, err := s.GetProxy(*proxyID); err != nil {
			return fmt.Errorf("selected proxy: %w", err)
		}
	}
	updates := "updated_at=?"
	values := []any{time.Now().Unix()}
	if enabled != nil {
		updates += ",enabled=?"
		values = append(values, boolInt(*enabled))
	}
	if setProxy {
		updates += ",proxy_id=?"
		values = append(values, proxyID)
	}
	values = append(values, userID, grantPublicID)
	result, err := s.db.Exec("UPDATE cloud_received_account_links SET "+updates+" WHERE user_id=? AND grant_public_id=?", values...)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetCloudReceivedAccountHealth(userID int64, grantPublicID string, enabled bool, healthy bool, message string) error {
	status := "needs_attention"
	if healthy {
		status = "healthy"
	}
	now := time.Now().Unix()
	result, err := s.db.Exec(`UPDATE cloud_received_account_links SET enabled=?,health_status=?,health_message=?,
		last_checked_at=?,updated_at=? WHERE user_id=? AND grant_public_id=?`, boolInt(enabled), status,
		message, now, now, userID, grantPublicID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCloudReceivedAccountLink(userID int64, grantPublicID string) error {
	_, err := s.db.Exec("DELETE FROM cloud_received_account_links WHERE user_id=? AND grant_public_id=?", userID, grantPublicID)
	return err
}

func (s *Store) ListCloudReceivedAccountLinks(userID int64) ([]CloudReceivedAccountLink, error) {
	rows, err := s.db.Query(`SELECT l.id,l.user_id,l.grant_public_id,l.owner_name,l.group_name,l.remote_status,
		l.enabled,l.rpm_limit,l.concurrency_limit,l.quota_requests,l.used_requests,l.proxy_id,
		l.health_status,l.health_message,l.last_checked_at,
		k.base_url,k.guest_key_cipher,l.created_at,l.updated_at FROM cloud_received_account_links l
		JOIN cloud_received_keys k ON k.user_id=l.user_id AND k.grant_public_id=l.grant_public_id
		WHERE l.user_id=? ORDER BY l.updated_at DESC,l.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []CloudReceivedAccountLink
	for rows.Next() {
		var link CloudReceivedAccountLink
		var enabled int
		var keyCipher string
		var createdAt, updatedAt, lastCheckedAt int64
		if err := rows.Scan(&link.ID, &link.UserID, &link.GrantPublicID, &link.OwnerName, &link.GroupName,
			&link.RemoteStatus, &enabled, &link.RPMLimit, &link.ConcurrencyLimit, &link.QuotaRequests,
			&link.UsedRequests, &link.ProxyID, &link.HealthStatus, &link.HealthMessage, &lastCheckedAt,
			&link.BaseURL, &keyCipher, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		link.Enabled = enabled != 0
		link.GuestKey, err = s.cipher.Decrypt(keyCipher)
		if err != nil {
			return nil, err
		}
		link.CreatedAt = unixToTime(createdAt)
		link.UpdatedAt = unixToTime(updatedAt)
		if lastCheckedAt > 0 {
			link.LastCheckedAt = unixToTime(lastCheckedAt)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) ListActiveCloudReceivedAccounts() ([]*Account, error) {
	rows, err := s.db.Query(`SELECT l.id,l.grant_public_id,l.owner_name,l.group_name,l.concurrency_limit,l.proxy_id,
		k.base_url,k.guest_key_cipher,l.created_at,l.updated_at FROM cloud_received_account_links l
		JOIN cloud_received_keys k ON k.user_id=l.user_id AND k.grant_public_id=l.grant_public_id
		JOIN cloud_session cs ON cs.user_id=l.user_id WHERE l.enabled=1 AND l.remote_status='active'
		ORDER BY l.updated_at DESC,l.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []*Account
	for rows.Next() {
		var id, createdAt, updatedAt int64
		var grantID, ownerName, groupName, baseURL, keyCipher string
		var concurrency int
		var proxyID *int64
		if err := rows.Scan(&id, &grantID, &ownerName, &groupName, &concurrency, &proxyID, &baseURL, &keyCipher, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		key, err := s.cipher.Decrypt(keyCipher)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, &Account{
			ID: -id, AccountType: AccountTypeAPIKey, BaseURL: baseURL, APIKey: key,
			Email: ownerName, PlanType: "cloud_share", Status: AccountActive, ProxyID: proxyID,
			MaxConcurrency: max(1, concurrency), QueueCapacity: DefaultAccountQueueCapacity,
			ClientUID: "cloud:" + grantID, CreatedAt: unixToTime(createdAt), UpdatedAt: unixToTime(updatedAt),
		})
		_ = groupName
	}
	return accounts, rows.Err()
}
