package control

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"

	"sub2api-desktop/core/internal/store"
)

func TestProxyTestRetriesTransientTargetFailure(t *testing.T) {
	var attempts atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
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
	port, _ := strconv.Atoi(portText)
	result := testProxyLatencyTo(context.Background(), &store.Proxy{Type: store.ProxyHTTP, Host: host, Port: port}, "http://example.test/health")
	if !result.OK {
		t.Fatalf("proxy test failed after retry: %+v", result)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestProxyTestLive(t *testing.T) {
	raw := os.Getenv("AMBER_TEST_PROXY")
	if raw == "" {
		t.Skip("AMBER_TEST_PROXY is not set")
	}
	parsed, err := url.Parse(raw)
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
	proxyType := store.ProxyType(parsed.Scheme)
	result := testProxyLatency(context.Background(), &store.Proxy{Type: proxyType, Host: host, Port: port})
	if !result.OK {
		t.Fatalf("live proxy test failed: %+v", result)
	}
}
