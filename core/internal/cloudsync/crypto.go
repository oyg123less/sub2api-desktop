package cloudsync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemoryKiB = 64 * 1024
	argonTime      = 3
	argonThreads   = 1
	keySize        = 32
)

var (
	wrapAAD  = []byte("amber-vault-key-v1")
	vaultAAD = []byte("amber-vault-item-v1")
)

type authMaterial struct {
	SaltKDF         []byte
	SaltAuth        []byte
	MasterKey       []byte
	AuthHash        []byte
	VaultKey        []byte
	WrappedVaultKey string
}

func newRegistrationMaterial(password string) (*authMaterial, error) {
	if len(password) < 12 {
		return nil, errors.New("master password must contain at least 12 characters")
	}
	saltKDF, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	saltAuth, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	vaultKey, err := randomBytes(keySize)
	if err != nil {
		return nil, err
	}
	material := deriveAuthMaterial(password, saltKDF, saltAuth)
	material.VaultKey = vaultKey
	material.WrappedVaultKey, err = seal(material.MasterKey, vaultKey, wrapAAD)
	if err != nil {
		material.clear()
		return nil, err
	}
	return material, nil
}

func newRewrapMaterial(password string, vaultKey []byte) (*authMaterial, error) {
	if len(password) < 12 {
		return nil, errors.New("master password must contain at least 12 characters")
	}
	if len(vaultKey) != keySize {
		return nil, errors.New("invalid cloud vault key")
	}
	saltKDF, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	saltAuth, err := randomBytes(16)
	if err != nil {
		return nil, err
	}
	material := deriveAuthMaterial(password, saltKDF, saltAuth)
	material.VaultKey = append([]byte(nil), vaultKey...)
	material.WrappedVaultKey, err = wrapVaultKey(material.MasterKey, vaultKey)
	if err != nil {
		material.clear()
		return nil, err
	}
	return material, nil
}

func deriveAuthMaterial(password string, saltKDF, saltAuth []byte) *authMaterial {
	master := argon2.IDKey([]byte(password), saltKDF, argonTime, argonMemoryKiB, argonThreads, keySize)
	authHash := argon2.IDKey(master, saltAuth, argonTime, argonMemoryKiB, argonThreads, keySize)
	return &authMaterial{SaltKDF: append([]byte(nil), saltKDF...), SaltAuth: append([]byte(nil), saltAuth...), MasterKey: master, AuthHash: authHash}
}

func (m *authMaterial) clear() {
	wipe(m.MasterKey)
	wipe(m.AuthHash)
	wipe(m.VaultKey)
}

func wrapVaultKey(masterKey, vaultKey []byte) (string, error) {
	return seal(masterKey, vaultKey, wrapAAD)
}

func unwrapVaultKey(masterKey []byte, wrapped string) ([]byte, error) {
	plain, err := open(masterKey, wrapped, wrapAAD)
	if err != nil {
		return nil, errors.New("master password could not unlock the cloud vault")
	}
	if len(plain) != keySize {
		wipe(plain)
		return nil, errors.New("invalid cloud vault key")
	}
	return plain, nil
}

func encryptVaultItem(vaultKey, plaintext []byte) (string, error) {
	return seal(vaultKey, plaintext, vaultAAD)
}

func decryptVaultItem(vaultKey []byte, ciphertext string) ([]byte, error) {
	plain, err := open(vaultKey, ciphertext, vaultAAD)
	if err != nil {
		return nil, errors.New("cloud vault item authentication failed")
	}
	return plain, nil
}

func seal(key, plaintext, aad []byte) (string, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return "", err
	}
	nonce, err := randomBytes(aead.NonceSize())
	if err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, plaintext, aad)
	return "v1." + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func open(key []byte, value string, aad []byte) ([]byte, error) {
	if !strings.HasPrefix(value, "v1.") {
		return nil, errors.New("unsupported ciphertext version")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "v1."))
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	if len(raw) < aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("ciphertext too short")
	}
	return aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], aad)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("invalid key length %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return nil, err
	}
	return value, nil
}

func encodeBytes(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeBytes(value string, size int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(decoded) != size {
		return nil, fmt.Errorf("invalid decoded size %d", len(decoded))
	}
	return decoded, nil
}

func wipe(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
