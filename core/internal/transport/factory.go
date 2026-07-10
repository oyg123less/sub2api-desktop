// Package transport is the single construction point for every outbound HTTP
// client used by the gateway, OAuth, proxy tests, and diagnostics.
package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"sub2api-desktop/core/internal/store"
	"sub2api-desktop/core/internal/tlsfingerprint"
)

type Purpose string

const (
	PurposeGateway    Purpose = "gateway"
	PurposeOAuth      Purpose = "oauth"
	PurposeProxyTest  Purpose = "proxy_test"
	PurposeDiagnostic Purpose = "diagnostic"
)

type ErrorKind string

const (
	ErrorProxyConnect ErrorKind = "proxy_connect"
	ErrorProxyAuth    ErrorKind = "proxy_auth"
	ErrorProxyTLS     ErrorKind = "proxy_tls"
	ErrorTargetTLS    ErrorKind = "target_tls"
	ErrorTargetHTTP   ErrorKind = "target_http"
)

type Error struct {
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string { return string(e.Kind) + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type Options struct {
	Proxy              *store.Proxy
	Purpose            Purpose
	FingerprintProfile string
	Timeout            time.Duration
	RootCAs            *x509.CertPool
}

func NewClient(options Options) (*http.Client, error) {
	proxyURL, err := buildProxyURL(options.Proxy)
	if err != nil {
		return nil, err
	}
	profile := strings.TrimSpace(options.FingerprintProfile)
	standardProfile := profile == "" || profile == "standard"
	transport := &http.Transport{
		// Some local mixed-port proxies terminate CONNECT tunnels when Go
		// advertises HTTP/2. Keep the v0.1.1 HTTP/1.1 behavior for proxied
		// standard traffic while retaining HTTP/2 for direct connections.
		ForceAttemptHTTP2:     proxyURL == nil && standardProfile,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: options.RootCAs},
	}

	useFingerprint := !standardProfile
	if useFingerprint {
		transport.ForceAttemptHTTP2 = false
		fingerprint := &tlsfingerprint.Profile{Name: profile}
		switch {
		case proxyURL == nil:
			transport.DialTLSContext = tlsfingerprint.NewDialer(fingerprint, nil).DialTLSContext
		case proxyURL.Scheme == "socks5":
			transport.DialTLSContext = tlsfingerprint.NewSOCKS5ProxyDialer(fingerprint, proxyURL).DialTLSContext
		default:
			transport.DialTLSContext = tlsfingerprint.NewHTTPProxyDialer(fingerprint, proxyURL).DialTLSContext
		}
	} else if proxyURL != nil {
		if proxyURL.Scheme == "socks5" {
			dialContext, err := socks5DialContext(proxyURL)
			if err != nil {
				return nil, err
			}
			transport.DialContext = dialContext
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	base := http.RoundTripper(transport)
	if options.Proxy != nil {
		base = &classifiedRoundTripper{base: transport, proxyType: options.Proxy.Type}
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout(options.Purpose)
	}
	return &http.Client{Transport: base, Timeout: timeout}, nil
}

func defaultTimeout(purpose Purpose) time.Duration {
	switch purpose {
	case PurposeGateway:
		return 10 * time.Minute
	case PurposeProxyTest, PurposeDiagnostic:
		return 15 * time.Second
	default:
		return 60 * time.Second
	}
}

func buildProxyURL(value *store.Proxy) (*url.URL, error) {
	if value == nil {
		return nil, nil
	}
	if value.Host == "" || value.Port < 1 || value.Port > 65535 {
		return nil, &Error{Kind: ErrorProxyConnect, Err: errors.New("proxy host or port is invalid")}
	}
	scheme := strings.ToLower(strings.TrimSpace(string(value.Type)))
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		return nil, &Error{Kind: ErrorProxyConnect, Err: fmt.Errorf("unsupported proxy type %q", scheme)}
	}
	result := &url.URL{Scheme: scheme, Host: net.JoinHostPort(value.Host, fmt.Sprintf("%d", value.Port))}
	if value.Username != "" {
		if value.Password == "" {
			result.User = url.User(value.Username)
		} else {
			result.User = url.UserPassword(value.Username, value.Password)
		}
	}
	return result, nil
}

func socks5DialContext(proxyURL *url.URL) (func(context.Context, string, string) (net.Conn, error), error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		auth = &proxy.Auth{User: proxyURL.User.Username(), Password: password}
	}
	base := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, base)
	if err != nil {
		return nil, &Error{Kind: ErrorProxyConnect, Err: err}
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, &Error{Kind: ErrorProxyConnect, Err: errors.New("SOCKS5 dialer does not support context")}
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		connection, err := contextDialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, &Error{Kind: ErrorProxyConnect, Err: err}
		}
		return connection, nil
	}, nil
}

type classifiedRoundTripper struct {
	base      http.RoundTripper
	proxyType store.ProxyType
}

func (r *classifiedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(request)
	if err != nil {
		return nil, classifyProxyError(err, r.proxyType)
	}
	return response, nil
}

func classifyProxyError(err error, proxyType store.ProxyType) error {
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "407") || strings.Contains(message, "proxy authentication"):
		return &Error{Kind: ErrorProxyAuth, Err: errors.New("proxy authentication failed")}
	case strings.Contains(message, "proxy tls") || (proxyType == store.ProxyHTTPS && strings.Contains(message, "certificate")):
		return &Error{Kind: ErrorProxyTLS, Err: errors.New("TLS negotiation with proxy failed")}
	case strings.Contains(message, "proxyconnect") || strings.Contains(message, "proxy connect") ||
		strings.Contains(message, "socks5") || strings.Contains(message, "connection refused") ||
		strings.Contains(message, "connectex") || strings.Contains(message, "dial tcp"):
		return &Error{Kind: ErrorProxyConnect, Err: err}
	case strings.Contains(message, "tls") || strings.Contains(message, "certificate"):
		return &Error{Kind: ErrorTargetTLS, Err: errors.New("TLS negotiation with target failed")}
	default:
		return &Error{Kind: ErrorTargetHTTP, Err: err}
	}
}

// IsTransientNetworkError reports whether one retry may recover from an
// interrupted tunnel or a short-lived network stall.
func IsTransientNetworkError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{"unexpected eof", "connection reset", "connection aborted", "broken pipe", "use of closed network connection"} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func Kind(err error) ErrorKind {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Kind
	}
	return ErrorTargetHTTP
}
