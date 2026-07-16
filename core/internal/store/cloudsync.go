package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	CloudKindAccount     = "account"
	CloudKindProxy       = "proxy"
	CloudKindCodexRemote = "codex_remote"
	CloudKindSettings    = "settings"
)

type CloudSession struct {
	UserID          int64
	Email           string
	Role            string
	SaltKDF         string
	SaltAuth        string
	WrappedVaultKey string
	VaultKey        string
	RefreshToken    string
	SyncCursor      string
	LastSyncAt      time.Time
}

type CloudTombstone struct {
	Kind        string
	ClientUID   string
	SyncVersion int
	UpdatedAt   time.Time
}

type CloudSettingsState struct {
	ClientUID   string
	SyncVersion int
	SyncDirty   bool
	UpdatedAt   time.Time
}

type CloudItemMeta struct {
	Kind        string
	ClientUID   string
	SyncVersion int
	SyncDirty   bool
	UpdatedAt   time.Time
}

type CloudConflict struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"`
	ClientUID   string    `json:"client_uid"`
	DisplayName string    `json:"display_name,omitempty"`
	Resolution  string    `json:"resolution"`
	Details     string    `json:"details,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) SaveCloudSession(session CloudSession) error {
	vaultCipher, err := s.cipher.Encrypt(session.VaultKey)
	if err != nil {
		return err
	}
	refreshCipher, err := s.cipher.Encrypt(session.RefreshToken)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err = s.db.Exec(`INSERT INTO cloud_session
		(id,user_id,email,role,salt_kdf,salt_auth,wrapped_vault_key,vault_key_cipher,refresh_token_cipher,sync_cursor,last_sync_at,created_at,updated_at)
		VALUES(1,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET user_id=excluded.user_id,email=excluded.email,role=excluded.role,
		salt_kdf=excluded.salt_kdf,salt_auth=excluded.salt_auth,wrapped_vault_key=excluded.wrapped_vault_key,
		vault_key_cipher=excluded.vault_key_cipher,refresh_token_cipher=excluded.refresh_token_cipher,
		sync_cursor=excluded.sync_cursor,last_sync_at=excluded.last_sync_at,updated_at=excluded.updated_at`,
		session.UserID, session.Email, session.Role, session.SaltKDF, session.SaltAuth, session.WrappedVaultKey,
		vaultCipher, refreshCipher, session.SyncCursor, timeToUnix(session.LastSyncAt), now, now)
	return err
}

func (s *Store) LoadCloudSession() (*CloudSession, error) {
	var session CloudSession
	var vaultCipher, refreshCipher string
	var lastSync int64
	err := s.db.QueryRow(`SELECT user_id,email,role,salt_kdf,salt_auth,wrapped_vault_key,
		vault_key_cipher,refresh_token_cipher,sync_cursor,last_sync_at FROM cloud_session WHERE id=1`).Scan(
		&session.UserID, &session.Email, &session.Role, &session.SaltKDF, &session.SaltAuth, &session.WrappedVaultKey,
		&vaultCipher, &refreshCipher, &session.SyncCursor, &lastSync)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if session.VaultKey, err = s.cipher.Decrypt(vaultCipher); err != nil {
		return nil, err
	}
	if session.RefreshToken, err = s.cipher.Decrypt(refreshCipher); err != nil {
		return nil, err
	}
	session.LastSyncAt = unixToTime(lastSync)
	return &session, nil
}

func (s *Store) DeleteCloudSession() error {
	_, err := s.db.Exec("DELETE FROM cloud_session WHERE id=1")
	return err
}

func (s *Store) UpdateCloudSessionProgress(refreshToken, cursor string, syncedAt time.Time) error {
	refreshCipher, err := s.cipher.Encrypt(refreshToken)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE cloud_session SET refresh_token_cipher=?,sync_cursor=?,last_sync_at=?,updated_at=? WHERE id=1`,
		refreshCipher, cursor, timeToUnix(syncedAt), time.Now().Unix())
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetCloudApplying(value bool) error {
	flag := "0"
	if value {
		flag = "1"
	}
	_, err := s.db.Exec(`INSERT INTO cloud_sync_runtime(key,value) VALUES('applying',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, flag)
	return err
}

func (s *Store) cloudApplying() (bool, error) {
	var value string
	if err := s.db.QueryRow(`SELECT value FROM cloud_sync_runtime WHERE key='applying'`).Scan(&value); err != nil {
		return false, err
	}
	return value == "1", nil
}

func (s *Store) CloudTombstones() ([]CloudTombstone, error) {
	rows, err := s.db.Query(`SELECT kind,client_uid,sync_version,updated_at FROM cloud_sync_tombstones ORDER BY updated_at,kind,client_uid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CloudTombstone
	for rows.Next() {
		var item CloudTombstone
		var updated int64
		if err := rows.Scan(&item.Kind, &item.ClientUID, &item.SyncVersion, &updated); err != nil {
			return nil, err
		}
		item.UpdatedAt = unixToTime(updated)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CloudSettingsState() (CloudSettingsState, error) {
	var state CloudSettingsState
	var dirty int
	var updated int64
	err := s.db.QueryRow(`SELECT client_uid,sync_version,sync_dirty,updated_at FROM cloud_sync_settings WHERE id=1`).Scan(
		&state.ClientUID, &state.SyncVersion, &dirty, &updated)
	state.SyncDirty = dirty != 0
	state.UpdatedAt = unixToTime(updated)
	return state, err
}

func (s *Store) ApplyCloudSettingsState(clientUID string, version int, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	_, err := s.db.Exec(`UPDATE cloud_sync_settings SET client_uid=?,sync_version=?,sync_dirty=0,updated_at=? WHERE id=1`,
		clientUID, version, updatedAt.Unix())
	return err
}

func (s *Store) MarkCloudItemSynced(kind, clientUID string, version int) error {
	var table string
	switch kind {
	case CloudKindAccount:
		table = "accounts"
	case CloudKindProxy:
		table = "proxies"
	case CloudKindCodexRemote:
		table = "codex_remote_targets"
	case CloudKindSettings:
		_, err := s.db.Exec(`UPDATE cloud_sync_settings SET sync_version=?,sync_dirty=0 WHERE client_uid=?`, version, clientUID)
		return err
	default:
		return fmt.Errorf("unknown cloud item kind %q", kind)
	}
	_, err := s.db.Exec(`UPDATE `+table+` SET sync_version=?,sync_dirty=0 WHERE client_uid=?`, version, clientUID)
	return err
}

func (s *Store) RebaseCloudItem(kind, clientUID string, version int) error {
	var table string
	switch kind {
	case CloudKindAccount:
		table = "accounts"
	case CloudKindProxy:
		table = "proxies"
	case CloudKindCodexRemote:
		table = "codex_remote_targets"
	case CloudKindSettings:
		_, err := s.db.Exec(`UPDATE cloud_sync_settings SET sync_version=? WHERE client_uid=?`, version, clientUID)
		return err
	default:
		return fmt.Errorf("unknown cloud item kind %q", kind)
	}
	_, err := s.db.Exec(`UPDATE `+table+` SET sync_version=? WHERE client_uid=?`, version, clientUID)
	return err
}

func (s *Store) MarkCloudTombstoneSynced(kind, clientUID string) error {
	_, err := s.db.Exec(`DELETE FROM cloud_sync_tombstones WHERE kind=? AND client_uid=?`, kind, clientUID)
	return err
}

func (s *Store) PendingCloudCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM accounts WHERE sync_dirty=1)+
		(SELECT COUNT(*) FROM proxies WHERE sync_dirty=1)+
		(SELECT COUNT(*) FROM codex_remote_targets WHERE sync_dirty=1)+
		(SELECT COUNT(*) FROM cloud_sync_settings WHERE sync_dirty=1)+
		(SELECT COUNT(*) FROM cloud_sync_tombstones)`).Scan(&count)
	return count, err
}

func (s *Store) CloudItemMeta(kind, clientUID string) (CloudItemMeta, error) {
	meta := CloudItemMeta{Kind: kind, ClientUID: clientUID}
	var dirty int
	var updated int64
	var err error
	switch kind {
	case CloudKindAccount:
		err = s.db.QueryRow(`SELECT sync_version,sync_dirty,updated_at FROM accounts WHERE client_uid=?`, clientUID).Scan(&meta.SyncVersion, &dirty, &updated)
	case CloudKindProxy:
		err = s.db.QueryRow(`SELECT sync_version,sync_dirty,updated_at FROM proxies WHERE client_uid=?`, clientUID).Scan(&meta.SyncVersion, &dirty, &updated)
	case CloudKindCodexRemote:
		err = s.db.QueryRow(`SELECT sync_version,sync_dirty,updated_at FROM codex_remote_targets WHERE client_uid=?`, clientUID).Scan(&meta.SyncVersion, &dirty, &updated)
	case CloudKindSettings:
		err = s.db.QueryRow(`SELECT sync_version,sync_dirty,updated_at FROM cloud_sync_settings WHERE client_uid=?`, clientUID).Scan(&meta.SyncVersion, &dirty, &updated)
	default:
		return meta, ErrNotFound
	}
	if errors.Is(err, sql.ErrNoRows) {
		return meta, ErrNotFound
	}
	meta.SyncDirty = dirty != 0
	meta.UpdatedAt = unixToTime(updated)
	return meta, err
}

func (s *Store) AddCloudConflict(kind, clientUID, resolution, details string) error {
	_, err := s.db.Exec(`INSERT INTO cloud_sync_conflicts(kind,client_uid,resolution,details,created_at) VALUES(?,?,?,?,?)`,
		kind, clientUID, resolution, details, time.Now().Unix())
	return err
}

func (s *Store) ListCloudConflicts(limit int) ([]CloudConflict, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id,kind,client_uid,resolution,details,created_at FROM cloud_sync_conflicts ORDER BY created_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CloudConflict
	for rows.Next() {
		var item CloudConflict
		var created int64
		if err := rows.Scan(&item.ID, &item.Kind, &item.ClientUID, &item.Resolution, &item.Details, &created); err != nil {
			return nil, err
		}
		item.CreatedAt = unixToTime(created)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range result {
		result[index].DisplayName = s.cloudConflictDisplayName(result[index].Kind, result[index].ClientUID)
	}
	return result, nil
}

func (s *Store) cloudConflictDisplayName(kind, clientUID string) string {
	var name string
	var err error
	switch kind {
	case CloudKindAccount:
		err = s.db.QueryRow(`SELECT COALESCE(NULLIF(email,''),NULLIF(chatgpt_account_id,''),base_url) FROM accounts WHERE client_uid=?`, clientUID).Scan(&name)
	case CloudKindProxy:
		err = s.db.QueryRow(`SELECT name FROM proxies WHERE client_uid=?`, clientUID).Scan(&name)
	case CloudKindCodexRemote:
		err = s.db.QueryRow(`SELECT name FROM codex_remote_targets WHERE client_uid=?`, clientUID).Scan(&name)
	default:
		return ""
	}
	if err != nil {
		return ""
	}
	return name
}

func (s *Store) GetProxyByClientUID(clientUID string) (*Proxy, error) {
	proxy, err := s.scanProxy(s.db.QueryRow(`SELECT `+proxyCols+` FROM proxies WHERE client_uid=?`, clientUID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return proxy, err
}

func (s *Store) GetAccountByClientUID(clientUID string) (*Account, error) {
	account, err := s.scanAccount(s.db.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE client_uid=?`, clientUID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return account, err
}

func (s *Store) GetCodexRemoteTargetByClientUID(clientUID string) (*CodexRemoteTarget, error) {
	target, err := s.scanCodexRemoteTarget(s.db.QueryRow(`SELECT `+codexRemoteTargetCols+` FROM codex_remote_targets WHERE client_uid=?`, clientUID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return target, err
}

func (s *Store) ApplyCloudProxy(proxy *Proxy, version int, updatedAt time.Time) error {
	passwordCipher, err := s.cipher.Encrypt(proxy.Password)
	if err != nil {
		return err
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	createdAt := proxy.CreatedAt
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	_, err = s.db.Exec(`INSERT INTO proxies(name,type,host,port,username,password,created_at,client_uid,sync_version,sync_dirty,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,0,?) ON CONFLICT(client_uid) DO UPDATE SET name=excluded.name,type=excluded.type,
		host=excluded.host,port=excluded.port,username=excluded.username,password=excluded.password,sync_version=excluded.sync_version,
		sync_dirty=0,updated_at=excluded.updated_at`, proxy.Name, string(proxy.Type), proxy.Host, proxy.Port, proxy.Username,
		passwordCipher, createdAt.Unix(), proxy.ClientUID, version, updatedAt.Unix())
	return err
}

func (s *Store) ApplyCloudAccount(account *Account, proxyUID string, version int, updatedAt time.Time) error {
	accessCipher, err := s.cipher.Encrypt(account.AccessToken)
	if err != nil {
		return err
	}
	refreshCipher, err := s.cipher.Encrypt(account.RefreshToken)
	if err != nil {
		return err
	}
	idCipher, err := s.cipher.Encrypt(account.IDToken)
	if err != nil {
		return err
	}
	apiKeyCipher, err := s.cipher.Encrypt(account.APIKey)
	if err != nil {
		return err
	}
	var proxyID any
	if proxyUID != "" {
		proxy, proxyErr := s.GetProxyByClientUID(proxyUID)
		if proxyErr == nil {
			proxyID = proxy.ID
		} else if !errors.Is(proxyErr, ErrNotFound) {
			return proxyErr
		}
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	createdAt := account.CreatedAt
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	fingerprint := AccountCredentialFingerprint(account.AccountType, account.AccessToken, account.RefreshToken, account.BaseURL, account.APIKey)
	_, err = s.db.Exec(`INSERT INTO accounts
		(account_type,base_url,api_key,email,chatgpt_account_id,plan_type,access_token,refresh_token,id_token,expires_at,
		status,status_reason,rate_limited_until,proxy_id,last_used_at,created_at,updated_at,usage_snapshot,credential_fingerprint,
		last_success_at,consecutive_failures,next_retry_at,client_uid,sync_version,sync_dirty)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,0,?,0,?,?, '',?,0,0,0,?,?,0)
		ON CONFLICT(client_uid) DO UPDATE SET account_type=excluded.account_type,base_url=excluded.base_url,api_key=excluded.api_key,
		email=excluded.email,chatgpt_account_id=excluded.chatgpt_account_id,plan_type=excluded.plan_type,
		access_token=excluded.access_token,refresh_token=excluded.refresh_token,id_token=excluded.id_token,expires_at=excluded.expires_at,
		status=excluded.status,status_reason=excluded.status_reason,proxy_id=excluded.proxy_id,updated_at=excluded.updated_at,
		credential_fingerprint=excluded.credential_fingerprint,sync_version=excluded.sync_version,sync_dirty=0`,
		string(account.AccountType), account.BaseURL, apiKeyCipher, account.Email, account.ChatGPTAccountID, account.PlanType,
		accessCipher, refreshCipher, idCipher, timeToUnix(account.ExpiresAt), string(account.Status), account.StatusReason,
		proxyID, createdAt.Unix(), updatedAt.Unix(), fingerprint, account.ClientUID, version)
	return err
}

func (s *Store) ApplyCloudCodexRemote(target *CodexRemoteTarget, version int, updatedAt time.Time) error {
	passwordCipher, err := s.cipher.Encrypt(target.Password)
	if err != nil {
		return err
	}
	apiKeyCipher, err := s.cipher.Encrypt(target.APIKey)
	if err != nil {
		return err
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	createdAt := target.CreatedAt
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	_, err = s.db.Exec(`INSERT INTO codex_remote_targets
		(name,host,port,user,password_cipher,remote_port,model,mode,base_url,api_key_cipher,tunnel_enabled,injected,
		created_at,updated_at,client_uid,sync_version,sync_dirty)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0) ON CONFLICT(client_uid) DO UPDATE SET name=excluded.name,host=excluded.host,
		port=excluded.port,user=excluded.user,password_cipher=excluded.password_cipher,remote_port=excluded.remote_port,
		model=excluded.model,mode=excluded.mode,base_url=excluded.base_url,api_key_cipher=excluded.api_key_cipher,
		tunnel_enabled=excluded.tunnel_enabled,injected=excluded.injected,updated_at=excluded.updated_at,
		sync_version=excluded.sync_version,sync_dirty=0`, target.Name, target.Host, target.Port, target.User, passwordCipher,
		target.RemotePort, target.Model, target.Mode, target.BaseURL, apiKeyCipher, boolInt(target.TunnelEnabled),
		boolInt(target.Injected), createdAt.Unix(), updatedAt.Unix(), target.ClientUID, version)
	return err
}

func (s *Store) DeleteCloudItem(kind, clientUID string) error {
	var table string
	switch kind {
	case CloudKindAccount:
		table = "accounts"
	case CloudKindProxy:
		table = "proxies"
	case CloudKindCodexRemote:
		table = "codex_remote_targets"
	default:
		return fmt.Errorf("unknown cloud item kind %q", kind)
	}
	_, err := s.db.Exec(`DELETE FROM `+table+` WHERE client_uid=?`, clientUID)
	return err
}
