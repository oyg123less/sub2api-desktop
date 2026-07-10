package account

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	appcrypto "sub2api-desktop/core/internal/crypto"
	"sub2api-desktop/core/internal/store"
)

const (
	accessA = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhIn0.aaaaaaaa"
	accessB = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJiIn0.bbbbbbbb"
	idA     = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJpZGEifQ.aaaaaaaa"
	idB     = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJpZGIifQ.bbbbbbbb"
)

type fakeIdentityVerifier map[string]VerifiedIdentity

func (v fakeIdentityVerifier) VerifyIDToken(_ context.Context, token string) (VerifiedIdentity, error) {
	identity, ok := v[token]
	if !ok {
		return VerifiedIdentity{}, errors.New("unverified token")
	}
	return identity, nil
}

func TestImportConflictingVerifiedIdentityDoesNotWritePartialBatch(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{
		idA: {Email: "same@example.com", ChatGPTAccountID: "acct_same", Level: IdentitySigned},
		idB: {Email: "same@example.com", ChatGPTAccountID: "acct_same", Level: IdentitySigned},
	})
	raw := mustImportJSON(t, []ImportEntry{{AccessToken: accessA, IDToken: idA}, {AccessToken: accessB, IDToken: idB}})
	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Create != 1 || preview.Summary.Conflict != 1 {
		t.Fatalf("unexpected preview summary: %#v", preview.Summary)
	}
	if _, err := manager.CommitImport(context.Background(), raw, preview.ContentSHA256, false); err == nil {
		t.Fatal("conflicting batch was committed")
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 0 {
		t.Fatalf("partial batch was written: %d accounts", len(accounts))
	}
}

func TestImportTwoDistinctVerifiedAccountsCommitsBoth(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{
		idA: {Email: "a@example.com", ChatGPTAccountID: "acct_a", Level: IdentitySigned},
		idB: {Email: "b@example.com", ChatGPTAccountID: "acct_b", Level: IdentitySigned},
	})
	raw := mustImportJSON(t, []ImportEntry{{AccessToken: accessA, IDToken: idA}, {AccessToken: accessB, IDToken: idB}})
	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.CommitImport(context.Background(), raw, preview.ContentSHA256, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 {
		t.Fatalf("imported = %d, want 2", result.Imported)
	}
	accounts, _ := st.ListAccounts()
	if len(accounts) != 2 {
		t.Fatalf("account count = %d, want 2", len(accounts))
	}
}

func TestAmbiguousSharedAccountIDDoesNotMergeDistinctCredentials(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{})
	raw := []byte(`[{"account_id":"shared","access_token":"` + accessA + `"},{"account_id":"shared","access_token":"` + accessB + `"}]`)
	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Create != 2 || preview.Summary.Conflict != 0 {
		t.Fatalf("ambiguous account IDs were merged: %#v", preview.Summary)
	}
	if _, err := manager.CommitImport(context.Background(), raw, preview.ContentSHA256, false); err != nil {
		t.Fatal(err)
	}
	accounts, _ := st.ListAccounts()
	if len(accounts) != 2 {
		t.Fatalf("account count = %d, want 2", len(accounts))
	}
	if accounts[0].Status != store.AccountPending || accounts[1].Status != store.AccountPending {
		t.Fatalf("unverified accounts were not pending: %s, %s", accounts[0].Status, accounts[1].Status)
	}
}

func TestImportUpdatePreservesMissingTokens(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{
		idA: {Email: "new@example.com", ChatGPTAccountID: "acct_existing", PlanType: "pro", Level: IdentitySigned},
	})
	existing, err := st.CreateAccount(&store.Account{
		Email: "old@example.com", ChatGPTAccountID: "acct_existing", PlanType: "plus",
		AccessToken: accessA, RefreshToken: "refresh_old_12345678901234567890", IDToken: idB, Status: store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := mustImportJSON(t, []ImportEntry{{RefreshToken: "refresh_new_12345678901234567890", IDToken: idA}})
	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Update != 1 || preview.Rows[0].MatchedAccountID != existing.ID {
		t.Fatalf("existing account was not matched: %#v", preview)
	}
	if _, err := manager.CommitImport(context.Background(), raw, preview.ContentSHA256, false); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetAccount(existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AccessToken != accessA || updated.IDToken != idA || updated.RefreshToken != "refresh_new_12345678901234567890" {
		t.Fatalf("missing token merge failed: access=%q refresh=%q id=%q", updated.AccessToken, updated.RefreshToken, updated.IDToken)
	}
	if updated.Email != "new@example.com" || updated.PlanType != "pro" {
		t.Fatalf("verified metadata was not updated: %#v", updated)
	}
}

func newImportTestManager(t *testing.T, verifier IdentityVerifier) (*Manager, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	cipher, err := appcrypto.LoadOrCreate(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "sub2api.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	manager := NewManager(st)
	manager.identityVerifier = verifier
	return manager, st
}

func mustImportJSON(t *testing.T, entries []ImportEntry) []byte {
	t.Helper()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
