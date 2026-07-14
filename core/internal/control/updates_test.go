package control

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"sub2api-desktop/core/internal/store"
)

type updateTestSettings struct {
	value store.Settings
}

func (s *updateTestSettings) Get() store.Settings       { return s.value }
func (s *updateTestSettings) Save(store.Settings) error { return nil }

func TestUpdateCheckerCachesReleaseAndUsesConfiguredUserAgent(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("User-Agent"); got != "Amber-Test/0.2.1" {
			t.Errorf("User-Agent = %q", got)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.0","name":"Amber 0.3.0","body":"notes","html_url":"https://example.test/release","published_at":"2026-07-14T00:00:00Z"}`))
	}))
	defer server.Close()

	checker := &updateChecker{
		settings:    &updateTestSettings{value: store.Settings{UserAgent: "Amber-Test/0.2.1"}},
		listProxies: func() ([]*store.Proxy, error) { return nil, nil },
		version:     "0.2.1",
		latestURL:   server.URL,
	}
	first, err := checker.latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := checker.latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.TagName != "v0.3.0" || first.HTMLURL == "" || first.CheckedAt.IsZero() {
		t.Fatalf("unexpected release: %+v", first)
	}
	if second.TagName != first.TagName || calls.Load() != 1 {
		t.Fatalf("cached release mismatch or extra request: second=%+v calls=%d", second, calls.Load())
	}
}

func TestUpdateCheckerUsesSavedProxy(t *testing.T) {
	var proxyCalls atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.0","html_url":"https://example.test/release"}`))
	}))
	defer proxyServer.Close()
	parsed, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	checker := &updateChecker{
		settings: &updateTestSettings{value: store.Settings{UserAgent: "Amber-Test"}},
		listProxies: func() ([]*store.Proxy, error) {
			return []*store.Proxy{{Type: store.ProxyHTTP, Host: host, Port: port}}, nil
		},
		version:   "0.2.1",
		latestURL: "http://github.invalid/releases/latest",
	}
	if _, err := checker.latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if proxyCalls.Load() != 1 {
		t.Fatalf("proxy calls = %d, want 1", proxyCalls.Load())
	}
}
