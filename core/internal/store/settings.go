package store

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
)

// DefaultUserAgent mimics the official Codex CLI client.
const DefaultUserAgent = "codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color"

// DefaultSettings returns the built-in defaults (used on first run).
func DefaultSettings() Settings {
	return Settings{
		ListenPort:         8080,
		AllowLAN:           false,
		LocalAPIKey:        "",
		InjectInstr:        true,
		DefaultModel:       "gpt-5.4",
		UserAgent:          DefaultUserAgent,
		Originator:         "codex_cli_rs",
		Language:           "zh-CN",
		AutoStartServer:    false,
		TLSFingerprint:     true,
		RejectUnknownModel: false,
	}
}

// GenerateLocalAPIKey returns a fresh local API key (sk-local-...).
func GenerateLocalAPIKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "sk-local-" + hex.EncodeToString(b)
}

func (s *Store) getKV(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return "", false, nil
		}
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) setKV(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// LoadSettings returns settings, seeding defaults (and a fresh API key) on first run.
func (s *Store) LoadSettings() (Settings, error) {
	def := DefaultSettings()
	out := def

	get := func(key string) (string, error) {
		v, ok, err := s.getKV(key)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", nil
		}
		return v, nil
	}

	if v, err := get("listen_port"); err != nil {
		return out, err
	} else if v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			out.ListenPort = n
		}
	}
	if v, err := get("allow_lan"); err != nil {
		return out, err
	} else if v != "" {
		out.AllowLAN = v == "1"
	}
	if v, err := get("local_api_key"); err != nil {
		return out, err
	} else if v != "" {
		out.LocalAPIKey = v
	}
	if v, err := get("inject_instructions"); err != nil {
		return out, err
	} else if v != "" {
		out.InjectInstr = v == "1"
	}
	if v, err := get("default_model"); err != nil {
		return out, err
	} else if v != "" {
		out.DefaultModel = v
	}
	if v, err := get("user_agent"); err != nil {
		return out, err
	} else if v != "" {
		out.UserAgent = v
	}
	if v, err := get("originator"); err != nil {
		return out, err
	} else if v != "" {
		out.Originator = v
	}
	if v, err := get("language"); err != nil {
		return out, err
	} else if v != "" {
		out.Language = v
	}
	if v, err := get("auto_start_server"); err != nil {
		return out, err
	} else if v != "" {
		out.AutoStartServer = v == "1"
	}
	if v, err := get("tls_fingerprint"); err != nil {
		return out, err
	} else if v != "" {
		out.TLSFingerprint = v == "1"
	}
	if v, err := get("reject_unknown_model"); err != nil {
		return out, err
	} else if v != "" {
		out.RejectUnknownModel = v == "1"
	}

	// Seed a local API key on first run.
	if out.LocalAPIKey == "" {
		out.LocalAPIKey = GenerateLocalAPIKey()
		if err := s.setKV("local_api_key", out.LocalAPIKey); err != nil {
			return out, err
		}
	}
	return out, nil
}

// SaveSettings persists all settings fields.
func (s *Store) SaveSettings(v Settings) error {
	b2s := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	kv := map[string]string{
		"listen_port":          strconv.Itoa(v.ListenPort),
		"allow_lan":            b2s(v.AllowLAN),
		"local_api_key":        v.LocalAPIKey,
		"inject_instructions":  b2s(v.InjectInstr),
		"default_model":        v.DefaultModel,
		"user_agent":           v.UserAgent,
		"originator":           v.Originator,
		"language":             v.Language,
		"auto_start_server":    b2s(v.AutoStartServer),
		"tls_fingerprint":      b2s(v.TLSFingerprint),
		"reject_unknown_model": b2s(v.RejectUnknownModel),
	}
	for k, val := range kv {
		if err := s.setKV(k, val); err != nil {
			return err
		}
	}
	return nil
}
