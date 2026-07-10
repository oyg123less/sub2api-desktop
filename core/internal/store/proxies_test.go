package store

import (
	"path/filepath"
	"testing"

	appcrypto "sub2api-desktop/core/internal/crypto"
)

func TestProxyPatchPasswordSemantics(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created, err := s.CreateProxy(&Proxy{Name: "proxy", Type: ProxyHTTP, Host: "127.0.0.1", Port: 8080, Username: "user", Password: "original"})
	if err != nil {
		t.Fatal(err)
	}

	empty := ""
	for name, patch := range map[string]ProxyPatch{
		"omitted":         {},
		"null equivalent": {Password: nil},
		"empty":           {Password: &empty},
	} {
		t.Run(name, func(t *testing.T) {
			updated, err := s.UpdateProxyPatch(created.ID, patch)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Password != "original" {
				t.Fatalf("password = %q", updated.Password)
			}
		})
	}

	replacement := "replacement"
	updated, err := s.UpdateProxyPatch(created.ID, ProxyPatch{Password: &replacement})
	if err != nil || updated.Password != replacement {
		t.Fatalf("replace: password=%q err=%v", updated.Password, err)
	}
	updated, err = s.UpdateProxyPatch(created.ID, ProxyPatch{ClearPassword: true})
	if err != nil || updated.Password != "" {
		t.Fatalf("clear: password=%q err=%v", updated.Password, err)
	}
}
