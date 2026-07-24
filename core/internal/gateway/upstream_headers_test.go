package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sub2api-desktop/core/internal/store"
)

func TestUpstreamClientHeadersRequireStrictOfficialIdentity(t *testing.T) {
	cfg := store.DefaultSettings()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Header.Set("User-Agent", "third-party/1.0 codex_cli_rs/0.146.0")
	request.Header.Set("originator", "third-party")
	request.Header.Set("OpenAI-Beta", "untrusted-beta")
	request.Header.Set("session_id", "untrusted-session")

	headers := upstreamClientHeadersFromRequest(request, cfg)
	if headers.UserAgent != store.DefaultUserAgent || headers.Originator != "codex_cli_rs" {
		t.Fatalf("non-official identity crossed the gateway boundary: %#v", headers)
	}
	if headers.OpenAIBeta != "responses=experimental" || headers.SessionID == "untrusted-session" {
		t.Fatalf("non-official client metadata crossed the gateway boundary: %#v", headers)
	}
}

func TestUpstreamClientHeadersRecognizeOfficialTrailer(t *testing.T) {
	cfg := store.DefaultSettings()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Header.Set("User-Agent", "custom/0.146.0 (Windows 11; x86_64) (codex-tui; 0.146.0)")
	request.Header.Set("originator", "custom")
	request.Header.Set("OpenAI-Beta", "responses=stable")
	request.Header.Set("session_id", "official-session")

	headers := upstreamClientHeadersFromRequest(request, cfg)
	if headers.UserAgent != request.UserAgent() || headers.OpenAIBeta != "responses=stable" || headers.SessionID != "official-session" {
		t.Fatalf("official Codex trailer identity was not preserved: %#v", headers)
	}
}
