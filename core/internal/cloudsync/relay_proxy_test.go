package cloudsync

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestDialRelayWebSocketUsesHTTPConnectProxyAndClientHeaders(t *testing.T) {
	headers := make(chan http.Header, 1)
	target := httptest.NewServer(websocket.Server{
		Handshake: func(_ *websocket.Config, request *http.Request) error {
			headers <- request.Header.Clone()
			return nil
		},
		Handler: func(connection *websocket.Conn) {
			var value string
			if err := websocket.Message.Receive(connection, &value); err == nil {
				_ = websocket.Message.Send(connection, value)
			}
		},
	})
	defer target.Close()

	proxyUsed := make(chan string, 1)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodConnect {
			http.Error(writer, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		client, _, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		upstream, err := net.DialTimeout("tcp", request.Host, 5*time.Second)
		if err != nil {
			_ = client.Close()
			return
		}
		proxyUsed <- request.Host
		_, _ = fmt.Fprint(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
		go func() { _, _ = io.Copy(client, upstream); _ = client.Close() }()
	}))
	defer proxyServer.Close()

	config, err := websocket.NewConfig("ws"+strings.TrimPrefix(target.URL, "http"), target.URL)
	if err != nil {
		t.Fatal(err)
	}
	parsedProxy, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := dialRelayWebSocket(ctx, config, parsedProxy)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := websocket.Message.Send(connection, "relay-ready"); err != nil {
		t.Fatal(err)
	}
	var echoed string
	if err := websocket.Message.Receive(connection, &echoed); err != nil {
		t.Fatal(err)
	}
	if echoed != "relay-ready" {
		t.Fatalf("echo = %q", echoed)
	}
	select {
	case authority := <-proxyUsed:
		if authority != strings.TrimPrefix(target.URL, "http://") {
			t.Fatalf("CONNECT authority = %q", authority)
		}
	case <-ctx.Done():
		t.Fatal("HTTP CONNECT proxy was not used")
	}
	select {
	case requestHeaders := <-headers:
		if requestHeaders.Get("User-Agent") != amberUserAgent || requestHeaders.Get("X-Amber-Client-Version") != amberClientVersion {
			t.Fatalf("missing relay client headers: %#v", requestHeaders)
		}
	case <-ctx.Done():
		t.Fatal("WebSocket handshake headers were not received")
	}
}

func TestHTTPConnectProxyHonorsContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()
	proxyURL := &url.URL{Scheme: "http", Host: listener.Addr().String()}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = dialHTTPConnectProxy(ctx, proxyURL, "example.test:443")
	if err == nil {
		t.Fatal("stalled proxy unexpectedly connected")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("proxy cancellation took too long: %s", time.Since(started))
	}
	select {
	case connection := <-accepted:
		_ = connection.Close()
	default:
	}
}
