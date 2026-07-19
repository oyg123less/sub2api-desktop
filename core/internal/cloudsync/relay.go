package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"sub2api-desktop/core/internal/store"
)

// RelayHandler runs a request through the existing local gateway while
// constraining account selection to the supplied cloud account UID.
type RelayHandler func(context.Context, http.ResponseWriter, *http.Request, string, func())

type relayChallenge struct {
	Challenge string `json:"challenge"`
	ExpiresAt string `json:"expires_at"`
}

type relayMessage struct {
	Protocol        int               `json:"protocol"`
	Type            string            `json:"type"`
	RequestID       string            `json:"request_id,omitempty"`
	GroupID         string            `json:"group_id,omitempty"`
	AccountUID      string            `json:"account_uid,omitempty"`
	Endpoint        string            `json:"endpoint,omitempty"`
	Model           string            `json:"model,omitempty"`
	Accept          string            `json:"accept,omitempty"`
	Body            string            `json:"body,omitempty"`
	Status          int               `json:"status,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Data            string            `json:"data,omitempty"`
	ErrorCode       string            `json:"error_code,omitempty"`
	Message         string            `json:"message,omitempty"`
	UpstreamStarted bool              `json:"upstream_started,omitempty"`
}

type relaySession struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func (c *cloudClient) relayChallenge(ctx context.Context, accessToken, deviceID string) (relayChallenge, error) {
	var response relayChallenge
	err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/devices/%s/challenge", url.PathEscape(deviceID)), accessToken, nil, &response)
	return response, err
}

func (m *Manager) SetRelayHandler(handler RelayHandler) {
	m.relayMu.Lock()
	m.relayHandler = handler
	m.relayMu.Unlock()
}

func (m *Manager) closeRelaySession() {
	m.relayMu.Lock()
	closer := m.relayCloser
	m.relayCloser = nil
	m.relayMu.Unlock()
	if closer != nil {
		_ = closer.Close()
	}
}

func (m *Manager) runRelay(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		m.mu.RLock()
		session := m.session
		m.mu.RUnlock()
		if session == nil {
			if !waitRelay(ctx, 2*time.Second) {
				return
			}
			continue
		}
		identity, err := m.store.LoadCloudIdentity(session.UserID)
		m.relayMu.Lock()
		handlerReady := m.relayHandler != nil
		m.relayMu.Unlock()
		if err != nil || !identity.RelayEnabled || identity.DevicePublicID == "" || !handlerReady {
			if !waitRelay(ctx, 2*time.Second) {
				return
			}
			continue
		}
		if err := m.connectRelay(ctx, *identity); err != nil && ctx.Err() == nil {
			m.logger.Warn("owner relay disconnected", "error_type", fmt.Sprintf("%T", err))
			if !waitRelay(ctx, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func waitRelay(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (m *Manager) connectRelay(ctx context.Context, identity store.CloudIdentity) error {
	m.opMu.Lock()
	accessToken, userID, vaultKey, err := m.cloudV2Access(ctx)
	if len(vaultKey) > 0 {
		wipe(vaultKey)
	}
	if err == nil && userID != identity.UserID {
		err = errors.New("cloud identity changed")
	}
	var challenge relayChallenge
	if err == nil {
		challenge, err = m.client.relayChallenge(ctx, accessToken, identity.DevicePublicID)
	}
	m.opMu.Unlock()
	if err != nil {
		return err
	}
	proof, err := signRelayChallenge(identity.DevicePrivateKey, identity.DevicePublicID, challenge.Challenge, challenge.ExpiresAt)
	if err != nil {
		return err
	}
	base, err := url.Parse(m.client.baseURL)
	if err != nil {
		return err
	}
	if base.Scheme == "https" {
		base.Scheme = "wss"
	} else {
		base.Scheme = "ws"
	}
	base.Path = "/v1/relay/connect"
	query := base.Query()
	query.Set("device_id", identity.DevicePublicID)
	query.Set("protocol", "1")
	base.RawQuery = query.Encode()
	config, err := websocket.NewConfig(base.String(), m.client.baseURL)
	if err != nil {
		return err
	}
	config.Header.Set("Authorization", "Bearer "+accessToken)
	config.Header.Set("X-Amber-Device-Challenge", challenge.Challenge)
	config.Header.Set("X-Amber-Device-Challenge-Expires", challenge.ExpiresAt)
	config.Header.Set("X-Amber-Device-Proof", proof)
	proxyURL, err := m.relayProxyURL()
	if err != nil {
		return err
	}
	conn, err := dialRelayWebSocket(ctx, config, proxyURL)
	if err != nil {
		return err
	}
	session := &relaySession{conn: conn, cancels: make(map[string]context.CancelFunc)}
	m.relayMu.Lock()
	if m.relayCloser != nil {
		_ = m.relayCloser.Close()
	}
	m.relayCloser = conn
	m.relayMu.Unlock()
	defer func() {
		session.cancelAll()
		_ = conn.Close()
		m.relayMu.Lock()
		if m.relayCloser == conn {
			m.relayCloser = nil
		}
		m.relayMu.Unlock()
	}()
	if err := session.send(relayMessage{Protocol: 1, Type: "hello"}); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if session.send(relayMessage{Protocol: 1, Type: "heartbeat"}) != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	go func() { <-ctx.Done(); _ = conn.Close() }()
	for {
		var raw string
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			return err
		}
		if len(raw) > 128*1024 {
			return errors.New("owner relay message is too large")
		}
		var message relayMessage
		if err := json.Unmarshal([]byte(raw), &message); err != nil || message.Protocol != 1 {
			continue
		}
		switch message.Type {
		case "relay_request":
			go m.handleRelayRequest(ctx, session, message)
		case "cancel_request":
			session.cancel(message.RequestID)
		case "hello_ack", "heartbeat_ack", "chunk_ack":
			// Control acknowledgements do not require an Agent response.
		}
		select {
		case <-done:
			return ctx.Err()
		default:
		}
	}
}

func (s *relaySession) send(message relayMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return websocket.JSON.Send(s.conn, message)
}

func (s *relaySession) register(requestID string, cancel context.CancelFunc) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if requestID == "" || s.cancels[requestID] != nil {
		return false
	}
	s.cancels[requestID] = cancel
	return true
}

func (s *relaySession) unregister(requestID string) {
	s.mu.Lock()
	delete(s.cancels, requestID)
	s.mu.Unlock()
}

func (s *relaySession) cancel(requestID string) {
	s.mu.Lock()
	cancel := s.cancels[requestID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *relaySession) cancelAll() {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for _, cancel := range s.cancels {
		cancels = append(cancels, cancel)
	}
	s.cancels = make(map[string]context.CancelFunc)
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (m *Manager) handleRelayRequest(parent context.Context, session *relaySession, message relayMessage) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	if !session.register(message.RequestID, cancel) {
		cancel()
		_ = session.send(relayMessage{Protocol: 1, Type: "relay_error", RequestID: message.RequestID, Status: 400, ErrorCode: "invalid_relay_request", Message: "The relay request is invalid."})
		return
	}
	defer func() { cancel(); session.unregister(message.RequestID) }()
	if message.AccountUID == "" || (message.Endpoint != "responses" && message.Endpoint != "chat/completions") {
		_ = session.send(relayMessage{Protocol: 1, Type: "relay_error", RequestID: message.RequestID, Status: 400, ErrorCode: "invalid_relay_request", Message: "The relay request is invalid."})
		return
	}
	body, err := decodeRawURL(message.Body, 0)
	if err != nil || len(body) > 4*1024*1024 {
		_ = session.send(relayMessage{Protocol: 1, Type: "relay_error", RequestID: message.RequestID, Status: 413, ErrorCode: "request_too_large", Message: "The relay request body is invalid."})
		return
	}
	m.relayMu.Lock()
	handler := m.relayHandler
	m.relayMu.Unlock()
	if handler == nil {
		_ = session.send(relayMessage{Protocol: 1, Type: "relay_error", RequestID: message.RequestID, Status: 503, ErrorCode: "owner_relay_unavailable", Message: "The local relay is unavailable."})
		return
	}
	if err := session.send(relayMessage{Protocol: 1, Type: "relay_accepted", RequestID: message.RequestID}); err != nil {
		return
	}
	path := "/v1/" + message.Endpoint
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://amber-relay.local"+path, bytes.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", message.Accept)
	request.Header.Set("X-Request-ID", message.RequestID)
	writer := &relayResponseWriter{session: session, requestID: message.RequestID, header: make(http.Header)}
	var started sync.Once
	handler(ctx, writer, request, message.AccountUID, func() {
		started.Do(func() {
			writer.upstreamStarted = true
			_ = session.send(relayMessage{Protocol: 1, Type: "upstream_started", RequestID: message.RequestID})
		})
	})
	if ctx.Err() != nil && !writer.wroteHeader {
		_ = session.send(relayMessage{Protocol: 1, Type: "relay_error", RequestID: message.RequestID, Status: 504,
			ErrorCode: "relay_timeout", Message: "The local relay request timed out.", UpstreamStarted: writer.upstreamStarted})
		return
	}
	writer.finish()
}

type relayResponseWriter struct {
	session         *relaySession
	requestID       string
	header          http.Header
	status          int
	wroteHeader     bool
	upstreamStarted bool
	failed          bool
}

func (w *relayResponseWriter) Header() http.Header { return w.header }

func (w *relayResponseWriter) WriteHeader(status int) {
	if w.wroteHeader || w.failed {
		return
	}
	w.wroteHeader = true
	w.status = status
	headers := make(map[string]string)
	for _, name := range []string{"Content-Type", "OpenAI-Processing-Ms", "X-Request-Id", "X-Ratelimit-Limit-Requests", "X-Ratelimit-Remaining-Requests", "X-Ratelimit-Reset-Requests"} {
		if value := w.header.Get(name); value != "" {
			headers[strings.ToLower(name)] = value
		}
	}
	if err := w.session.send(relayMessage{Protocol: 1, Type: "response_start", RequestID: w.requestID, Status: status, Headers: headers}); err != nil {
		w.failed = true
	}
}

func (w *relayResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.failed {
		return 0, io.ErrClosedPipe
	}
	for offset := 0; offset < len(data); offset += 64 * 1024 {
		end := offset + 64*1024
		if end > len(data) {
			end = len(data)
		}
		if err := w.session.send(relayMessage{Protocol: 1, Type: "response_chunk", RequestID: w.requestID, Data: rawURL(data[offset:end])}); err != nil {
			w.failed = true
			return offset, err
		}
	}
	return len(data), nil
}

func (w *relayResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
}

func (w *relayResponseWriter) finish() {
	if w.failed {
		return
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if !w.failed {
		_ = w.session.send(relayMessage{Protocol: 1, Type: "response_end", RequestID: w.requestID})
	}
}
