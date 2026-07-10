// Package store implements SQLite-backed persistence for accounts, proxies,
// request logs and settings. Sensitive fields are encrypted via crypto.Cipher.
package store

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"

	"sub2api-desktop/core/internal/crypto"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// Store owns the database connection and encryption cipher.
type Store struct {
	db              *sql.DB
	cipher          *crypto.Cipher
	dbPath          string
	schemaVersion   int
	migrationBackup string
}

// Open opens (and migrates) the SQLite database at dbPath.
func Open(dbPath string, cipher *crypto.Cipher) (*Store, error) {
	existed := databaseExists(dbPath)
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: serialize writes; WAL still allows concurrent reads via separate conns
	s := &Store{db: db, cipher: cipher, dbPath: dbPath}
	version, backup, err := s.migrate(existed)
	if err != nil {
		_ = db.Close()
		if backup != "" {
			if restoreErr := restoreDatabase(dbPath, backup); restoreErr != nil {
				return nil, errors.Join(err, restoreErr)
			}
		}
		return nil, err
	}
	s.schemaVersion = version
	s.migrationBackup = backup
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// SchemaVersion is the latest successfully applied database migration.
func (s *Store) SchemaVersion() int { return s.schemaVersion }

// MigrationBackup returns the upgrade backup created during this process run.
func (s *Store) MigrationBackup() string { return s.migrationBackup }

func unixToTime(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func timeToUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
