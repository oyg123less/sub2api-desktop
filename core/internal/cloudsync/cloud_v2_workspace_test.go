package cloudsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

func TestReceivedShareUsesCatalogDefaultTestModel(t *testing.T) {
	vaultKey := make([]byte, keySize)
	for index := range vaultKey {
		vaultKey[index] = byte(index + 1)
	}
	var receivedModel string
	var receivedStream bool
	var receivedMaxOutputTokens *int
	var receivedAccept string
	var receivedInput []struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/refresh":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access","access_expires_in":900,"refresh_token":"refresh-next"}`))
		case "/v1/responses":
			var payload struct {
				Model           string `json:"model"`
				Stream          bool   `json:"stream"`
				MaxOutputTokens *int   `json:"max_output_tokens"`
				Input           []struct {
					Type    string `json:"type"`
					Role    string `json:"role"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode test request: %v", err)
			}
			receivedModel = payload.Model
			receivedStream = payload.Stream
			receivedMaxOutputTokens = payload.MaxOutputTokens
			receivedAccept = r.Header.Get("Accept")
			receivedInput = payload.Input
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"shared-test"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	st := openTestStore(t, "received-share-test-model")
	if err := st.SaveCloudSession(store.CloudSession{
		UserID: 1, Email: "recipient@example.test", Role: "user", SaltKDF: "salt-kdf", SaltAuth: "salt-auth",
		WrappedVaultKey: "wrapped", VaultKey: rawURL(vaultKey), RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedKey(store.CloudReceivedKey{
		UserID: 1, GrantPublicID: "sgr_test_model", KeyVersion: 1, KeyPrefix: "sk-amber-test",
		BaseURL: server.URL + "/v1", GuestKey: "sk-amber-received-test-key",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCloudReceivedAccountLink(store.CloudReceivedAccountLink{
		UserID: 1, GrantPublicID: "sgr_test_model", OwnerName: "Owner", GroupName: "Shared test",
		RemoteStatus: "active", Enabled: false, RPMLimit: 30, ConcurrencyLimit: 2,
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(st, &testSettings{value: store.DefaultSettings()}, server.URL, "site-key", server.Client(), nil)
	result, err := manager.TestReceivedShare(context.Background(), "sgr_test_model")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || receivedModel != openai.DefaultTestModel {
		t.Fatalf("received share test = %#v, model = %q", result, receivedModel)
	}
	if !receivedStream || receivedMaxOutputTokens != nil || receivedAccept != "text/event-stream" {
		t.Fatalf("received share test stream = %v, max_output_tokens = %v, accept = %q", receivedStream, receivedMaxOutputTokens, receivedAccept)
	}
	if len(receivedInput) != 1 || receivedInput[0].Type != "message" || receivedInput[0].Role != "user" ||
		len(receivedInput[0].Content) != 1 || receivedInput[0].Content[0].Type != "input_text" ||
		receivedInput[0].Content[0].Text == "" {
		t.Fatalf("received share test input = %#v", receivedInput)
	}
	links, err := st.ListCloudReceivedAccountLinks(1)
	if err != nil || len(links) != 1 || !links[0].Enabled || links[0].HealthStatus != "healthy" || links[0].LastCheckedAt.IsZero() {
		t.Fatalf("received share health was not enabled after a successful test: %#v, %v", links, err)
	}
}

func TestListConnectEventsUsesCursor(t *testing.T) {
	vaultKey := make([]byte, keySize)
	for index := range vaultKey {
		vaultKey[index] = byte(index + 1)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/refresh":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access","access_expires_in":900,"refresh_token":"refresh-next"}`))
		case "/v1/events":
			if r.URL.Query().Get("cursor") != "41" || r.URL.Query().Get("limit") != "100" {
				t.Errorf("unexpected event query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"events":[{"id":42,"event_type":"connect.access_updated","entity_type":"received_share","entity_public_id":"sgr_test","payload":{},"created_at":"2026-07-19T00:00:00Z"}],"cursor":42,"has_more":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	st := openTestStore(t, "connect-events")
	if err := st.SaveCloudSession(store.CloudSession{
		UserID: 1, Email: "recipient@example.test", Role: "user", SaltKDF: "salt-kdf", SaltAuth: "salt-auth",
		WrappedVaultKey: "wrapped", VaultKey: rawURL(vaultKey), RefreshToken: "refresh",
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(st, &testSettings{value: store.DefaultSettings()}, server.URL, "site-key", server.Client(), nil)
	response, err := manager.ListConnectEvents(context.Background(), 41)
	if err != nil || response.Cursor != 42 || len(response.Events) != 1 || response.Events[0].EntityPublicID != "sgr_test" {
		t.Fatalf("unexpected event response: %#v, %v", response, err)
	}
}

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
