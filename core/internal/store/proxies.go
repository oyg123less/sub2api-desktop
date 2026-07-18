package store

import (
	"database/sql"
	"errors"
	"time"
)

func (s *Store) scanProxy(row interface {
	Scan(dest ...any) error
}) (*Proxy, error) {
	var (
		p         Proxy
		typ       string
		passEnc   string
		createdAt int64
		updatedAt int64
		syncDirty int
	)
	if err := row.Scan(&p.ID, &p.Name, &typ, &p.Host, &p.Port, &p.Username, &passEnc, &createdAt,
		&p.ClientUID, &p.SyncVersion, &syncDirty, &updatedAt); err != nil {
		return nil, err
	}
	pass, err := s.cipher.Decrypt(passEnc)
	if err != nil {
		return nil, err
	}
	p.Type = ProxyType(typ)
	p.Password = pass
	p.CreatedAt = unixToTime(createdAt)
	p.UpdatedAt = unixToTime(updatedAt)
	p.SyncDirty = syncDirty != 0
	return &p, nil
}

const proxyCols = `id, name, type, host, port, username, password, created_at, client_uid, sync_version, sync_dirty, updated_at`

// CreateProxy inserts a proxy (password encrypted).
func (s *Store) CreateProxy(p *Proxy) (*Proxy, error) {
	passEnc, err := s.cipher.Encrypt(p.Password)
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(`INSERT INTO proxies (name, type, host, port, username, password, created_at) VALUES (?,?,?,?,?,?,?)`,
		p.Name, string(p.Type), p.Host, p.Port, p.Username, passEnc, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetProxy(id)
}

// GetProxy fetches a proxy by id.
func (s *Store) GetProxy(id int64) (*Proxy, error) {
	row := s.db.QueryRow(`SELECT `+proxyCols+` FROM proxies WHERE id=?`, id)
	p, err := s.scanProxy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// ListProxies returns all proxies.
func (s *Store) ListProxies() ([]*Proxy, error) {
	rows, err := s.db.Query(`SELECT ` + proxyCols + ` FROM proxies ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Proxy
	for rows.Next() {
		p, err := s.scanProxy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProxy updates an existing proxy's fields (password re-encrypted).
func (s *Store) UpdateProxy(id int64, p *Proxy) (*Proxy, error) {
	passEnc, err := s.cipher.Encrypt(p.Password)
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(`UPDATE proxies SET name=?, type=?, host=?, port=?, username=?, password=? WHERE id=?`,
		p.Name, string(p.Type), p.Host, p.Port, p.Username, passEnc, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetProxy(id)
}

type ProxyPatch struct {
	Name          *string
	Type          *ProxyType
	Host          *string
	Port          *int
	Username      *string
	Password      *string
	ClearPassword bool
}

// UpdateProxyPatch preserves the encrypted password unless the caller sends a
// non-empty replacement or explicitly requests clearing it.
func (s *Store) UpdateProxyPatch(id int64, patch ProxyPatch) (*Proxy, error) {
	existing, err := s.GetProxy(id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Type != nil {
		existing.Type = *patch.Type
	}
	if patch.Host != nil {
		existing.Host = *patch.Host
	}
	if patch.Port != nil {
		existing.Port = *patch.Port
	}
	if patch.Username != nil {
		existing.Username = *patch.Username
	}
	if patch.ClearPassword {
		existing.Password = ""
	} else if patch.Password != nil && *patch.Password != "" {
		existing.Password = *patch.Password
	}
	return s.UpdateProxy(id, existing)
}

// DeleteProxy removes a proxy and clears it from any accounts referencing it.
func (s *Store) DeleteProxy(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE accounts SET proxy_id=NULL WHERE proxy_id=?`, id); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM proxies WHERE id=?`, id)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return err
		}
		return ErrNotFound
	}
	return tx.Commit()
}
