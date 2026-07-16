package cloudsync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sub2api-desktop/core/internal/crypto"
	"sub2api-desktop/core/internal/store"
)

type testSettings struct {
	mu    sync.Mutex
	value store.Settings
}

func (s *testSettings) Get() store.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

func (s *testSettings) Save(value store.Settings) error {
	s.mu.Lock()
	s.value = value
	s.mu.Unlock()
	return nil
}

type mockCloud struct {
	mu         sync.Mutex
	email      string
	authHash   string
	saltKDF    string
	saltAuth   string
	wrapped    string
	items      map[string]remoteVaultItem
	lastUpload string
	resends    int
	pullPages  [][]remoteVaultItem
	pullCalls  int
}

func newMockCloud() *mockCloud {
	return &mockCloud{items: make(map[string]remoteVaultItem)}
}

func (m *mockCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	decode := func(target any) bool {
		if err := json.NewDecoder(r.Body).Decode(target); err != nil {
			http.Error(w, `{"error":{"code":"invalid_json","message":"invalid"}}`, http.StatusBadRequest)
			return false
		}
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch r.URL.Path {
	case "/v1/auth/register":
		var body registerRequest
		if !decode(&body) {
			return
		}
		m.email, m.authHash, m.saltKDF, m.saltAuth, m.wrapped = body.Email, body.AuthHash, body.SaltKDF, body.SaltAuth, body.WrappedVaultKey
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	case "/v1/auth/verify-email", "/v1/auth/logout":
		_, _ = w.Write([]byte(`{"ok":true}`))
	case "/v1/auth/resend-verification":
		m.resends++
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	case "/v1/auth/parameters":
		_, _ = fmt.Fprintf(w, `{"salt_kdf":%q,"salt_auth":%q}`, m.saltKDF, m.saltAuth)
	case "/v1/auth/login":
		var body map[string]string
		if !decode(&body) {
			return
		}
		if body["email"] != m.email || body["auth_hash"] != m.authHash {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_credentials","message":"invalid credentials"}}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"access_token":"access","access_expires_in":900,"refresh_token":"refresh","user":{"id":1,"email":%q,"role":"user"},"salt_kdf":%q,"salt_auth":%q,"wrapped_vault_key":%q}`,
			m.email, m.saltKDF, m.saltAuth, m.wrapped)
	case "/v1/auth/refresh":
		_, _ = w.Write([]byte(`{"access_token":"access","access_expires_in":900,"refresh_token":"refresh-next"}`))
	case "/v1/vault":
		if len(m.pullPages) > 0 {
			index := min(m.pullCalls, len(m.pullPages)-1)
			page := m.pullPages[index]
			m.pullCalls++
			_ = json.NewEncoder(w).Encode(pullResponse{Items: page, Cursor: fmt.Sprintf("2026-07-17T00:00:00.000Z|%d", (index+1)*cloudPullPageSize)})
			return
		}
		items := make([]remoteVaultItem, 0, len(m.items))
		for _, item := range m.items {
			items = append(items, item)
		}
		_ = json.NewEncoder(w).Encode(pullResponse{Items: items, Cursor: time.Now().UTC().Format(time.RFC3339Nano)})
	case "/v1/vault/batch":
		var body struct {
			Items []remoteVaultItem `json:"items"`
		}
		raw := new(strings.Builder)
		decoder := json.NewDecoder(r.Body)
		var payload json.RawMessage
		if err := decoder.Decode(&payload); err != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		raw.Write(payload)
		m.lastUpload = raw.String()
		if err := json.Unmarshal(payload, &body); err != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		updated := make([]remoteVaultItem, 0, len(body.Items))
		for _, item := range body.Items {
			key := item.Kind + ":" + item.ClientUID
			existing, found := m.items[key]
			if found && existing.Version != item.Version {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "vault_conflict", "message": "conflict"}, "conflicts": []remoteVaultItem{existing}})
				return
			}
			item.Version++
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			m.items[key] = item
			updated = append(updated, item)
		}
		_ = json.NewEncoder(w).Encode(pushResponse{Items: updated, Cursor: time.Now().UTC().Format(time.RFC3339Nano)})
	default:
		http.NotFound(w, r)
	}
}

func TestPendingRegistrationCanBeResentAndCancelled(t *testing.T) {
	cloud := newMockCloud()
	server := httptest.NewServer(cloud)
	defer server.Close()
	manager := NewManager(openTestStore(t, "pending"), &testSettings{value: store.DefaultSettings()}, server.URL, "site-key", server.Client(), nil)
	t.Cleanup(manager.Close)
	ctx := context.Background()
	const email = "pending@example.test"
	if err := manager.Register(ctx, RegisterInput{Email: email, Password: "correct horse battery staple", TurnstileToken: "test"}); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if !status.PendingVerification || status.Email != email {
		t.Fatalf("unexpected pending status: %+v", status)
	}
	if err := manager.ResendVerification(ctx, email); err != nil {
		t.Fatal(err)
	}
	cloud.mu.Lock()
	resends := cloud.resends
	cloud.mu.Unlock()
	if resends != 1 {
		t.Fatalf("resends = %d, want 1", resends)
	}
	manager.CancelRegistration()
	status = manager.Status()
	if status.PendingVerification || status.Email != "" {
		t.Fatalf("pending registration was not cleared: %+v", status)
	}
	if err := manager.ResendVerification(ctx, email); err == nil {
		t.Fatal("resend succeeded after pending registration was cancelled")
	}
}

func TestSyncPullsMultipleRemotePagesInOneRun(t *testing.T) {
	cloud := newMockCloud()
	server := httptest.NewServer(cloud)
	defer server.Close()
	st := openTestStore(t, "multipage")
	manager := NewManager(st, &testSettings{value: store.DefaultSettings()}, server.URL, "site-key", server.Client(), nil)
	t.Cleanup(manager.Close)
	ctx := context.Background()
	if err := manager.Register(ctx, RegisterInput{Email: "pages@example.test", Password: "correct horse battery staple", TurnstileToken: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.VerifyEmail(ctx, "pages@example.test", "123456"); err != nil {
		t.Fatal(err)
	}
	manager.mu.RLock()
	vaultKey := append([]byte(nil), manager.vaultKey...)
	manager.mu.RUnlock()
	defer wipe(vaultKey)
	ciphertext, err := encryptEnvelope(vaultKey, time.Now(), map[string]string{"deleted": "item"})
	if err != nil {
		t.Fatal(err)
	}
	first := make([]remoteVaultItem, cloudPullPageSize)
	for index := range first {
		first[index] = remoteVaultItem{Kind: store.CloudKindAccount, ClientUID: fmt.Sprintf("deleted-%04d", index), Ciphertext: ciphertext, Version: 1, Deleted: true}
	}
	second := []remoteVaultItem{{Kind: store.CloudKindAccount, ClientUID: "deleted-final", Ciphertext: ciphertext, Version: 1, Deleted: true}}
	cloud.mu.Lock()
	cloud.pullPages = [][]remoteVaultItem{first, second}
	cloud.mu.Unlock()
	if err := manager.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	cloud.mu.Lock()
	pullCalls := cloud.pullCalls
	cloud.mu.Unlock()
	if pullCalls != 2 {
		t.Fatalf("pull calls = %d, want 2", pullCalls)
	}
}

func openTestStore(t *testing.T, name string) *store.Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cipher, err := crypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "data.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSyncCopiesEncryptedDataBetweenTwoInstallations(t *testing.T) {
	cloud := newMockCloud()
	server := httptest.NewServer(cloud)
	defer server.Close()
	settingsA := &testSettings{value: store.DefaultSettings()}
	settingsB := &testSettings{value: store.DefaultSettings()}
	storeA := openTestStore(t, "a")
	storeB := openTestStore(t, "b")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	managerA := NewManager(storeA, settingsA, server.URL, "site-key", server.Client(), logger)
	managerB := NewManager(storeB, settingsB, server.URL, "site-key", server.Client(), logger)
	t.Cleanup(managerA.Close)
	t.Cleanup(managerB.Close)

	ctx := context.Background()
	if err := managerA.Register(ctx, RegisterInput{Email: "sync@example.test", Password: "correct horse battery staple", TurnstileToken: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := managerA.VerifyEmail(ctx, "sync@example.test", "123456"); err != nil {
		t.Fatal(err)
	}
	proxy, err := storeA.CreateProxy(&store.Proxy{Name: "edge", Type: store.ProxyHTTP, Host: "127.0.0.1", Port: 7890, Username: "proxy-user", Password: "proxy-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.CreateAccount(&store.Account{
		AccountType: store.AccountTypeOAuth, Email: "upstream@example.test", ChatGPTAccountID: "acct-sync",
		AccessToken: "secret-access-token", RefreshToken: "secret-refresh-token", IDToken: "secret-id-token",
		ExpiresAt: time.Now().Add(time.Hour), Status: store.AccountActive, ProxyID: &proxy.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.CreateCodexRemoteTarget(&store.CodexRemoteTarget{
		Name: "remote", Host: "host.example.test", Port: 22, User: "deploy", Password: "ssh-secret",
		RemotePort: 8080, Model: "gpt-5.6", Mode: "tunnel", TunnelEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := managerA.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	cloud.mu.Lock()
	upload := cloud.lastUpload
	cloud.mu.Unlock()
	for _, secret := range []string{"secret-access-token", "secret-refresh-token", "proxy-secret", "ssh-secret"} {
		if strings.Contains(upload, secret) {
			t.Fatalf("cloud upload exposed %q", secret)
		}
	}

	if err := managerB.Login(ctx, "sync@example.test", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := managerB.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	accounts, err := storeB.ListAccounts()
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts=%d err=%v", len(accounts), err)
	}
	if accounts[0].AccessToken != "secret-access-token" || accounts[0].RefreshToken != "secret-refresh-token" {
		t.Fatal("account credentials did not round-trip")
	}
	proxies, err := storeB.ListProxies()
	if err != nil || len(proxies) != 1 || proxies[0].Password != "proxy-secret" {
		t.Fatalf("proxies=%#v err=%v", proxies, err)
	}
	targets, err := storeB.ListCodexRemoteTargets()
	if err != nil || len(targets) != 1 || targets[0].Password != "ssh-secret" {
		t.Fatalf("targets=%#v err=%v", targets, err)
	}
	if accounts[0].ProxyID == nil || *accounts[0].ProxyID != proxies[0].ID {
		t.Fatal("account proxy relationship was not restored")
	}
	pending, err := storeB.PendingCloudCount()
	if err != nil || pending != 0 {
		t.Fatalf("pending=%d err=%v", pending, err)
	}
}
