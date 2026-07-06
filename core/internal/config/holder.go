// Package config holds the live application settings, backed by the store.
package config

import (
	"sync"

	"sub2api-desktop/core/internal/store"
)

// Holder is a concurrency-safe live settings container that persists changes.
type Holder struct {
	mu      sync.RWMutex
	current store.Settings
	store   *store.Store
}

// NewHolder loads settings from the store and returns a holder.
func NewHolder(s *store.Store) (*Holder, error) {
	cfg, err := s.LoadSettings()
	if err != nil {
		return nil, err
	}
	return &Holder{current: cfg, store: s}, nil
}

// Get returns a copy of the current settings.
func (h *Holder) Get() store.Settings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

// Save persists and swaps in new settings.
func (h *Holder) Save(v store.Settings) error {
	if err := h.store.SaveSettings(v); err != nil {
		return err
	}
	h.mu.Lock()
	h.current = v
	h.mu.Unlock()
	return nil
}
