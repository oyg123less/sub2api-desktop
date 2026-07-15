package cloudsync

import (
	"bytes"
	"strings"
	"testing"
)

func TestVaultCryptoUsesIndependentAuthenticatedKeys(t *testing.T) {
	material, err := newRegistrationMaterial("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	defer material.clear()
	if len(material.MasterKey) != keySize || len(material.AuthHash) != keySize || len(material.VaultKey) != keySize {
		t.Fatalf("unexpected key sizes: master=%d auth=%d vault=%d", len(material.MasterKey), len(material.AuthHash), len(material.VaultKey))
	}
	if bytes.Equal(material.MasterKey, material.VaultKey) || bytes.Equal(material.AuthHash, material.VaultKey) {
		t.Fatal("vault key must be independent from password-derived keys")
	}
	unwrapped, err := unwrapVaultKey(material.MasterKey, material.WrappedVaultKey)
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(unwrapped)
	if !bytes.Equal(unwrapped, material.VaultKey) {
		t.Fatal("unwrapped vault key differs")
	}
	plaintext := []byte(`{"access_token":"plaintext-secret"}`)
	ciphertext, err := encryptVaultItem(material.VaultKey, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "plaintext-secret") {
		t.Fatal("ciphertext exposed plaintext")
	}
	decrypted, err := decryptVaultItem(material.VaultKey, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted = %q", decrypted)
	}
	wrong := bytes.Repeat([]byte{0x44}, keySize)
	if _, err := decryptVaultItem(wrong, ciphertext); err == nil {
		t.Fatal("wrong vault key decrypted ciphertext")
	}
}

func TestArgonParametersAndPasswordChangeKeepVaultKey(t *testing.T) {
	initial, err := newRegistrationMaterial("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	defer initial.clear()
	next, err := newRewrapMaterial("a different strong master password", initial.VaultKey)
	if err != nil {
		t.Fatal(err)
	}
	defer next.clear()
	if bytes.Equal(initial.MasterKey, next.MasterKey) || bytes.Equal(initial.AuthHash, next.AuthHash) {
		t.Fatal("password change must derive fresh authentication material")
	}
	unwrapped, err := unwrapVaultKey(next.MasterKey, next.WrappedVaultKey)
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(unwrapped)
	if !bytes.Equal(unwrapped, initial.VaultKey) {
		t.Fatal("password change replaced the vault key")
	}
}
