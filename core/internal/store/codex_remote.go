package store

import (
	"database/sql"
	"errors"
	"time"
)

const codexRemoteTargetCols = `id, name, host, port, user, password_cipher, remote_port, model, tunnel_enabled, injected, created_at, updated_at`

func (s *Store) scanCodexRemoteTarget(row interface{ Scan(...any) error }) (*CodexRemoteTarget, error) {
	var target CodexRemoteTarget
	var passwordCipher string
	var tunnelEnabled, injected int
	var createdAt, updatedAt int64
	if err := row.Scan(&target.ID, &target.Name, &target.Host, &target.Port, &target.User, &passwordCipher,
		&target.RemotePort, &target.Model, &tunnelEnabled, &injected, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	password, err := s.cipher.Decrypt(passwordCipher)
	if err != nil {
		return nil, err
	}
	target.Password = password
	target.TunnelEnabled = tunnelEnabled != 0
	target.Injected = injected != 0
	target.CreatedAt = unixToTime(createdAt)
	target.UpdatedAt = unixToTime(updatedAt)
	return &target, nil
}

func (s *Store) CreateCodexRemoteTarget(target *CodexRemoteTarget) (*CodexRemoteTarget, error) {
	passwordCipher, err := s.cipher.Encrypt(target.Password)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	result, err := s.db.Exec(`INSERT INTO codex_remote_targets
		(name, host, port, user, password_cipher, remote_port, model, tunnel_enabled, injected, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, target.Name, target.Host, target.Port, target.User, passwordCipher,
		target.RemotePort, target.Model, boolInt(target.TunnelEnabled), boolInt(target.Injected), now, now)
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
	result, err := s.db.Exec(`UPDATE codex_remote_targets SET name=?, host=?, port=?, user=?, password_cipher=?,
		remote_port=?, model=?, tunnel_enabled=?, injected=?, updated_at=? WHERE id=?`,
		target.Name, target.Host, target.Port, target.User, passwordCipher, target.RemotePort, target.Model,
		boolInt(target.TunnelEnabled), boolInt(target.Injected), time.Now().Unix(), target.ID)
	if err != nil {
		return nil, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return nil, ErrNotFound
	}
	return s.GetCodexRemoteTarget(target.ID)
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
