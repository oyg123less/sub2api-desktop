package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appcrypto "sub2api-desktop/core/internal/crypto"
)

const CurrentSchemaVersion = 6

type migration struct {
	version int
	name    string
	apply   func(*sql.Tx, *appcrypto.Cipher) error
}

var migrations = []migration{
	{version: 1, name: "v0.1.1 baseline", apply: migrateBaseline},
	{version: 2, name: "v0.2.0 reliability", apply: migrateV020},
	{version: 3, name: "v0.2.0 compatibility settings", apply: migrateV020CompatibilitySettings},
	{version: 4, name: "v0.2.2 api-key accounts", apply: migrateV022APIKeyAccounts},
	{version: 5, name: "v0.2.3 Codex remote targets", apply: migrateV023CodexRemoteTargets},
	{version: 6, name: "v0.2.4 Codex direct remote targets", apply: migrateV024CodexDirectTargets},
}

func databaseExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func (s *Store) migrate(existed bool) (int, string, error) {
	current, err := currentSchemaVersion(s.db)
	if err != nil {
		return 0, "", fmt.Errorf("inspect schema version: %w", err)
	}
	if current > CurrentSchemaVersion {
		return current, "", fmt.Errorf("database schema %d is newer than supported schema %d", current, CurrentSchemaVersion)
	}

	backup := ""
	if existed && current < CurrentSchemaVersion {
		backup, err = backupDatabase(s.db, s.dbPath)
		if err != nil {
			return current, "", fmt.Errorf("create pre-v0.2.0 backup: %w", err)
		}
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return current, backup, fmt.Errorf("start migration %d: %w", m.version, err)
		}
		if err := m.apply(tx, s.cipher); err != nil {
			_ = tx.Rollback()
			return current, backup, fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, name, applied_at) VALUES(?,?,?)`, m.version, m.name, time.Now().Unix()); err != nil {
			_ = tx.Rollback()
			return current, backup, fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return current, backup, fmt.Errorf("commit migration %d: %w", m.version, err)
		}
		current = m.version
	}

	if err := quickCheck(s.db); err != nil {
		return current, backup, err
	}
	return current, backup, nil
}

func currentSchemaVersion(db *sql.DB) (int, error) {
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&exists); err != nil {
		return 0, err
	}
	if exists == 0 {
		return 0, nil
	}
	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func migrateBaseline(tx *sql.Tx, _ *appcrypto.Cipher) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL DEFAULT '',
			chatgpt_account_id TEXT NOT NULL DEFAULT '',
			plan_type TEXT NOT NULL DEFAULT '',
			access_token TEXT NOT NULL DEFAULT '',
			refresh_token TEXT NOT NULL DEFAULT '',
			id_token TEXT NOT NULL DEFAULT '',
			expires_at INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			status_reason TEXT NOT NULL DEFAULT '',
			rate_limited_until INTEGER NOT NULL DEFAULT 0,
			proxy_id INTEGER,
			last_used_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			usage_snapshot TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS proxies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT 'http',
			host TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER,
			account_email TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			status_code INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			stream INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_created ON request_logs(created_at)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return addColumnIfMissing(tx, "accounts", "usage_snapshot", `TEXT NOT NULL DEFAULT ''`)
}

func migrateV020(tx *sql.Tx, cipher *appcrypto.Cipher) error {
	accountColumns := []struct{ name, declaration string }{
		{"credential_fingerprint", `TEXT NOT NULL DEFAULT ''`},
		{"last_success_at", `INTEGER NOT NULL DEFAULT 0`},
		{"consecutive_failures", `INTEGER NOT NULL DEFAULT 0`},
		{"next_retry_at", `INTEGER NOT NULL DEFAULT 0`},
	}
	for _, column := range accountColumns {
		if err := addColumnIfMissing(tx, "accounts", column.name, column.declaration); err != nil {
			return err
		}
	}
	logColumns := []struct{ name, declaration string }{
		{"request_id", `TEXT NOT NULL DEFAULT ''`},
		{"requested_model", `TEXT NOT NULL DEFAULT ''`},
		{"resolved_model", `TEXT NOT NULL DEFAULT ''`},
		{"error_kind", `TEXT NOT NULL DEFAULT ''`},
		{"attempt_count", `INTEGER NOT NULL DEFAULT 1`},
		{"terminal_event", `TEXT NOT NULL DEFAULT ''`},
	}
	for _, column := range logColumns {
		if err := addColumnIfMissing(tx, "request_logs", column.name, column.declaration); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS migration_audit (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		migration_version INTEGER NOT NULL,
		action TEXT NOT NULL,
		primary_account_id INTEGER NOT NULL,
		merged_account_id INTEGER NOT NULL,
		details TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`); err != nil {
		return err
	}
	if err := mergeDuplicateAccounts(tx, cipher); err != nil {
		return err
	}
	if err := populateCredentialFingerprints(tx, cipher); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_accounts_fingerprint ON accounts(credential_fingerprint)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_chatgpt_id_unique ON accounts(chatgpt_account_id) WHERE chatgpt_account_id <> ''`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func migrateV020CompatibilitySettings(tx *sql.Tx, _ *appcrypto.Cipher) error {
	_, err := tx.Exec(`INSERT INTO settings(key, value)
		VALUES('compatibility_profile', COALESCE(
			(SELECT CASE WHEN value='1' THEN 'codex' ELSE 'standard' END FROM settings WHERE key='tls_fingerprint'),
			'standard'
		))
		ON CONFLICT(key) DO NOTHING`)
	return err
}

func migrateV022APIKeyAccounts(tx *sql.Tx, _ *appcrypto.Cipher) error {
	columns := []struct{ name, declaration string }{
		{"account_type", `TEXT NOT NULL DEFAULT 'oauth'`},
		{"base_url", `TEXT NOT NULL DEFAULT ''`},
		{"api_key", `TEXT NOT NULL DEFAULT ''`},
	}
	for _, column := range columns {
		if err := addColumnIfMissing(tx, "accounts", column.name, column.declaration); err != nil {
			return err
		}
	}
	return nil
}

func migrateV023CodexRemoteTargets(tx *sql.Tx, _ *appcrypto.Cipher) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS codex_remote_targets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 22,
		user TEXT NOT NULL,
		password_cipher TEXT NOT NULL DEFAULT '',
		remote_port INTEGER NOT NULL DEFAULT 8080,
		model TEXT NOT NULL DEFAULT '',
		tunnel_enabled INTEGER NOT NULL DEFAULT 1,
		injected INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	return err
}

func migrateV024CodexDirectTargets(tx *sql.Tx, _ *appcrypto.Cipher) error {
	columns := []struct{ name, declaration string }{
		{"mode", `TEXT NOT NULL DEFAULT 'tunnel'`},
		{"base_url", `TEXT NOT NULL DEFAULT ''`},
		{"api_key_cipher", `TEXT NOT NULL DEFAULT ''`},
	}
	for _, column := range columns {
		if err := addColumnIfMissing(tx, "codex_remote_targets", column.name, column.declaration); err != nil {
			return err
		}
	}
	return nil
}

func addColumnIfMissing(tx *sql.Tx, table, column, declaration string) error {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + declaration)
	return err
}

type duplicateAccount struct {
	id                            int64
	email, plan                   string
	accessEnc, refreshEnc, idEnc  string
	access, refresh, idToken      string
	expiresAt, createdAt, updated int64
	proxyID                       sql.NullInt64
}

func mergeDuplicateAccounts(tx *sql.Tx, cipher *appcrypto.Cipher) error {
	rows, err := tx.Query(`SELECT chatgpt_account_id FROM accounts WHERE chatgpt_account_id <> '' GROUP BY chatgpt_account_id HAVING COUNT(*) > 1`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, chatGPTID := range ids {
		accountRows, err := tx.Query(`SELECT id, email, plan_type, access_token, refresh_token, id_token, expires_at, proxy_id, created_at, updated_at
			FROM accounts WHERE chatgpt_account_id=? ORDER BY updated_at DESC, id DESC`, chatGPTID)
		if err != nil {
			return err
		}
		var group []duplicateAccount
		for accountRows.Next() {
			var a duplicateAccount
			if err := accountRows.Scan(&a.id, &a.email, &a.plan, &a.accessEnc, &a.refreshEnc, &a.idEnc, &a.expiresAt, &a.proxyID, &a.createdAt, &a.updated); err != nil {
				_ = accountRows.Close()
				return err
			}
			if a.access, err = cipher.Decrypt(a.accessEnc); err != nil {
				_ = accountRows.Close()
				return err
			}
			if a.refresh, err = cipher.Decrypt(a.refreshEnc); err != nil {
				_ = accountRows.Close()
				return err
			}
			if a.idToken, err = cipher.Decrypt(a.idEnc); err != nil {
				_ = accountRows.Close()
				return err
			}
			group = append(group, a)
		}
		if err := accountRows.Close(); err != nil {
			return err
		}
		if len(group) < 2 {
			continue
		}

		primary := group[0]
		earliest := primary.createdAt
		for _, candidate := range group {
			if candidate.createdAt < earliest {
				earliest = candidate.createdAt
			}
			if primary.email == "" && candidate.email != "" {
				primary.email = candidate.email
			}
			if primary.plan == "" && candidate.plan != "" {
				primary.plan = candidate.plan
			}
			if primary.access == "" && candidate.access != "" {
				primary.access, primary.accessEnc, primary.expiresAt = candidate.access, candidate.accessEnc, candidate.expiresAt
			}
			if primary.refresh == "" && candidate.refresh != "" {
				primary.refresh, primary.refreshEnc = candidate.refresh, candidate.refreshEnc
			}
			if primary.idToken == "" && candidate.idToken != "" {
				primary.idToken, primary.idEnc = candidate.idToken, candidate.idEnc
			}
			if !primary.proxyID.Valid && candidate.proxyID.Valid {
				primary.proxyID = candidate.proxyID
			}
		}
		fingerprint := CredentialFingerprint(primary.access, primary.refresh)
		if _, err := tx.Exec(`UPDATE accounts SET email=?, plan_type=?, access_token=?, refresh_token=?, id_token=?, expires_at=?, proxy_id=?, created_at=?, credential_fingerprint=? WHERE id=?`,
			primary.email, primary.plan, primary.accessEnc, primary.refreshEnc, primary.idEnc, primary.expiresAt, primary.proxyID, earliest, fingerprint, primary.id); err != nil {
			return err
		}
		for _, merged := range group[1:] {
			if _, err := tx.Exec(`UPDATE request_logs SET account_id=? WHERE account_id=?`, primary.id, merged.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO migration_audit(migration_version, action, primary_account_id, merged_account_id, details, created_at) VALUES(2,'merge_duplicate_account',?,?,?,?)`,
				primary.id, merged.id, `{"reason":"duplicate_chatgpt_account_id"}`, time.Now().Unix()); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM accounts WHERE id=?`, merged.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func populateCredentialFingerprints(tx *sql.Tx, cipher *appcrypto.Cipher) error {
	rows, err := tx.Query(`SELECT id, access_token, refresh_token FROM accounts WHERE credential_fingerprint=''`)
	if err != nil {
		return err
	}
	type update struct {
		id          int64
		fingerprint string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var accessEnc, refreshEnc string
		if err := rows.Scan(&id, &accessEnc, &refreshEnc); err != nil {
			_ = rows.Close()
			return err
		}
		access, err := cipher.Decrypt(accessEnc)
		if err != nil {
			_ = rows.Close()
			return err
		}
		refresh, err := cipher.Decrypt(refreshEnc)
		if err != nil {
			_ = rows.Close()
			return err
		}
		updates = append(updates, update{id: id, fingerprint: CredentialFingerprint(access, refresh)})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range updates {
		if _, err := tx.Exec(`UPDATE accounts SET credential_fingerprint=? WHERE id=?`, item.fingerprint, item.id); err != nil {
			return err
		}
	}
	return nil
}

// CredentialFingerprint creates a stable, non-reversible identity for exact
// credential matching. Refresh tokens are preferred because they outlive an
// access token rotation.
func CredentialFingerprint(access, refresh string) string {
	value := access
	prefix := "access:"
	if refresh != "" {
		value = refresh
		prefix = "refresh:"
	}
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(prefix + value))
	return hex.EncodeToString(sum[:])
}

// AccountCredentialFingerprint preserves the existing OAuth fingerprint
// behavior while giving API-key accounts a stable endpoint-scoped identity.
func AccountCredentialFingerprint(accountType AccountType, access, refresh, baseURL, apiKey string) string {
	if accountType != AccountTypeAPIKey {
		return CredentialFingerprint(access, refresh)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	normalizedURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if parsed, err := url.Parse(normalizedURL); err == nil {
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		parsed.Fragment = ""
		normalizedURL = parsed.String()
	}
	sum := sha256.Sum256([]byte("api_key:" + normalizedURL + "\x00" + apiKey))
	return hex.EncodeToString(sum[:])
}

func backupDatabase(db *sql.DB, dbPath string) (string, error) {
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	backup := filepath.Join(filepath.Dir(dbPath), filepath.Base(dbPath)+".pre-v0.2.0-"+stamp+".bak")
	if err := copyFile(dbPath, backup); err != nil {
		return "", err
	}
	return backup, nil
}

func restoreDatabase(dbPath, backup string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale sqlite file: %w", err)
		}
	}
	if err := copyFile(backup, dbPath); err != nil {
		return fmt.Errorf("restore database backup: %w", err)
	}
	return nil
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func quickCheck(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA quick_check`)
	if err != nil {
		return fmt.Errorf("database quick_check: %w", err)
	}
	defer rows.Close()
	var results []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("database quick_check: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("database quick_check: %w", err)
	}
	if len(results) != 1 || results[0] != "ok" {
		sort.Strings(results)
		return fmt.Errorf("database quick_check failed: %v", results)
	}
	return nil
}
