package store

import (
	"strings"
	"testing"
	"time"
)

func TestCloudSharingSecretsAreEncryptedAtRest(t *testing.T) {
	st := openCloudTestStore(t)
	identity := CloudIdentity{
		UserID:           42,
		X25519PublicKey:  "x25519-public",
		X25519PrivateKey: "x25519-private-plaintext",
		DevicePublicKey:  "ed25519-public",
		DevicePrivateKey: "ed25519-private-plaintext",
		DeviceName:       "Test device",
		RelayEnabled:     true,
	}
	if err := st.SaveCloudIdentity(identity); err != nil {
		t.Fatal(err)
	}
	received := CloudReceivedKey{
		UserID:        42,
		GrantPublicID: "sgr_test",
		KeyVersion:    2,
		KeyPrefix:     "sk-amber-prefix",
		BaseURL:       "https://cloud.example.test/v1",
		GuestKey:      "sk-amber-received-secret-plaintext",
	}
	if err := st.SaveCloudReceivedKey(received); err != nil {
		t.Fatal(err)
	}
	hostState := CloudConnectHostState{
		UserID: 42, ConnectionCode: "572814639", PasswordVersion: 3,
		Password: "AB3D5F", ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if err := st.SaveCloudConnectHostState(hostState); err != nil {
		t.Fatal(err)
	}
	claimAttempt := CloudConnectClaimAttempt{
		UserID: 42, IdempotencyKey: "claim-attempt-0001", ConnectionCode: "572814639",
		Password: "AB3D5F", KeyMaterialJSON: `{"key_prefix":"sk-amber-test"}`,
		GuestKey: "sk-amber-pending-claim-secret",
	}
	if err := st.SaveCloudConnectClaimAttempt(claimAttempt); err != nil {
		t.Fatal(err)
	}

	var identityCipher, deviceCipher, guestCipher, passwordCipher, attemptPasswordCipher, attemptGuestCipher string
	if err := st.db.QueryRow(`SELECT x25519_private_cipher,device_private_cipher FROM cloud_identities WHERE user_id=?`, identity.UserID).
		Scan(&identityCipher, &deviceCipher); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT guest_key_cipher FROM cloud_received_keys WHERE user_id=? AND grant_public_id=?`,
		received.UserID, received.GrantPublicID).Scan(&guestCipher); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT password_cipher FROM cloud_connect_host_state WHERE user_id=?`, hostState.UserID).Scan(&passwordCipher); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT password_cipher,guest_key_cipher FROM cloud_connect_claim_attempts
		WHERE user_id=? AND idempotency_key=?`, claimAttempt.UserID, claimAttempt.IdempotencyKey).Scan(&attemptPasswordCipher, &attemptGuestCipher); err != nil {
		t.Fatal(err)
	}
	for cipher, plaintext := range map[string]string{
		identityCipher:        identity.X25519PrivateKey,
		deviceCipher:          identity.DevicePrivateKey,
		guestCipher:           received.GuestKey,
		passwordCipher:        hostState.Password,
		attemptPasswordCipher: claimAttempt.Password,
		attemptGuestCipher:    claimAttempt.GuestKey,
	} {
		if cipher == "" || strings.Contains(cipher, plaintext) {
			t.Fatalf("secret was stored without encryption")
		}
	}

	loadedIdentity, err := st.LoadCloudIdentity(identity.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedIdentity.X25519PrivateKey != identity.X25519PrivateKey || loadedIdentity.DevicePrivateKey != identity.DevicePrivateKey || !loadedIdentity.RelayEnabled {
		t.Fatalf("loaded cloud identity differs: %#v", loadedIdentity)
	}
	loadedKey, err := st.LoadCloudReceivedKey(received.UserID, received.GrantPublicID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedKey.GuestKey != received.GuestKey || loadedKey.KeyVersion != received.KeyVersion {
		t.Fatalf("loaded received key differs: %#v", loadedKey)
	}
	loadedState, err := st.LoadCloudConnectHostState(hostState.UserID)
	if err != nil || loadedState.Password != hostState.Password || loadedState.ConnectionCode != hostState.ConnectionCode {
		t.Fatalf("loaded connection state differs: %#v, %v", loadedState, err)
	}
	loadedAttempt, err := st.LoadCloudConnectClaimAttempt(claimAttempt.UserID, claimAttempt.IdempotencyKey)
	if err != nil || loadedAttempt.GuestKey != claimAttempt.GuestKey || loadedAttempt.Password != claimAttempt.Password {
		t.Fatalf("loaded claim attempt differs: %#v, %v", loadedAttempt, err)
	}
}

func TestCloudReceivedLinkParticipatesInGatewaySchedulingWithoutBecomingALocalAccount(t *testing.T) {
	st := openCloudTestStore(t)
	if err := st.SaveCloudSession(CloudSession{
		UserID: 7, Email: "recipient@example.test", Role: "user", SaltKDF: "kdf", SaltAuth: "auth",
		WrappedVaultKey: "wrapped", VaultKey: "vault-key", RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	key := CloudReceivedKey{
		UserID: 7, GrantPublicID: "sgr_connect_test", KeyVersion: 1, KeyPrefix: "sk-amber-connect",
		BaseURL: "https://cloud.example.test/v1", GuestKey: "sk-amber-connect-secret",
	}
	if err := st.SaveCloudReceivedKey(key); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedAccountLink(CloudReceivedAccountLink{
		UserID: 7, GrantPublicID: key.GrantPublicID, OwnerName: "Owner", GroupName: "Quick share",
		RemoteStatus: "active", Enabled: true, RPMLimit: 30, ConcurrencyLimit: 4,
	}); err != nil {
		t.Fatal(err)
	}
	scheduled, err := st.ListActiveCloudReceivedAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduled) != 1 || scheduled[0].ID >= 0 || scheduled[0].APIKey != key.GuestKey ||
		scheduled[0].BaseURL != key.BaseURL || scheduled[0].MaxConcurrency != 4 || scheduled[0].PlanType != "cloud_share" {
		t.Fatalf("unexpected scheduled cloud share: %#v", scheduled)
	}
	local, err := st.ListAccounts()
	if err != nil || len(local) != 0 {
		t.Fatalf("received share leaked into local account list: %#v, %v", local, err)
	}
	disabled := false
	if err := st.SetCloudReceivedAccountLink(7, key.GrantPublicID, &disabled, nil, false); err != nil {
		t.Fatal(err)
	}
	scheduled, err = st.ListActiveCloudReceivedAccounts()
	if err != nil || len(scheduled) != 0 {
		t.Fatalf("disabled share remained schedulable: %#v, %v", scheduled, err)
	}
}

func TestCloudReceivedAccountAlwaysUsesCurrentKeyAndPreservesManagedState(t *testing.T) {
	st := openCloudTestStore(t)
	if err := st.SaveCloudSession(CloudSession{
		UserID: 8, Email: "recipient@example.test", Role: "user", SaltKDF: "kdf", SaltAuth: "auth",
		WrappedVaultKey: "wrapped", VaultKey: "vault-key", RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	grantID := "sgr_rotated"
	if err := st.SaveCloudReceivedKey(CloudReceivedKey{
		UserID: 8, GrantPublicID: grantID, KeyVersion: 1, KeyPrefix: "sk-amber-old",
		BaseURL: "https://old.example.test/v1", GuestKey: "sk-amber-old-secret",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedAccountLink(CloudReceivedAccountLink{
		UserID: 8, GrantPublicID: grantID, OwnerName: "Owner", GroupName: "Shared workspace",
		RemoteStatus: "active", Enabled: true, RPMLimit: 30, ConcurrencyLimit: 3,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedKey(CloudReceivedKey{
		UserID: 8, GrantPublicID: grantID, KeyVersion: 2, KeyPrefix: "sk-amber-new",
		BaseURL: "https://new.example.test/v1", GuestKey: "sk-amber-new-secret",
	}); err != nil {
		t.Fatal(err)
	}

	accounts, err := st.ListCloudReceivedAccounts()
	if err != nil || len(accounts) != 1 {
		t.Fatalf("managed accounts = %#v, err = %v", accounts, err)
	}
	managed := accounts[0]
	if managed.ID >= 0 || managed.Source != "cloud_share" || managed.CloudGrantID != grantID ||
		managed.CloudOwnerName != "Owner" || managed.CloudGroupName != "Shared workspace" ||
		managed.APIKey != "sk-amber-new-secret" || managed.BaseURL != "https://new.example.test/v1" ||
		managed.Status != AccountActive || !managed.CloudLocalEnabled || managed.MaxConcurrency != 3 {
		t.Fatalf("managed account did not use current share state: %#v", managed)
	}

	enabled := false
	if err := st.SetCloudReceivedAccountLink(8, grantID, &enabled, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateCloudReceivedAccountHealth(8, grantID, true, "healthy"); err != nil {
		t.Fatal(err)
	}
	disabled, err := st.GetCloudReceivedAccount(managed.ID)
	if err != nil || disabled.Status != AccountDisabled || disabled.CloudLocalEnabled || disabled.StatusReason != "cloud_share_disabled" {
		t.Fatalf("health probe changed disabled routing: %#v, err = %v", disabled, err)
	}

	enabled = true
	if err := st.SetCloudReceivedAccountLink(8, grantID, &enabled, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedAccountLink(CloudReceivedAccountLink{
		UserID: 8, GrantPublicID: grantID, OwnerName: "Owner", GroupName: "Shared workspace",
		RemoteStatus: "paused", Enabled: true, RPMLimit: 30, ConcurrencyLimit: 3,
	}); err != nil {
		t.Fatal(err)
	}
	paused, err := st.GetCloudReceivedAccount(managed.ID)
	if err != nil || paused.Status != AccountDisabled || paused.StatusReason != "cloud_share_paused" {
		t.Fatalf("paused share state = %#v, err = %v", paused, err)
	}
	active, err := st.ListActiveCloudReceivedAccounts()
	if err != nil || len(active) != 0 {
		t.Fatalf("paused share remained schedulable: %#v, err = %v", active, err)
	}
}
