package store

import (
	"database/sql"
	"errors"
	"time"
)

const codexRemoteTargetCols = `id, name, host, port, user, password_cipher, remote_port, model, mode, base_url, api_key_cipher, tunnel_enabled, injected, created_at, updated_at, client_uid, sync_version, sync_dirty`

func (s *Store) scanCodexRemoteTarget(row interface{ Scan(...any) error }) (*CodexRemoteTarget, error) {
	var target CodexRemoteTarget
	var passwordCipher, apiKeyCipher string
	var tunnelEnabled, injected int
	var createdAt, updatedAt int64
	var syncDirty int
	if err := row.Scan(&target.ID, &target.Name, &target.Host, &target.Port, &target.User, &passwordCipher,
		&target.RemotePort, &target.Model, &target.Mode, &target.BaseURL, &apiKeyCipher,
		&tunnelEnabled, &injected, &createdAt, &updatedAt, &target.ClientUID, &target.SyncVersion, &syncDirty); err != nil {
		return nil, err
	}
	password, err := s.cipher.Decrypt(passwordCipher)
	if err != nil {
		return nil, err
	}
	target.Password = password
	if apiKeyCipher != "" {
		apiKey, err := s.cipher.Decrypt(apiKeyCipher)
		if err != nil {
			return nil, err
		}
		target.APIKey = apiKey
	}
	if target.Mode == "" {
		target.Mode = "tunnel"
	}
	target.TunnelEnabled = tunnelEnabled != 0
	target.Injected = injected != 0
	target.CreatedAt = unixToTime(createdAt)
	target.UpdatedAt = unixToTime(updatedAt)
	target.SyncDirty = syncDirty != 0
	return &target, nil
}

func (s *Store) CreateCodexRemoteTarget(target *CodexRemoteTarget) (*CodexRemoteTarget, error) {
	passwordCipher, err := s.cipher.Encrypt(target.Password)
	if err != nil {
		return nil, err
	}
	apiKeyCipher, err := encryptOptional(s, target.APIKey)
	if err != nil {
		return nil, err
	}
	if target.Mode == "" {
		target.Mode = "tunnel"
	}
	now := time.Now().Unix()
	result, err := s.db.Exec(`INSERT INTO codex_remote_targets
		(name, host, port, user, password_cipher, remote_port, model, mode, base_url, api_key_cipher, tunnel_enabled, injected, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, target.Name, target.Host, target.Port, target.User, passwordCipher,
		target.RemotePort, target.Model, target.Mode, target.BaseURL, apiKeyCipher,
		boolInt(target.TunnelEnabled), boolInt(target.Injected), now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetCodexRemoteTarget(id)
}

func (s *Store) UpdateCodexRemoteTarget(target *CodexRemoteTarget) (*CodexRemoteTarget, error) {
	passwordCipher, err := s.cipher.Encrypt(target.Password)
	if err != nil {
		return nil, err
	}
	apiKeyCipher, err := s.codexRemoteAPIKeyCipherForUpdate(target)
	if err != nil {
		return nil, err
	}
	if target.Mode == "" {
		target.Mode = "tunnel"
	}
	result, err := s.db.Exec(`UPDATE codex_remote_targets SET name=?, host=?, port=?, user=?, password_cipher=?,
		remote_port=?, model=?, mode=?, base_url=?, api_key_cipher=?, tunnel_enabled=?, injected=?, updated_at=? WHERE id=?`,
		target.Name, target.Host, target.Port, target.User, passwordCipher, target.RemotePort, target.Model,
		target.Mode, target.BaseURL, apiKeyCipher, boolInt(target.TunnelEnabled), boolInt(target.Injected), time.Now().Unix(), target.ID)
	if err != nil {
		return nil, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return nil, ErrNotFound
	}
	return s.GetCodexRemoteTarget(target.ID)
}

func (s *Store) codexRemoteAPIKeyCipherForUpdate(target *CodexRemoteTarget) (string, error) {
	if target.APIKey != "" {
		return s.cipher.Encrypt(target.APIKey)
	}
	if target.Mode != "direct" {
		return "", nil
	}
	var encrypted string
	if err := s.db.QueryRow(`SELECT api_key_cipher FROM codex_remote_targets WHERE id=?`, target.ID).Scan(&encrypted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return encrypted, nil
}

func encryptOptional(s *Store, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return s.cipher.Encrypt(value)
}

func (s *Store) GetCodexRemoteTarget(id int64) (*CodexRemoteTarget, error) {
	target, err := s.scanCodexRemoteTarget(s.db.QueryRow(`SELECT `+codexRemoteTargetCols+` FROM codex_remote_targets WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return target, err
}

func (s *Store) ListCodexRemoteTargets() ([]*CodexRemoteTarget, error) {
	rows, err := s.db.Query(`SELECT ` + codexRemoteTargetCols + ` FROM codex_remote_targets ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	targets := []*CodexRemoteTarget{}
	for rows.Next() {
		target, err := s.scanCodexRemoteTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func (s *Store) DeleteCodexRemoteTarget(id int64) error {
	result, err := s.db.Exec(`DELETE FROM codex_remote_targets WHERE id=?`, id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return ErrNotFound
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
