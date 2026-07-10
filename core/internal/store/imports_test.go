package store

import (
	"path/filepath"
	"testing"

	appcrypto "sub2api-desktop/core/internal/crypto"
)

func TestApplyAccountImportsRollsBackWholeBatch(t *testing.T) {
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_, err = st.ApplyAccountImports([]AccountImportMutation{
		{Index: 1, ChatGPTAccountID: "acct_duplicate", AccessToken: "first", IdentityVerified: true},
		{Index: 2, ChatGPTAccountID: "acct_duplicate", AccessToken: "second", IdentityVerified: true},
	})
	if err == nil {
		t.Fatal("duplicate batch unexpectedly committed")
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 0 {
		t.Fatalf("transaction left %d partial accounts", len(accounts))
	}
}
