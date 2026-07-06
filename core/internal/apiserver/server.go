// Package apiserver exposes the local OpenAI-compatible API (/v1/*) that
// clients like Cherry Studio, Cursor and ChatBox connect to.
package apiserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/openai"
	"sub2api-desktop/core/internal/store"
)

// Handler builds the /v1 API mux.
type Handler struct {
	engine   *gateway.Engine
	settings func() store.Settings
}

// New creates the API handler.
func New(engine *gateway.Engine, settings func() store.Settings) *Handler {
	return &Handler{engine: engine, settings: settings}
}

// Mount registers routes on the given mux under the API surface.
func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /v1/models", h.auth(h.models))
	mux.HandleFunc("POST /v1/chat/completions", h.auth(h.engine.ChatCompletions))
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().Unix()})
}

func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := h.settings().LocalAPIKey
		if key == "" { // safety: never run fully open
			writeErr(w, http.StatusServiceUnavailable, "local api key not configured")
			return
		}
		provided := extractBearer(r.Header.Get("Authorization"))
		if provided == "" {
			provided = r.Header.Get("x-api-key")
		}
		if provided != key {
			writeErr(w, http.StatusUnauthorized, "invalid api key")
			return
		}
		next(w, r)
	}
}

func extractBearer(h string) string {
	if h == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(h)
}

func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	data := make([]map[string]any, 0, len(openai.DefaultModels))
	for _, m := range openai.DefaultModels {
		if strings.HasPrefix(m.ID, "gpt-image") {
			continue // image models not supported by this gateway
		}
		data = append(data, map[string]any{
			"id":       m.ID,
			"object":   "model",
			"created":  m.Created,
			"owned_by": m.OwnedBy,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": msg, "type": "invalid_request_error"}})
}
