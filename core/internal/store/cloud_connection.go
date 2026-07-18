package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CloudConnectionMode string

const (
	CloudConnectionSystem CloudConnectionMode = "system"
	CloudConnectionProxy  CloudConnectionMode = "proxy"
	CloudConnectionDirect CloudConnectionMode = "direct"
)

type CloudConnectionSettings struct {
	Mode      CloudConnectionMode `json:"mode"`
	ProxyID   *int64              `json:"proxy_id"`
	UpdatedAt time.Time           `json:"updated_at"`
}

func ValidateCloudConnectionSettings(value CloudConnectionSettings) error {
	switch value.Mode {
	case CloudConnectionSystem, CloudConnectionDirect:
		if value.ProxyID != nil {
			return errors.New("proxy_id is only valid in proxy mode")
		}
	case CloudConnectionProxy:
		if value.ProxyID == nil || *value.ProxyID <= 0 {
			return errors.New("proxy mode requires a valid proxy_id")
		}
	default:
		return fmt.Errorf("unsupported cloud connection mode %q", strings.TrimSpace(string(value.Mode)))
	}
	return nil
}

func (s *Store) CloudConnectionSettings() (CloudConnectionSettings, error) {
	var value CloudConnectionSettings
	var mode string
	var proxyID sql.NullInt64
	var updatedAt int64
	err := s.db.QueryRow(`SELECT mode,proxy_id,updated_at FROM cloud_connection_settings WHERE id=1`).Scan(&mode, &proxyID, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudConnectionSettings{Mode: CloudConnectionSystem}, nil
	}
	if err != nil {
		return value, err
	}
	value.Mode = CloudConnectionMode(mode)
	if proxyID.Valid {
		value.ProxyID = &proxyID.Int64
	}
	value.UpdatedAt = unixToTime(updatedAt)
	return value, nil
}

func (s *Store) SaveCloudConnectionSettings(value CloudConnectionSettings) error {
	if err := ValidateCloudConnectionSettings(value); err != nil {
		return err
	}
	if value.Mode == CloudConnectionProxy {
		if _, err := s.GetProxy(*value.ProxyID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("selected cloud proxy: %w", ErrNotFound)
			}
			return err
		}
	}
	_, err := s.db.Exec(`INSERT INTO cloud_connection_settings(id,mode,proxy_id,updated_at)
		VALUES(1,?,?,?) ON CONFLICT(id) DO UPDATE SET mode=excluded.mode,proxy_id=excluded.proxy_id,updated_at=excluded.updated_at`,
		string(value.Mode), value.ProxyID, time.Now().Unix())
	return err
}
