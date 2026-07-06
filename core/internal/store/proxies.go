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
	)
	if err := row.Scan(&p.ID, &p.Name, &typ, &p.Host, &p.Port, &p.Username, &passEnc, &createdAt); err != nil {
		return nil, err
	}
	pass, err := s.cipher.Decrypt(passEnc)
	if err != nil {
		return nil, err
	}
	p.Type = ProxyType(typ)
	p.Password = pass
	p.CreatedAt = unixToTime(createdAt)
	return &p, nil
}

const proxyCols = `id, name, type, host, port, username, password, created_at`

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

// DeleteProxy removes a proxy and clears it from any accounts referencing it.
func (s *Store) DeleteProxy(id int64) error {
	if _, err := s.db.Exec(`UPDATE accounts SET proxy_id=NULL WHERE proxy_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM proxies WHERE id=?`, id)
	return err
}
