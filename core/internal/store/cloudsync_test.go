package store

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	appcrypto "sub2api-desktop/core/internal/crypto"
)

func openCloudTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(dir, "data.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCloudMigrationTracksDirtyRowsAndTombstones(t *testing.T) {
	st := openCloudTestStore(t)
	account, err := st.CreateAccount(&Account{
		AccountType: AccountTypeOAuth, Email: "sync@example.test", AccessToken: "access", RefreshToken: "refresh", Status: AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuid.MatchString(account.ClientUID) || !account.SyncDirty || account.SyncVersion != 0 {
		t.Fatalf("unexpected sync metadata: uid=%q dirty=%v version=%d", account.ClientUID, account.SyncDirty, account.SyncVersion)
	}
	if err := st.MarkCloudItemSynced(CloudKindAccount, account.ClientUID, 3); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateTokens(account.ID, "new-access", "new-refresh", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.SyncDirty || updated.SyncVersion != 3 {
		t.Fatalf("credential update did not mark dirty: %#v", updated)
	}
	if err := st.DeleteAccount(account.ID); err != nil {
		t.Fatal(err)
	}
	tombstones, err := st.CloudTombstones()
	if err != nil || len(tombstones) != 1 {
		t.Fatalf("tombstones=%#v err=%v", tombstones, err)
	}
	if tombstones[0].ClientUID != account.ClientUID || tombstones[0].SyncVersion != 3 {
		t.Fatalf("unexpected tombstone: %#v", tombstones[0])
	}
}

func TestCloudSessionSecretsUseInstallationCipher(t *testing.T) {
	st := openCloudTestStore(t)
	session := CloudSession{
		UserID: 7, Email: "cloud@example.test", Role: "user", SaltKDF: "salt-kdf", SaltAuth: "salt-auth",
		WrappedVaultKey: "v1.wrapped", VaultKey: "vault-key-plaintext", RefreshToken: "refresh-token-plaintext",
	}
	if err := st.SaveCloudSession(session); err != nil {
		t.Fatal(err)
	}
	var vaultCipher, refreshCipher string
	if err := st.db.QueryRow(`SELECT vault_key_cipher,refresh_token_cipher FROM cloud_session WHERE id=1`).Scan(&vaultCipher, &refreshCipher); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(vaultCipher, session.VaultKey) || strings.Contains(refreshCipher, session.RefreshToken) {
		t.Fatal("cloud session stored plaintext secrets")
	}
	loaded, err := st.LoadCloudSession()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VaultKey != session.VaultKey || loaded.RefreshToken != session.RefreshToken {
		t.Fatalf("loaded session differs: %#v", loaded)
	}
}

func TestSettingsBecomeDirtyOutsideRemoteApply(t *testing.T) {
	st := openCloudTestStore(t)
	state, err := st.CloudSettingsState()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkCloudItemSynced(CloudKindSettings, state.ClientUID, 2); err != nil {
		t.Fatal(err)
	}
	settings, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Language = "en-US"
	if err := st.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	state, err = st.CloudSettingsState()
	if err != nil || !state.SyncDirty || state.SyncVersion != 2 {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}
