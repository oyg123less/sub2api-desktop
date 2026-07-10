package gateway_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

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
