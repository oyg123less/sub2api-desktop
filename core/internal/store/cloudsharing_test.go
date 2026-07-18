package store

import (
	"strings"
	"testing"
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

	var identityCipher, deviceCipher, guestCipher string
	if err := st.db.QueryRow(`SELECT x25519_private_cipher,device_private_cipher FROM cloud_identities WHERE user_id=?`, identity.UserID).
		Scan(&identityCipher, &deviceCipher); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT guest_key_cipher FROM cloud_received_keys WHERE user_id=? AND grant_public_id=?`,
		received.UserID, received.GrantPublicID).Scan(&guestCipher); err != nil {
		t.Fatal(err)
	}
	for cipher, plaintext := range map[string]string{
		identityCipher: identity.X25519PrivateKey,
		deviceCipher:   identity.DevicePrivateKey,
		guestCipher:    received.GuestKey,
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
}
