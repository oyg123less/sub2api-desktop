package store

import (
	"database/sql"
	"errors"
	"time"
)

// CloudIdentity stores per-cloud-user envelope and device keys. Private key
// fields are encrypted by the installation cipher before they reach SQLite.
type CloudIdentity struct {
	UserID           int64
	X25519PublicKey  string
	X25519PrivateKey string
	DevicePublicKey  string
	DevicePrivateKey string
	DevicePublicID   string
	DeviceName       string
	RelayEnabled     bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CloudReceivedKey struct {
	UserID        int64
	GrantPublicID string
	KeyVersion    int
	KeyPrefix     string
	BaseURL       string
	GuestKey      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (s *Store) SaveCloudIdentity(identity CloudIdentity) error {
	privateCipher, err := s.cipher.Encrypt(identity.X25519PrivateKey)
	if err != nil {
		return err
	}
	deviceCipher, err := s.cipher.Encrypt(identity.DevicePrivateKey)
	if err != nil {
		return err
	}
	now := time.Now()
	created := identity.CreatedAt
	if created.IsZero() {
		created = now
	}
	_, err = s.db.Exec(`INSERT INTO cloud_identities
		(user_id,x25519_public_key,x25519_private_cipher,device_public_key,device_private_cipher,device_public_id,
		 device_name,relay_enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET x25519_public_key=excluded.x25519_public_key,
		x25519_private_cipher=excluded.x25519_private_cipher,device_public_key=excluded.device_public_key,
		device_private_cipher=excluded.device_private_cipher,device_public_id=excluded.device_public_id,
		device_name=excluded.device_name,relay_enabled=excluded.relay_enabled,updated_at=excluded.updated_at`,
		identity.UserID, identity.X25519PublicKey, privateCipher, identity.DevicePublicKey, deviceCipher,
		identity.DevicePublicID, identity.DeviceName, boolInt(identity.RelayEnabled), created.Unix(), now.Unix())
	return err
}

func (s *Store) LoadCloudIdentity(userID int64) (*CloudIdentity, error) {
	var identity CloudIdentity
	var privateCipher, deviceCipher string
	var relayEnabled int
	var createdAt, updatedAt int64
	err := s.db.QueryRow(`SELECT user_id,x25519_public_key,x25519_private_cipher,device_public_key,device_private_cipher,
		device_public_id,device_name,relay_enabled,created_at,updated_at FROM cloud_identities WHERE user_id=?`, userID).Scan(
		&identity.UserID, &identity.X25519PublicKey, &privateCipher, &identity.DevicePublicKey, &deviceCipher,
		&identity.DevicePublicID, &identity.DeviceName, &relayEnabled, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if identity.X25519PrivateKey, err = s.cipher.Decrypt(privateCipher); err != nil {
		return nil, err
	}
	if identity.DevicePrivateKey, err = s.cipher.Decrypt(deviceCipher); err != nil {
		return nil, err
	}
	identity.RelayEnabled = relayEnabled != 0
	identity.CreatedAt = unixToTime(createdAt)
	identity.UpdatedAt = unixToTime(updatedAt)
	return &identity, nil
}

func (s *Store) UpdateCloudDevice(userID int64, publicID string, relayEnabled bool) error {
	result, err := s.db.Exec(`UPDATE cloud_identities SET device_public_id=?,relay_enabled=?,updated_at=? WHERE user_id=?`,
		publicID, boolInt(relayEnabled), time.Now().Unix(), userID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SaveCloudReceivedKey(key CloudReceivedKey) error {
	ciphertext, err := s.cipher.Encrypt(key.GuestKey)
	if err != nil {
		return err
	}
	now := time.Now()
	created := key.CreatedAt
	if created.IsZero() {
		created = now
	}
	_, err = s.db.Exec(`INSERT INTO cloud_received_keys
		(user_id,grant_public_id,key_version,key_prefix,base_url,guest_key_cipher,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(user_id,grant_public_id) DO UPDATE SET
		key_version=excluded.key_version,key_prefix=excluded.key_prefix,base_url=excluded.base_url,
		guest_key_cipher=excluded.guest_key_cipher,updated_at=excluded.updated_at`,
		key.UserID, key.GrantPublicID, key.KeyVersion, key.KeyPrefix, key.BaseURL, ciphertext, created.Unix(), now.Unix())
	return err
}

func (s *Store) LoadCloudReceivedKey(userID int64, grantPublicID string) (*CloudReceivedKey, error) {
	var key CloudReceivedKey
	var ciphertext string
	var createdAt, updatedAt int64
	err := s.db.QueryRow(`SELECT user_id,grant_public_id,key_version,key_prefix,base_url,guest_key_cipher,created_at,updated_at
		FROM cloud_received_keys WHERE user_id=? AND grant_public_id=?`, userID, grantPublicID).Scan(
		&key.UserID, &key.GrantPublicID, &key.KeyVersion, &key.KeyPrefix, &key.BaseURL, &ciphertext, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if key.GuestKey, err = s.cipher.Decrypt(ciphertext); err != nil {
		return nil, err
	}
	key.CreatedAt = unixToTime(createdAt)
	key.UpdatedAt = unixToTime(updatedAt)
	return &key, nil
}

func (s *Store) DeleteCloudReceivedKey(userID int64, grantPublicID string) error {
	_, err := s.db.Exec("DELETE FROM cloud_received_keys WHERE user_id=? AND grant_public_id=?", userID, grantPublicID)
	return err
}
