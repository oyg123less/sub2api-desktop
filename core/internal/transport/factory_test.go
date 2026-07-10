package transport

import (
	"io"
	"net/http"
	"testing"

	"sub2api-desktop/core/internal/store"
)

func TestStandardHTTP2Policy(t *testing.T) {
	direct, err := NewClient(Options{FingerprintProfile: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	directTransport, ok := direct.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("direct transport type = %T", direct.Transport)
	}
	if !directTransport.ForceAttemptHTTP2 {
		t.Fatal("direct standard client should retain HTTP/2")
	}

	proxied, err := NewClient(Options{
		Proxy:              &store.Proxy{Type: store.ProxyHTTP, Host: "127.0.0.1", Port: 12000},
		FingerprintProfile: "standard",
	})
	if err != nil {
		t.Fatal(err)
	}
	wrapper, ok := proxied.Transport.(*classifiedRoundTripper)
	if !ok {
		t.Fatalf("proxied transport type = %T", proxied.Transport)
	}
	proxyTransport, ok := wrapper.base.(*http.Transport)
	if !ok {
		t.Fatalf("wrapped transport type = %T", wrapper.base)
	}
	if proxyTransport.ForceAttemptHTTP2 {
		t.Fatal("proxied standard client must keep the v0.1.1 HTTP/1.1 behavior")
	}
}

func TestUnexpectedEOFClassificationAndRetryability(t *testing.T) {
	err := classifyProxyError(io.ErrUnexpectedEOF, store.ProxyHTTP)
	if got := Kind(err); got != ErrorTargetHTTP {
		t.Fatalf("kind = %q, want %q", got, ErrorTargetHTTP)
	}
	if !IsTransientNetworkError(err) {
		t.Fatal("unexpected EOF should be retryable")
	}
}
