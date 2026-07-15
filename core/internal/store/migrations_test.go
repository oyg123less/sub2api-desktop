package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	appcrypto "sub2api-desktop/core/internal/crypto"
)

func TestOpenInitializesVersionedSchema(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "sub2api.db")
	st, err := Open(dbPath, cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if got := st.SchemaVersion(); got != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", got, CurrentSchemaVersion)
	}
	if got := st.MigrationBackup(); got != "" {
		t.Fatalf("new database unexpectedly created backup %q", got)
	}
	for _, column := range []string{"credential_fingerprint", "last_success_at", "consecutive_failures", "next_retry_at"} {
		if !testColumnExists(t, st.db, "accounts", column) {
			t.Fatalf("accounts.%s missing", column)
		}
	}
	for _, column := range []string{"request_id", "requested_model", "resolved_model", "error_kind", "attempt_count", "terminal_event"} {
		if !testColumnExists(t, st.db, "request_logs", column) {
			t.Fatalf("request_logs.%s missing", column)
		}
	}
}

func TestLegacyUpgradeBacksUpAndMergesDuplicates(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "sub2api.db")
	createLegacyDatabase(t, dbPath, cipher)

	st, err := Open(dbPath, cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if st.MigrationBackup() == "" {
		t.Fatal("legacy upgrade did not create a backup")
	}
	if _, err := os.Stat(st.MigrationBackup()); err != nil {
		t.Fatalf("upgrade backup missing: %v", err)
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("account count = %d, want 1", len(accounts))
	}
	account := accounts[0]
	if account.Email != "new@example.com" || account.AccessToken != "new-access" || account.RefreshToken != "old-refresh" {
		t.Fatalf("unexpected merged account: email=%q access=%q refresh=%q", account.Email, account.AccessToken, account.RefreshToken)
	}
	if got := account.CreatedAt.Unix(); got != 100 {
		t.Fatalf("created_at = %d, want earliest value 100", got)
	}
	if account.CredentialFingerprint != CredentialFingerprint("new-access", "old-refresh") {
		t.Fatal("credential fingerprint was not populated from merged credentials")
	}
	settings, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.CompatProfile != "codex" {
		t.Fatalf("compatibility profile = %q, want codex", settings.CompatProfile)
	}

	var logAccountID int64
	if err := st.db.QueryRow(`SELECT account_id FROM request_logs LIMIT 1`).Scan(&logAccountID); err != nil {
		t.Fatal(err)
	}
	if logAccountID != account.ID {
		t.Fatalf("request log account_id = %d, want %d", logAccountID, account.ID)
	}
	var auditCount int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM migration_audit WHERE action='merge_duplicate_account'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("migration audit count = %d, want 1", auditCount)
	}

	_, err = st.db.Exec(`INSERT INTO accounts(email, chatgpt_account_id, created_at, updated_at) VALUES('duplicate','acct-shared',1,1)`)
	if err == nil {
		t.Fatal("unique chatgpt_account_id index did not reject duplicate")
	}
}

func TestMigrationFailureRestoresLegacyDatabase(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "sub2api.db")
	createLegacyDatabase(t, dbPath, cipher)

	original := migrations[1].apply
	migrations[1].apply = func(*sql.Tx, *appcrypto.Cipher) error { return errors.New("injected migration failure") }
	t.Cleanup(func() { migrations[1].apply = original })

	if _, err := Open(dbPath, cipher); err == nil {
		t.Fatal("Open succeeded despite injected migration failure")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("restored account count = %d, want 2", count)
	}
	var migrationTable int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&migrationTable); err != nil {
		t.Fatal(err)
	}
	if migrationTable != 0 {
		t.Fatal("failed migration was not rolled back to the legacy database")
	}
}

func TestV022MigrationDefaultsLegacyAccountsAndEncryptsAPIKeys(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "sub2api.db")
	createLegacyDatabase(t, dbPath, cipher)

	st, err := Open(dbPath, cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, column := range []string{"account_type", "base_url", "api_key"} {
		if !testColumnExists(t, st.db, "accounts", column) {
			t.Fatalf("accounts.%s missing after v0.2.2 migration", column)
		}
	}
	legacyAccounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(legacyAccounts) != 1 || legacyAccounts[0].AccountType != AccountTypeOAuth {
		t.Fatalf("legacy account type = %#v, want oauth", legacyAccounts)
	}

	const apiKey = "sk-secret-api-key-value"
	created, err := st.CreateAccount(&Account{
		AccountType: AccountTypeAPIKey,
		BaseURL:     "https://api.example.com/v1/responses",
		APIKey:      apiKey,
		Email:       "Example API",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.APIKey != apiKey || created.AccountType != AccountTypeAPIKey {
		t.Fatal("API-key account was not decrypted correctly")
	}
	var storedAPIKey string
	if err := st.db.QueryRow(`SELECT api_key FROM accounts WHERE id=?`, created.ID).Scan(&storedAPIKey); err != nil {
		t.Fatal(err)
	}
	if storedAPIKey == "" || storedAPIKey == apiKey {
		t.Fatal("api_key was not encrypted at rest")
	}
	var migrationName string
	if err := st.db.QueryRow(`SELECT name FROM schema_migrations WHERE version=4`).Scan(&migrationName); err != nil {
		t.Fatal(err)
	}
	if migrationName != "v0.2.2 api-key accounts" {
		t.Fatalf("migration name = %q", migrationName)
	}
}

func TestV023MigrationStoresEncryptedCodexRemoteTargets(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, column := range []string{"id", "host", "port", "user", "password_cipher", "remote_port", "model", "tunnel_enabled", "injected"} {
		if !testColumnExists(t, st.db, "codex_remote_targets", column) {
			t.Fatalf("codex_remote_targets.%s missing", column)
		}
	}
	created, err := st.CreateCodexRemoteTarget(&CodexRemoteTarget{
		Name: "server", Host: "example.test", Port: 22, User: "deploy", Password: "test-password",
		RemotePort: 8080, Model: "gpt-5.6-sol", TunnelEnabled: true, Injected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var passwordCipher string
	if err := st.db.QueryRow(`SELECT password_cipher FROM codex_remote_targets WHERE id=?`, created.ID).Scan(&passwordCipher); err != nil {
		t.Fatal(err)
	}
	if passwordCipher == "" || passwordCipher == "test-password" {
		t.Fatal("remote target password was not encrypted")
	}
	loaded, err := st.GetCodexRemoteTarget(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Password != "test-password" || !loaded.TunnelEnabled || !loaded.Injected {
		t.Fatal("remote target did not round-trip through encrypted storage")
	}
	if _, err := st.db.Exec(`DROP TABLE codex_remote_targets`); err != nil {
		t.Fatal(err)
	}
	if err := migrateV023CodexRemoteTargetsInTest(st); err != nil {
		t.Fatal(err)
	}
	if !testColumnExists(t, st.db, "codex_remote_targets", "password_cipher") {
		t.Fatal("v0.2.3 migration was not idempotent")
	}
}

func TestV024MigrationDefaultsTunnelAndEncryptsDirectAPIKey(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, column := range []string{"mode", "base_url", "api_key_cipher"} {
		if !testColumnExists(t, st.db, "codex_remote_targets", column) {
			t.Fatalf("codex_remote_targets.%s missing after v0.2.4 migration", column)
		}
	}
	const apiKey = "fixture-direct-api-key"
	created, err := st.CreateCodexRemoteTarget(&CodexRemoteTarget{
		Name: "direct", Host: "example.test", Port: 22, User: "deploy", Password: "test-password",
		RemotePort: 8080, Model: "gpt-5.6-sol", Mode: "direct", BaseURL: "https://api.example.test/v1",
		APIKey: apiKey, Injected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var apiKeyCipher string
	if err := st.db.QueryRow(`SELECT api_key_cipher FROM codex_remote_targets WHERE id=?`, created.ID).Scan(&apiKeyCipher); err != nil {
		t.Fatal(err)
	}
	if apiKeyCipher == "" || apiKeyCipher == apiKey {
		t.Fatal("direct API key was not encrypted at rest")
	}
	decrypted, err := cipher.Decrypt(apiKeyCipher)
	if err != nil || decrypted != apiKey {
		t.Fatal("direct API key ciphertext did not decrypt correctly")
	}
	loaded, err := st.GetCodexRemoteTarget(created.ID)
	if err != nil || loaded.APIKey != apiKey || loaded.Mode != "direct" {
		t.Fatalf("direct target did not round-trip through encrypted storage: %#v, err=%v", loaded, err)
	}
	loaded.APIKey = ""
	loaded.Injected = false
	if _, err := st.UpdateCodexRemoteTarget(loaded); err != nil {
		t.Fatal(err)
	}
	var preservedCipher string
	if err := st.db.QueryRow(`SELECT api_key_cipher FROM codex_remote_targets WHERE id=?`, created.ID).Scan(&preservedCipher); err != nil {
		t.Fatal(err)
	}
	if preservedCipher != apiKeyCipher {
		t.Fatal("updating direct target metadata replaced its API-key ciphertext")
	}

	if _, err := st.db.Exec(`DROP TABLE codex_remote_targets`); err != nil {
		t.Fatal(err)
	}
	if err := migrateV023CodexRemoteTargetsInTest(st); err != nil {
		t.Fatal(err)
	}
	passwordCipher, err := cipher.Encrypt("legacy-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`INSERT INTO codex_remote_targets
		(name,host,port,user,password_cipher,remote_port,model,tunnel_enabled,injected,created_at,updated_at)
		VALUES('legacy','legacy.test',22,'deploy',?,8080,'gpt-5.6-sol',1,1,1,1)`, passwordCipher); err != nil {
		t.Fatal(err)
	}
	if err := migrateV024CodexDirectTargetsInTest(st); err != nil {
		t.Fatal(err)
	}
	if err := migrateV024CodexDirectTargetsInTest(st); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := st.db.QueryRow(`SELECT mode FROM codex_remote_targets WHERE name='legacy'`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "tunnel" {
		t.Fatalf("legacy target mode = %q, want tunnel", mode)
	}
}

func migrateV023CodexRemoteTargetsInTest(st *Store) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	if err := migrateV023CodexRemoteTargets(tx, st.cipher); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func migrateV024CodexDirectTargetsInTest(st *Store) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	if err := migrateV024CodexDirectTargets(tx, st.cipher); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func createLegacyDatabase(t *testing.T, dbPath string, cipher *appcrypto.Cipher) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL DEFAULT '', chatgpt_account_id TEXT NOT NULL DEFAULT '',
			plan_type TEXT NOT NULL DEFAULT '', access_token TEXT NOT NULL DEFAULT '', refresh_token TEXT NOT NULL DEFAULT '',
			id_token TEXT NOT NULL DEFAULT '', expires_at INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL DEFAULT 'active',
			status_reason TEXT NOT NULL DEFAULT '', rate_limited_until INTEGER NOT NULL DEFAULT 0, proxy_id INTEGER,
			last_used_at INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
			usage_snapshot TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE request_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER, account_email TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
			status_code INTEGER NOT NULL DEFAULT 0, prompt_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0, latency_ms INTEGER NOT NULL DEFAULT 0, stream INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL)`,
		`CREATE TABLE proxies (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT 'http', host TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL)`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	oldAccess, _ := cipher.Encrypt("old-access")
	oldRefresh, _ := cipher.Encrypt("old-refresh")
	newAccess, _ := cipher.Encrypt("new-access")
	result, err := db.Exec(`INSERT INTO accounts(email,chatgpt_account_id,access_token,refresh_token,created_at,updated_at) VALUES(?,?,?,?,100,200)`,
		"old@example.com", "acct-shared", oldAccess, oldRefresh)
	if err != nil {
		t.Fatal(err)
	}
	oldID, _ := result.LastInsertId()
	if _, err := db.Exec(`INSERT INTO accounts(email,chatgpt_account_id,access_token,refresh_token,created_at,updated_at) VALUES(?,?,?,?,150,300)`,
		"new@example.com", "acct-shared", newAccess, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO request_logs(account_id,created_at) VALUES(?,250)`, oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES('tls_fingerprint','1')`); err != nil {
		t.Fatal(err)
	}
}

func testColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	return false
}
