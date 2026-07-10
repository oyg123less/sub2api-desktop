package store

import (
	"os"
	"time"
)

type LogHealth struct {
	Rows            int64  `json:"rows"`
	DatabaseBytes   int64  `json:"database_bytes"`
	LatestErrorKind string `json:"latest_error_kind,omitempty"`
	OldestAt        int64  `json:"oldest_at,omitempty"`
	NewestAt        int64  `json:"newest_at,omitempty"`
}

// QuickCheck runs SQLite's lightweight integrity check.
func (s *Store) QuickCheck() error { return quickCheck(s.db) }

func (s *Store) DatabasePath() string { return s.dbPath }

func (s *Store) LogHealth() (LogHealth, error) {
	var health LogHealth
	err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(MIN(created_at),0), COALESCE(MAX(created_at),0) FROM request_logs`).
		Scan(&health.Rows, &health.OldestAt, &health.NewestAt)
	if err != nil {
		return health, err
	}
	_ = s.db.QueryRow(`SELECT error_kind FROM request_logs WHERE error_kind<>'' ORDER BY id DESC LIMIT 1`).Scan(&health.LatestErrorKind)
	if info, err := os.Stat(s.dbPath); err == nil {
		health.DatabaseBytes = info.Size()
	}
	return health, nil
}

func (s *Store) CleanupLogs(retentionDays int, maxRows int) (int64, error) {
	var total int64
	cutoff := int64(0)
	if retentionDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -retentionDays).Unix()
	}
	for {
		result, err := s.db.Exec(`DELETE FROM request_logs WHERE id IN (
			SELECT id FROM request_logs
			WHERE (? > 0 AND created_at < ?)
			   OR (? > 0 AND id <= (SELECT COALESCE(MAX(id),0) - ? FROM request_logs))
			ORDER BY id ASC LIMIT 1000
		)`, cutoff, cutoff, maxRows, maxRows)
		if err != nil {
			return total, err
		}
		deleted, _ := result.RowsAffected()
		total += deleted
		if deleted < 1000 {
			return total, nil
		}
	}
}
