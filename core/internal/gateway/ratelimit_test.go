package gateway

import (
	"bytes"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"sub2api-desktop/core/internal/account"
	appcrypto "sub2api-desktop/core/internal/crypto"
	"sub2api-desktop/core/internal/store"
)

func TestCaptureCodexUsageWarnsWhenPersistenceFails(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "db.sqlite"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	engine := New(st, account.NewManager(st), func() store.Settings { return store.Settings{} }, logger)
	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "25")

	usage := engine.captureCodexUsage(&store.Account{ID: 42, AccountType: store.AccountTypeOAuth}, headers)

	if usage == nil {
		t.Fatal("usage parsing changed after persistence failure")
	}
	output := logs.String()
	if !strings.Contains(output, "persist Codex usage snapshot failed") || !strings.Contains(output, "account_id=42") {
		t.Fatalf("missing persistence warning: %s", output)
	}
}
