package cloudsync

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
	"golang.org/x/net/websocket"

	"sub2api-desktop/core/internal/store"
)

type NetworkInfo struct {
	Mode          store.CloudConnectionMode `json:"mode"`
	ProxyID       *int64                    `json:"proxy_id,omitempty"`
	ProxyName     string                    `json:"proxy_name,omitempty"`
	ProxyType     string                    `json:"proxy_type,omitempty"`
	EffectiveFrom string                    `json:"effective_source"`
}

func newDefaultCloudHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = cloudProxy
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}

func newConfiguredCloudHTTPClient(baseURL string, value store.CloudConnectionSettings, selected *store.Proxy) (*http.Client, NetworkInfo, error) {
	info := NetworkInfo{Mode: value.Mode, ProxyID: value.ProxyID}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	switch value.Mode {
	case store.CloudConnectionDirect:
		transport.Proxy = nil
		info.EffectiveFrom = "direct"
	case store.CloudConnectionSystem:
		transport.Proxy = cloudProxy
		info.EffectiveFrom = systemProxySource(baseURL)
	case store.CloudConnectionProxy:
		if selected == nil || value.ProxyID == nil || selected.ID != *value.ProxyID {
			return nil, info, errors.New("selected Amber Cloud proxy is unavailable")
		}
		proxyURL, err := storedProxyURL(selected)
		if err != nil {
			return nil, info, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
		info.ProxyName = selected.Name
		info.ProxyType = string(selected.Type)
		info.EffectiveFrom = "amber_proxy"
	default:
		return nil, info, fmt.Errorf("unsupported Amber Cloud connection mode %q", value.Mode)
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, info, nil
}

func storedProxyURL(proxy *store.Proxy) (*url.URL, error) {
	if proxy == nil || strings.TrimSpace(proxy.Host) == "" || proxy.Port < 1 || proxy.Port > 65535 {
		return nil, errors.New("selected Amber Cloud proxy is invalid")
	}
	scheme := strings.ToLower(strings.TrimSpace(string(proxy.Type)))
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		return nil, errors.New("selected Amber Cloud proxy type is unsupported")
	}
	result := &url.URL{Scheme: scheme, Host: net.JoinHostPort(strings.TrimSpace(proxy.Host), strconv.Itoa(proxy.Port))}
	if proxy.Username != "" {
		if proxy.Password != "" {
			result.User = url.UserPassword(proxy.Username, proxy.Password)
		} else {
			result.User = url.User(proxy.Username)
		}
	}
	return result, nil
}

func systemProxySource(baseURL string) string {
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/health", nil)
	if err == nil {
		if proxy, proxyErr := http.ProxyFromEnvironment(request); proxyErr == nil && proxy != nil {
			return "environment"
		}
	}
	if parseStaticProxy(systemProxyAddress(), "https") != nil {
		return "windows"
	}
	return "direct"
}

func cloudProxy(request *http.Request) (*url.URL, error) {
	proxy, err := http.ProxyFromEnvironment(request)
	if proxy != nil || err != nil {
		return proxy, err
	}
	return parseStaticProxy(systemProxyAddress(), request.URL.Scheme), nil
}

func parseStaticProxy(value, scheme string) *url.URL {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	selected := value
	if strings.Contains(value, ";") || strings.Contains(value, "=") {
		selected = ""
		fallback := ""
		for _, entry := range strings.Split(value, ";") {
			key, address, found := strings.Cut(strings.TrimSpace(entry), "=")
			if !found {
				if fallback == "" {
					fallback = strings.TrimSpace(entry)
				}
				continue
			}
			if strings.EqualFold(strings.TrimSpace(key), scheme) {
				selected = strings.TrimSpace(address)
				break
			}
			if fallback == "" && (strings.EqualFold(strings.TrimSpace(key), "http") || strings.EqualFold(strings.TrimSpace(key), "https")) {
				fallback = strings.TrimSpace(address)
			}
		}
		if selected == "" {
			selected = fallback
		}
	}
	if selected == "" {
		return nil
	}
	if !strings.Contains(selected, "://") {
		selected = "http://" + selected
	}
	parsed, err := url.Parse(selected)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5") {
		return nil
	}
	return parsed
}

func (m *Manager) relayProxyURL() (*url.URL, error) {
	if m.httpOverride != nil {
		return nil, nil
	}
	value, err := m.store.CloudConnectionSettings()
	if err != nil {
		return nil, err
	}
	if err := store.ValidateCloudConnectionSettings(value); err != nil {
		return nil, err
	}
	switch value.Mode {
	case store.CloudConnectionDirect:
		return nil, nil
	case store.CloudConnectionSystem:
		request, err := http.NewRequest(http.MethodGet, strings.TrimRight(m.client.baseURL, "/")+"/health", nil)
		if err != nil {
			return nil, err
		}
		return cloudProxy(request)
	case store.CloudConnectionProxy:
		selected, err := m.store.GetProxy(*value.ProxyID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, errors.New("selected Amber Cloud proxy is unavailable")
			}
			return nil, err
		}
		return storedProxyURL(selected)
	default:
		return nil, fmt.Errorf("unsupported Amber Cloud connection mode %q", value.Mode)
	}
}

func dialRelayWebSocket(ctx context.Context, config *websocket.Config, proxyURL *url.URL) (*websocket.Conn, error) {
	if config == nil || config.Location == nil {
		return nil, errors.New("invalid owner relay WebSocket configuration")
	}
	if config.Header == nil {
		config.Header = make(http.Header)
	}
	config.Header.Set("User-Agent", amberUserAgent)
	config.Header.Set("X-Amber-Client-Version", amberClientVersion)
	if proxyURL == nil {
		return config.DialContext(ctx)
	}
	connection, err := dialRelayTunnel(ctx, config, proxyURL)
	if err != nil {
		return nil, err
	}
	return newRelayWebSocketClient(ctx, config, connection)
}

func dialRelayTunnel(ctx context.Context, config *websocket.Config, proxyURL *url.URL) (net.Conn, error) {
	target, err := relayAuthority(config.Location)
	if err != nil {
		return nil, err
	}
	var connection net.Conn
	switch strings.ToLower(proxyURL.Scheme) {
	case "socks5":
		proxyAddress, err := proxyAuthority(proxyURL)
		if err != nil {
			return nil, err
		}
		var auth *proxy.Auth
		if proxyURL.User != nil {
			password, _ := proxyURL.User.Password()
			auth = &proxy.Auth{User: proxyURL.User.Username(), Password: password}
		}
		dialer, err := proxy.SOCKS5("tcp", proxyAddress, auth, &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second})
		if err != nil {
			return nil, err
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, errors.New("SOCKS5 proxy does not support cancellable connections")
		}
		connection, err = contextDialer.DialContext(ctx, "tcp", target)
		if err != nil {
			return nil, fmt.Errorf("connect owner relay through SOCKS5 proxy: %w", err)
		}
	case "http", "https":
		connection, err = dialHTTPConnectProxy(ctx, proxyURL, target)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported Amber Cloud proxy type %q", proxyURL.Scheme)
	}

	if config.Location.Scheme == "ws" {
		return connection, nil
	}
	if config.Location.Scheme != "wss" {
		_ = connection.Close()
		return nil, errors.New("invalid owner relay WebSocket scheme")
	}
	tlsConfig := &tls.Config{ServerName: config.Location.Hostname()}
	if config.TlsConfig != nil {
		tlsConfig = config.TlsConfig.Clone()
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = config.Location.Hostname()
		}
	}
	tlsConnection := tls.Client(connection, tlsConfig)
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("owner relay TLS handshake: %w", err)
	}
	return tlsConnection, nil
}

func dialHTTPConnectProxy(ctx context.Context, proxyURL *url.URL, target string) (net.Conn, error) {
	proxyAddress, err := proxyAuthority(proxyURL)
	if err != nil {
		return nil, err
	}
	connection, err := (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, fmt.Errorf("connect Amber Cloud proxy: %w", err)
	}
	deadlineConnection := connection
	stopCancel := context.AfterFunc(ctx, func() { _ = deadlineConnection.SetDeadline(time.Now()) })
	defer stopCancel()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = connection.Close()
		}
	}()
	if strings.EqualFold(proxyURL.Scheme, "https") {
		proxyTLS := tls.Client(connection, &tls.Config{ServerName: proxyURL.Hostname()})
		if err := proxyTLS.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("Amber Cloud proxy TLS handshake: %w", err)
		}
		connection = proxyTLS
	}
	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: make(http.Header),
	}
	request.Header.Set("User-Agent", amberUserAgent)
	request.Header.Set("Proxy-Connection", "Keep-Alive")
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		credentials := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + password))
		request.Header.Set("Proxy-Authorization", "Basic "+credentials)
	}
	if err := request.Write(connection); err != nil {
		return nil, fmt.Errorf("send Amber Cloud proxy CONNECT: %w", err)
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, fmt.Errorf("read Amber Cloud proxy CONNECT response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4*1024))
		_ = response.Body.Close()
		return nil, fmt.Errorf("Amber Cloud proxy CONNECT failed with HTTP %d", response.StatusCode)
	}
	closeOnError = false
	stopCancel()
	_ = connection.SetDeadline(time.Time{})
	return &bufferedRelayConn{Conn: connection, reader: reader}, nil
}

type bufferedRelayConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedRelayConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

func newRelayWebSocketClient(ctx context.Context, config *websocket.Config, connection net.Conn) (*websocket.Conn, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	type result struct {
		conn *websocket.Conn
		err  error
	}
	done := make(chan result, 1)
	go func() {
		conn, err := websocket.NewClient(config, connection)
		done <- result{conn: conn, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = connection.SetDeadline(time.Now())
		<-done
		_ = connection.Close()
		return nil, ctx.Err()
	case outcome := <-done:
		if outcome.err != nil {
			_ = connection.Close()
			return nil, outcome.err
		}
		_ = connection.SetDeadline(time.Time{})
		return outcome.conn, nil
	}
}

func relayAuthority(location *url.URL) (string, error) {
	if location == nil || location.Hostname() == "" {
		return "", errors.New("invalid owner relay WebSocket address")
	}
	if location.Port() != "" {
		return location.Host, nil
	}
	switch location.Scheme {
	case "ws":
		return net.JoinHostPort(location.Hostname(), "80"), nil
	case "wss":
		return net.JoinHostPort(location.Hostname(), "443"), nil
	default:
		return "", errors.New("invalid owner relay WebSocket scheme")
	}
}

func proxyAuthority(proxyURL *url.URL) (string, error) {
	if proxyURL == nil || proxyURL.Hostname() == "" {
		return "", errors.New("Amber Cloud proxy address is invalid")
	}
	if proxyURL.Port() != "" {
		return proxyURL.Host, nil
	}
	switch strings.ToLower(proxyURL.Scheme) {
	case "http":
		return net.JoinHostPort(proxyURL.Hostname(), "80"), nil
	case "https":
		return net.JoinHostPort(proxyURL.Hostname(), "443"), nil
	case "socks5":
		return net.JoinHostPort(proxyURL.Hostname(), "1080"), nil
	default:
		return "", errors.New("Amber Cloud proxy type is unsupported")
	}
}
