package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type cloudRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cloudRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func cloudJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestCloudClientRetriesSafeReads(t *testing.T) {
	attempts := 0
	client, err := newCloudClient("https://cloud.example", &http.Client{Transport: cloudRoundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return nil, &net.DNSError{Err: "temporary lookup failure", Name: "cloud.example", IsTemporary: true}
		}
		return cloudJSONResponse(http.StatusOK, `{"items":[],"cursor":"cursor-1"}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	client.retryDelays = []time.Duration{0, 0}
	response, err := client.pull(context.Background(), "access", "")
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || response.Cursor != "cursor-1" {
		t.Fatalf("attempts=%d response=%+v", attempts, response)
	}
}

func TestCloudClientDoesNotReplayVaultWrites(t *testing.T) {
	attempts := 0
	client, err := newCloudClient("https://cloud.example", &http.Client{Transport: cloudRoundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return nil, context.DeadlineExceeded
	})})
	if err != nil {
		t.Fatal(err)
	}
	client.retryDelays = []time.Duration{0, 0}
	_, err = client.push(context.Background(), "access", []remoteVaultItem{{Kind: "account", ClientUID: "018f1f46-7a19-7cc2-88cb-f577e51d3999", Ciphertext: "v1.test"}})
	if err == nil {
		t.Fatal("push unexpectedly succeeded")
	}
	var cloudErr *CloudError
	if !errors.As(err, &cloudErr) || cloudErr.Code != "cloud_timeout" || cloudErr.Attempt != 1 {
		t.Fatalf("unexpected error: %#v", err)
	}
	if attempts != 1 {
		t.Fatalf("write attempts=%d, want 1", attempts)
	}
}

func TestCloudTransportErrorStages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "dns", err: &net.DNSError{Err: "missing", Name: "cloud.example"}, want: "dns"},
		{name: "timeout", err: context.DeadlineExceeded, want: "timeout"},
		{name: "connect", err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}, want: "connect"},
		{name: "tls", err: errors.New("remote error: tls handshake failure"), want: "tls"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cloudTransportStage(test.err); got != test.want {
				t.Fatalf("stage=%q, want %q", got, test.want)
			}
		})
	}
}

func TestParseStaticProxy(t *testing.T) {
	tests := []struct {
		value, scheme, want string
	}{
		{"127.0.0.1:7890", "https", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:8080;https=127.0.0.1:8443", "https", "http://127.0.0.1:8443"},
		{"socks5://127.0.0.1:1080", "https", "socks5://127.0.0.1:1080"},
	}
	for _, test := range tests {
		proxy := parseStaticProxy(test.value, test.scheme)
		if proxy == nil || proxy.String() != test.want {
			t.Fatalf("parseStaticProxy(%q, %q)=%v, want %q", test.value, test.scheme, proxy, test.want)
		}
	}
}

func TestSyncRetryDelay(t *testing.T) {
	want := []time.Duration{5 * time.Second, 15 * time.Second, time.Minute, 5 * time.Minute, 5 * time.Minute}
	for index, expected := range want {
		if got := syncRetryDelay(index + 1); got != expected {
			t.Fatalf("failure %d delay=%s, want %s", index+1, got, expected)
		}
	}
}

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
		case "/v1/admin/shares":
			_, _ = io.WriteString(w, `{"shares":[]}`)
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
