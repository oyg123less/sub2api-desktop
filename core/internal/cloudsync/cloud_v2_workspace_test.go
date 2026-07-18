package cloudsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"sub2api-desktop/core/internal/store"
)

func TestLoadWorkspaceRunsIndependentCloudReadsConcurrently(t *testing.T) {
	vaultKey := make([]byte, keySize)
	for index := range vaultKey {
		vaultKey[index] = byte(index + 1)
	}
	privateKey, publicKey, err := generateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	privateCipher, err := encryptVaultItem(vaultKey, []byte(privateKey))
	if err != nil {
		t.Fatal(err)
	}
	var active atomic.Int32
	var maximum atomic.Int32
	delayed := func(w http.ResponseWriter, body string) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/refresh":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access","access_expires_in":900,"refresh_token":"refresh-next"}`))
		case "/v1/profile":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"profile":{"display_name":"Owner","friend_code":"AMB-TEST-0001","encryption_public_key":%q,"encryption_private_cipher":%q,"encryption_key_version":1,"created_at":"2026-07-18T00:00:00Z","updated_at":"2026-07-18T00:00:00Z"}}`, publicKey, privateCipher)
		case "/v1/friends":
			delayed(w, `{"friends":[]}`)
		case "/v1/friend-requests":
			delayed(w, `{"requests":[]}`)
		case "/v1/share-groups":
			delayed(w, `{"groups":[]}`)
		case "/v1/received-shares":
			delayed(w, `{"shares":[]}`)
		case "/v1/devices":
			delayed(w, `{"devices":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	st := openTestStore(t, "workspace-parallel")
	if err := st.SaveCloudSession(store.CloudSession{
		UserID: 1, Email: "owner@example.test", Role: "user", SaltKDF: "salt-kdf", SaltAuth: "salt-auth",
		WrappedVaultKey: "wrapped", VaultKey: rawURL(vaultKey), RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudIdentity(store.CloudIdentity{
		UserID: 1, X25519PublicKey: publicKey, X25519PrivateKey: privateKey,
		DevicePublicKey: "device-public", DevicePrivateKey: "device-private", DeviceName: "Test device",
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(st, &testSettings{value: store.DefaultSettings()}, server.URL, "site-key", server.Client(), nil)

	started := time.Now()
	workspace, err := manager.LoadWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Profile.DisplayName != "Owner" {
		t.Fatalf("unexpected profile: %#v", workspace.Profile)
	}
	if maximum.Load() < 4 {
		t.Fatalf("workspace reads were serialized; maximum concurrency=%d", maximum.Load())
	}
	if elapsed := time.Since(started); elapsed >= 450*time.Millisecond {
		t.Fatalf("parallel workspace load took %s", elapsed)
	}
}
