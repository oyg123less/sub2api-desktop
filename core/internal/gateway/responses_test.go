package gateway_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

type failingStreamWriter struct {
	header http.Header
	writes int
	failAt int
}

func (w *failingStreamWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingStreamWriter) WriteHeader(int) {}
func (w *failingStreamWriter) Flush()          {}
func (w *failingStreamWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, errors.New("client connection closed")
	}
	return len(data), nil
}

func TestResponsesPassthrough(t *testing.T) {
	var captured http.Header
	upstream := httptest.NewServer(mockUpstreamSSE(t, &captured))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)

	mgr := account.NewManager(st)
	engine := gateway.New(st, mgr, func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	reqBody := `{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	engine.Responses(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	out := w.Body.String()
	if !strings.Contains(out, "response.created") || !strings.Contains(out, "response.completed") {
		t.Errorf("expected verbatim responses SSE, got: %s", out)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("missing content in stream: %s", out)
	}
	if captured.Get("originator") != "codex_cli_rs" {
		t.Errorf("expected originator codex_cli_rs, got %q", captured.Get("originator"))
	}
	if captured.Get("chatgpt-account-id") != "acc-123" {
		t.Errorf("expected chatgpt-account-id header")
	}
}

func TestResponsesNonStreamingAggregatesSSE(t *testing.T) {
	upstream := httptest.NewServer(mockUpstreamSSE(t, nil))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","input":"hi"}`))
	w := httptest.NewRecorder()
	engine.Responses(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content type = %q", got)
	}
	var response struct {
		Status string `json:"status"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "completed" || len(response.Output) != 1 || response.Output[0].Content[0].Text != "Hello, world" {
		t.Fatalf("unexpected aggregate: %+v", response)
	}
}

func TestResponsesTerminalFailures(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		wantKind string
	}{
		{
			name: "failed event",
			lines: []string{
				`data: {"type":"response.created","response":{"id":"r"}}`,
				`data: {"type":"response.failed","response":{"id":"r","status":"failed","error":{"code":"server_error","message":"generation failed"}}}`,
			},
			wantKind: "upstream_failed_event",
		},
		{
			name: "incomplete content filter",
			lines: []string{
				`data: {"type":"response.created","response":{"id":"r"}}`,
				`data: {"type":"response.incomplete","response":{"id":"r","status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`,
			},
			wantKind: "upstream_failed_event",
		},
		{
			name: "missing terminal",
			lines: []string{
				`data: {"type":"response.created","response":{"id":"r"}}`,
				`data: {"type":"response.output_text.delta","delta":"partial"}`,
			},
			wantKind: "upstream_stream_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, line := range tt.lines {
					_, _ = io.WriteString(w, line+"\n\n")
				}
			}))
			defer upstream.Close()
			t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)

			st := newTestStore(t)
			seedAccount(t, st)
			cfg, _ := st.LoadSettings()
			cfg.TLSFingerprint = false
			_ = st.SaveSettings(cfg)
			engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)

			r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","input":"hi"}`))
			w := httptest.NewRecorder()
			engine.Responses(w, r)
			if w.Code != http.StatusBadGateway {
				t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantKind) {
				t.Fatalf("body does not contain %q: %s", tt.wantKind, w.Body.String())
			}
			logs, err := st.RecentLogs(1)
			if err != nil || len(logs) != 1 {
				t.Fatalf("logs: %v %#v", err, logs)
			}
			if logs[0].StatusCode != http.StatusBadGateway || logs[0].ErrorKind != tt.wantKind {
				t.Fatalf("unexpected log: %+v", logs[0])
			}
		})
	}
}

func TestResponsesScannerErrorIsNotSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+strings.Repeat("x", 9<<20)+"\n\n")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)
	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","input":"hi"}`))
	w := httptest.NewRecorder()
	engine.Responses(w, r)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "upstream_stream_error") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	logs, _ := st.RecentLogs(1)
	if len(logs) != 1 || logs[0].TerminalEvent != "scanner_error" {
		t.Fatalf("unexpected log: %+v", logs)
	}
}

func TestCancelledResponsesStreamPersistsLatestUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range []string{
			`data: {"type":"response.created","response":{"id":"r"}}`,
			`data: {"type":"response.output_text.delta","delta":"hello"}`,
			`data: {"type":"response.in_progress","usage":{"input_tokens":12,"output_tokens":7,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":4}}}`,
		} {
			_, _ = io.WriteString(w, line+"\n\n")
		}
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)
	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"hi"}`))
	writer := &failingStreamWriter{failAt: 5}
	engine.Responses(writer, request)
	logs, err := st.RecentLogs(1)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs=%#v err=%v", logs, err)
	}
	log := logs[0]
	if log.StatusCode != 499 || log.ErrorKind != "client_cancelled" || log.PromptTokens != 12 || log.CachedTokens != 3 || log.CompletionTokens != 7 || log.ReasoningTokens != 4 || log.Estimated {
		t.Fatalf("unexpected cancellation log: %+v", log)
	}
}

func TestCancelledResponsesStreamEstimatesUsageWithoutUpstreamUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"type":"response.created","response":{"id":"r"}}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"hello"}`+"\n\n")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_UPSTREAM_URL", upstream.URL)
	st := newTestStore(t)
	seedAccount(t, st)
	cfg, _ := st.LoadSettings()
	cfg.TLSFingerprint = false
	_ = st.SaveSettings(cfg)
	engine := gateway.New(st, account.NewManager(st), func() store.Settings { s, _ := st.LoadSettings(); return s }, nil)
	body := `{"model":"gpt-5.5","stream":true,"input":"hi"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	writer := &failingStreamWriter{failAt: 4}
	engine.Responses(writer, request)
	logs, err := st.RecentLogs(1)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs=%#v err=%v", logs, err)
	}
	log := logs[0]
	wantPrompt := (len([]byte(body)) + 3) / 4
	if log.StatusCode != 499 || !log.Estimated || log.PromptTokens != wantPrompt || log.CompletionTokens != 2 {
		t.Fatalf("unexpected estimated cancellation log: %+v", log)
	}
}
