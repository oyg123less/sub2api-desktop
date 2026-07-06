package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"sub2api-desktop/core/internal/store"
	"sub2api-desktop/core/internal/tlsfingerprint"
)

// buildProxyURL converts a store.Proxy into a *url.URL, or nil for no proxy.
func buildProxyURL(p *store.Proxy) (*url.URL, error) {
	if p == nil {
		return nil, nil
	}
	scheme := string(p.Type)
	if scheme == "" {
		scheme = "http"
	}
	u := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", p.Host, p.Port),
	}
	if p.Username != "" {
		if p.Password != "" {
			u.User = url.UserPassword(p.Username, p.Password)
		} else {
			u.User = url.User(p.Username)
		}
	}
	return u, nil
}

// NewAuthClient builds a plain HTTP client (no TLS fingerprinting) for OAuth
// token endpoints, optionally routed through the given proxy.
func NewAuthClient(proxy *store.Proxy) (*http.Client, error) {
	return newHTTPClient(proxy, false, 60*time.Second)
}

// newHTTPClient builds an *http.Client that optionally aligns the TLS
// fingerprint with the real Codex client and optionally routes through a proxy.
func newHTTPClient(proxy *store.Proxy, useTLSFingerprint bool, timeout time.Duration) (*http.Client, error) {
	proxyURL, err := buildProxyURL(proxy)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		ForceAttemptHTTP2:     false, // Codex client uses HTTP/1.1 ALPN in the fingerprint
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if useTLSFingerprint {
		profile := &tlsfingerprint.Profile{Name: "codex"}
		var dialTLS func(ctx context.Context, network, addr string) (net.Conn, error)
		switch {
		case proxyURL == nil:
			dialTLS = tlsfingerprint.NewDialer(profile, nil).DialTLSContext
		case proxyURL.Scheme == "socks5":
			dialTLS = tlsfingerprint.NewSOCKS5ProxyDialer(profile, proxyURL).DialTLSContext
		default:
			dialTLS = tlsfingerprint.NewHTTPProxyDialer(profile, proxyURL).DialTLSContext
		}
		transport.DialTLSContext = dialTLS
	} else {
		if proxyURL != nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}
