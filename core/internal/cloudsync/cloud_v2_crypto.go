package cloudsync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const keyEnvelopeAlgorithm = "X25519-HKDF-SHA256-AES-256-GCM"

type keyEnvelope struct {
	Version            int    `json:"version"`
	Algorithm          string `json:"algorithm"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	Salt               string `json:"salt"`
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
}

type cloudKeyMaterial struct {
	KeyPrefix           string `json:"key_prefix"`
	GuestKeyHash        string `json:"guest_key_hash"`
	KeyEnvelope         string `json:"key_envelope"`
	EnvelopeContext     string `json:"envelope_context"`
	RecipientKeyVersion int    `json:"recipient_key_version"`
}

func rawURL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeRawURL(value string, size int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || (size > 0 && len(decoded) != size) {
		return nil, errors.New("invalid base64url key material")
	}
	return decoded, nil
}

func generateIdentityKeyPair() (privateKey, publicKey string, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return rawURL(key.Bytes()), rawURL(key.PublicKey().Bytes()), nil
}

func identityPublicKey(privateKey string) (string, error) {
	raw, err := decodeRawURL(privateKey, 32)
	if err != nil {
		return "", err
	}
	key, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", err
	}
	return rawURL(key.PublicKey().Bytes()), nil
}

func generateDeviceKeyPair() (privateKey, publicKey string, err error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return rawURL(private), rawURL(public), nil
}

func randomURLBytes(size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return rawURL(value), nil
}

func deriveEnvelopeAEAD(shared, salt []byte) (cipher.AEAD, error) {
	reader := hkdf.New(sha256.New, shared, salt, []byte("amber-share-key-envelope-v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, err
	}
	defer wipe(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func envelopeAAD(context string, recipientKeyVersion int) []byte {
	return []byte(fmt.Sprintf("amber-share-key-v1|%s|%d", context, recipientKeyVersion))
}

func createGuestKeyMaterial(recipientPublicKey string, recipientKeyVersion int) (cloudKeyMaterial, string, error) {
	publicRaw, err := decodeRawURL(recipientPublicKey, 32)
	if err != nil || recipientKeyVersion < 1 {
		return cloudKeyMaterial{}, "", errors.New("invalid recipient encryption key")
	}
	recipient, err := ecdh.X25519().NewPublicKey(publicRaw)
	if err != nil {
		return cloudKeyMaterial{}, "", err
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return cloudKeyMaterial{}, "", err
	}
	shared, err := ephemeral.ECDH(recipient)
	if err != nil {
		return cloudKeyMaterial{}, "", err
	}
	defer wipe(shared)
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return cloudKeyMaterial{}, "", err
	}
	aead, err := deriveEnvelopeAEAD(shared, salt)
	if err != nil {
		return cloudKeyMaterial{}, "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return cloudKeyMaterial{}, "", err
	}
	guestRandom, err := randomURLBytes(32)
	if err != nil {
		return cloudKeyMaterial{}, "", err
	}
	guestKey := "sk-amber-" + guestRandom
	contextRandom, err := randomURLBytes(24)
	if err != nil {
		return cloudKeyMaterial{}, "", err
	}
	context := "ctx_" + contextRandom
	ciphertext := aead.Seal(nil, nonce, []byte(guestKey), envelopeAAD(context, recipientKeyVersion))
	envelopeJSON, err := json.Marshal(keyEnvelope{
		Version: 1, Algorithm: keyEnvelopeAlgorithm, EphemeralPublicKey: rawURL(ephemeral.PublicKey().Bytes()),
		Salt: rawURL(salt), Nonce: rawURL(nonce), Ciphertext: rawURL(ciphertext),
	})
	if err != nil {
		return cloudKeyMaterial{}, "", err
	}
	hash := sha256.Sum256([]byte(guestKey))
	prefix := guestKey
	if len(prefix) > 18 {
		prefix = prefix[:18]
	}
	return cloudKeyMaterial{
		KeyPrefix: prefix, GuestKeyHash: rawURL(hash[:]), KeyEnvelope: string(envelopeJSON),
		EnvelopeContext: context, RecipientKeyVersion: recipientKeyVersion,
	}, guestKey, nil
}

func openGuestKeyEnvelope(identityPrivateKey, envelopeJSON, context string, recipientKeyVersion int) (string, error) {
	privateRaw, err := decodeRawURL(identityPrivateKey, 32)
	if err != nil {
		return "", err
	}
	private, err := ecdh.X25519().NewPrivateKey(privateRaw)
	if err != nil {
		return "", err
	}
	var envelope keyEnvelope
	if err := json.Unmarshal([]byte(envelopeJSON), &envelope); err != nil || envelope.Version != 1 || envelope.Algorithm != keyEnvelopeAlgorithm {
		return "", errors.New("invalid access key envelope")
	}
	ephRaw, err := decodeRawURL(envelope.EphemeralPublicKey, 32)
	if err != nil {
		return "", err
	}
	ephemeral, err := ecdh.X25519().NewPublicKey(ephRaw)
	if err != nil {
		return "", err
	}
	shared, err := private.ECDH(ephemeral)
	if err != nil {
		return "", err
	}
	defer wipe(shared)
	salt, err := decodeRawURL(envelope.Salt, 16)
	if err != nil {
		return "", err
	}
	nonce, err := decodeRawURL(envelope.Nonce, 12)
	if err != nil {
		return "", err
	}
	ciphertext, err := decodeRawURL(envelope.Ciphertext, 0)
	if err != nil {
		return "", err
	}
	aead, err := deriveEnvelopeAEAD(shared, salt)
	if err != nil {
		return "", err
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, envelopeAAD(context, recipientKeyVersion))
	if err != nil {
		return "", errors.New("access key envelope authentication failed")
	}
	defer wipe(plaintext)
	guestKey := string(plaintext)
	if len(guestKey) < 40 || len(guestKey) > 128 || guestKey[:9] != "sk-amber-" {
		return "", errors.New("access key envelope contains an invalid key")
	}
	return guestKey, nil
}

func signRelayChallenge(devicePrivateKey, deviceID, challenge, expiresAt string) (string, error) {
	privateRaw, err := decodeRawURL(devicePrivateKey, ed25519.PrivateKeySize)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("amber-relay-v1|%s|%s|%s", deviceID, challenge, expiresAt)))
	return rawURL(ed25519.Sign(ed25519.PrivateKey(privateRaw), digest[:])), nil
}
