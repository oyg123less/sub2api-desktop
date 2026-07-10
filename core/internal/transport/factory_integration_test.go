package transport

import (
	"bufio"
	"crypto/x509"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"sub2api-desktop/core/internal/store"
)

func TestHTTPAndHTTPSConnectProxies(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()

	httpProxy := httptest.NewServer(http.HandlerFunc(connectProxyHandler))
	defer httpProxy.Close()
	httpsProxy := httptest.NewTLSServer(http.HandlerFunc(connectProxyHandler))
	defer httpsProxy.Close()

	roots := x509.NewCertPool()
	roots.AddCert(target.Certificate())
	roots.AddCert(httpsProxy.Certificate())

	for _, test := range []struct {
		name      string
		proxyURL  string
		proxyType store.ProxyType
	}{
		{name: "http connect", proxyURL: httpProxy.URL, proxyType: store.ProxyHTTP},
		{name: "tls connect", proxyURL: httpsProxy.URL, proxyType: store.ProxyHTTPS},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxy := proxyRecord(t, test.proxyURL, test.proxyType)
			client, err := NewClient(Options{Proxy: proxy, Purpose: PurposeGateway, FingerprintProfile: "standard", Timeout: 5 * time.Second, RootCAs: roots})
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Get(target.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, _ := io.ReadAll(response.Body)
			if response.StatusCode != http.StatusOK || string(data) != "ok" {
				t.Fatalf("status=%d body=%q", response.StatusCode, data)
			}
		})
	}
}

func TestSOCKS5Proxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "via-socks")
	}))
	defer target.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go serveSOCKS5(listener)

	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	client, err := NewClient(Options{Proxy: &store.Proxy{Type: store.ProxySOCKS5, Host: host, Port: port}, Purpose: PurposeGateway, FingerprintProfile: "standard", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if string(data) != "via-socks" {
		t.Fatalf("body=%q", data)
	}
}

func connectProxyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
		return
	}
	upstream, err := net.DialTimeout("tcp", r.Host, 3*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}
	client, buffer, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	_ = buffer.Flush()
	go tunnel(client, upstream)
	go tunnel(upstream, client)
}

func tunnel(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	_ = src.Close()
}

func proxyRecord(t *testing.T, raw string, kind store.ProxyType) *store.Proxy {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	return &store.Proxy{Type: kind, Host: host, Port: port}
}

func serveSOCKS5(listener net.Listener) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go handleSOCKS5(connection)
	}
}

func handleSOCKS5(client net.Conn) {
	defer client.Close()
	reader := bufio.NewReader(client)
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil || header[0] != 5 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return
	}
	_, _ = client.Write([]byte{5, 0})
	request := make([]byte, 4)
	if _, err := io.ReadFull(reader, request); err != nil || request[1] != 1 {
		return
	}
	var host string
	switch request[3] {
	case 1:
		value := make([]byte, 4)
		_, _ = io.ReadFull(reader, value)
		host = net.IP(value).String()
	case 3:
		length, _ := reader.ReadByte()
		value := make([]byte, int(length))
		_, _ = io.ReadFull(reader, value)
		host = string(value)
	case 4:
		value := make([]byte, 16)
		_, _ = io.ReadFull(reader, value)
		host = net.IP(value).String()
	default:
		return
	}
	portBytes := make([]byte, 2)
	_, _ = io.ReadFull(reader, portBytes)
	address := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes))))
	upstream, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		_, _ = client.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	_, _ = client.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	done := make(chan struct{}, 1)
	go func() { _, _ = io.Copy(upstream, reader); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
	<-done
}
