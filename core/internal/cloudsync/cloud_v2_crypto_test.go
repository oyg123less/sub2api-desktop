package cloudsync

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"
)

func TestGuestKeyEnvelopeRoundTripAndAADBinding(t *testing.T) {
	privateKey, publicKey, err := generateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	material, guestKey, err := createGuestKeyMaterial(publicKey, 3)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openGuestKeyEnvelope(privateKey, material.KeyEnvelope, material.EnvelopeContext, material.RecipientKeyVersion)
	if err != nil {
		t.Fatal(err)
	}
	if opened != guestKey {
		t.Fatalf("opened guest key differs from generated key")
	}
	if _, err := openGuestKeyEnvelope(privateKey, material.KeyEnvelope, material.EnvelopeContext+"-tampered", material.RecipientKeyVersion); err == nil {
		t.Fatal("tampered envelope context was accepted")
	}
	if _, err := openGuestKeyEnvelope(privateKey, material.KeyEnvelope, material.EnvelopeContext, material.RecipientKeyVersion+1); err == nil {
		t.Fatal("wrong recipient key version was accepted")
	}
}

func TestRelayChallengeSignatureMatchesCanonicalProof(t *testing.T) {
	privateKey, publicKey, err := generateDeviceKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	deviceID := "rly_018f1f46-7a19-7cc2-88cb-f577e51d3999"
	challenge := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	expiresAt := "2026-07-18T12:00:00.000Z"
	signature, err := signRelayChallenge(privateKey, deviceID, challenge, expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	publicRaw, err := decodeRawURL(publicKey, ed25519.PublicKeySize)
	if err != nil {
		t.Fatal(err)
	}
	signatureRaw, err := decodeRawURL(signature, ed25519.SignatureSize)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("amber-relay-v1|%s|%s|%s", deviceID, challenge, expiresAt)))
	if !ed25519.Verify(ed25519.PublicKey(publicRaw), digest[:], signatureRaw) {
		t.Fatal("relay challenge signature did not verify")
	}
	tampered := sha256.Sum256([]byte(fmt.Sprintf("amber-relay-v1|%s|%s|%s", deviceID, challenge+"x", expiresAt)))
	if ed25519.Verify(ed25519.PublicKey(publicRaw), tampered[:], signatureRaw) {
		t.Fatal("relay signature verified for a tampered challenge")
	}
}
