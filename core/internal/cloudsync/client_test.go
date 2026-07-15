package cloudsync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminClientKeepsSecondFactorInHeader(t *testing.T) {
	const accessToken = "test-access-token"
	const adminKey = "transient-admin-second-factor"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			t.Errorf("authorization header = %q", got)
		}
		if got := r.Header.Get("X-Admin-Key"); got != adminKey {
			t.Errorf("admin header = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		if strings.Contains(r.URL.String(), adminKey) || strings.Contains(string(body), adminKey) {
			t.Fatal("administrator key escaped the request header")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/admin/users":
			_, _ = io.WriteString(w, `{"users":[{"id":2,"email":"user@example.test","role":"user"}]}`)
		case "/v1/admin/settings":
			_, _ = io.WriteString(w, `{"settings":[{"key":"registration_enabled","value":"true"}]}`)
		case "/v1/admin/stats":
			_, _ = io.WriteString(w, `{"users":2,"daily_active_users":1,"vault_items":4}`)
		case "/v1/admin/audit":
			_, _ = io.WriteString(w, `{"audit":[]}`)
		default:
			t.Errorf("unexpected admin path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "not_found"}})
		}
	}))
	defer server.Close()

	client, err := newCloudClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("new cloud client: %v", err)
	}
	overview, err := client.adminOverview(context.Background(), accessToken, adminKey)
	if err != nil {
		t.Fatalf("admin overview: %v", err)
	}
	if len(overview.Users) != 1 || overview.Stats.Users != 2 || overview.Stats.VaultItems != 4 {
		t.Fatalf("unexpected overview: %+v", overview)
	}
}
