package store

import (
	"errors"
	"time"
)

// ResolveAccountNetworkMode preserves legacy behavior while making every
// newly read account's routing choice explicit.
func ResolveAccountNetworkMode(mode AccountNetworkMode, proxyID *int64) AccountNetworkMode {
	if mode != "" {
		return mode
	}
	if proxyID != nil {
		return AccountNetworkProxy
	}
	return AccountNetworkDirect
}

func ValidateAccountNetwork(mode AccountNetworkMode, proxyID *int64) error {
	switch mode {
	case AccountNetworkDirect, AccountNetworkSystem:
		if proxyID != nil {
			return errors.New("proxy_id is only valid in proxy network mode")
		}
	case AccountNetworkProxy:
		if proxyID == nil || *proxyID <= 0 {
			return errors.New("proxy network mode requires a valid proxy_id")
		}
	default:
		return errors.New("network_mode must be direct, system, or proxy")
	}
	return nil
}

func (s *Store) SetAccountNetwork(id int64, mode AccountNetworkMode, proxyID *int64) error {
	mode = ResolveAccountNetworkMode(mode, proxyID)
	if err := ValidateAccountNetwork(mode, proxyID); err != nil {
		return err
	}
	if proxyID != nil {
		if _, err := s.GetProxy(*proxyID); err != nil {
			return err
		}
	}
	result, err := s.db.Exec(`UPDATE accounts SET network_mode=?,proxy_id=?,updated_at=? WHERE id=?`,
		string(mode), proxyID, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrNotFound
	}
	return nil
}
