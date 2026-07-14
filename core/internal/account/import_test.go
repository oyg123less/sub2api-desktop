package account

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
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

type countingIdentityVerifier struct {
	calls int
}

func (v *countingIdentityVerifier) VerifyIDToken(_ context.Context, _ string) (VerifiedIdentity, error) {
	v.calls++
	return VerifiedIdentity{Email: "cached@example.com", ChatGPTAccountID: "acct_cached", Level: IdentitySigned}, nil
}

func TestCommitImportReusesCachedPreviewPlan(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{})
	verifier := &countingIdentityVerifier{}
	manager.identityVerifier = verifier
	raw := mustImportJSON(t, []ImportEntry{{AccessToken: accessA, IDToken: idA}})

	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if verifier.calls != 1 {
		t.Fatalf("identity verifier calls after preview = %d, want 1", verifier.calls)
	}
	result, err := manager.CommitImport(context.Background(), raw, preview.ContentSHA256, false)
	if err != nil {
		t.Fatal(err)
	}
	if verifier.calls != 1 {
		t.Fatalf("identity verifier calls after cached commit = %d, want 1", verifier.calls)
	}
	if result.Imported != 1 {
		t.Fatalf("imported = %d, want 1", result.Imported)
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].ChatGPTAccountID != "acct_cached" {
		t.Fatal("cached preview plan was not committed")
	}
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

func TestImportPersistsDecodedUnverifiedIdentityForForwarding(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{})
	idToken := unsignedIdentityToken(t, "decoded@example.com", "acct_decoded", "plus")
	raw := mustImportJSON(t, []ImportEntry{{AccessToken: accessA, IDToken: idToken}})

	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Rows[0].IdentityVerified {
		t.Fatal("decoded identity was unexpectedly marked verified")
	}
	if _, err := manager.CommitImport(context.Background(), raw, preview.ContentSHA256, false); err != nil {
		t.Fatal(err)
	}

	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("account count = %d, want 1", len(accounts))
	}
	account := accounts[0]
	if account.ChatGPTAccountID != "acct_decoded" || account.Email != "decoded@example.com" || account.PlanType != "plus" {
		t.Fatalf("decoded forwarding identity was not persisted: %#v", account)
	}
}

func TestImportSameRefreshTokenDifferentVerifiedIdentitiesConflicts(t *testing.T) {
	manager, _ := newImportTestManager(t, fakeIdentityVerifier{
		idA: {ChatGPTAccountID: "acct_a", Level: IdentitySigned},
		idB: {ChatGPTAccountID: "acct_b", Level: IdentitySigned},
	})
	sharedRefresh := "refresh_shared_12345678901234567890"
	raw := mustImportJSON(t, []ImportEntry{
		{AccessToken: accessA, RefreshToken: sharedRefresh, IDToken: idA},
		{AccessToken: accessB, RefreshToken: sharedRefresh, IDToken: idB},
	})

	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Create != 1 || preview.Summary.Conflict != 1 || preview.Rows[1].Action != ImportConflict {
		t.Fatalf("same refresh token with different identities was not a conflict: %#v", preview)
	}
}

func TestImportSameRefreshTokenSameVerifiedIdentitySkipsDuplicate(t *testing.T) {
	manager, _ := newImportTestManager(t, fakeIdentityVerifier{
		idA: {ChatGPTAccountID: "acct_same", Level: IdentitySigned},
		idB: {ChatGPTAccountID: "acct_same", Level: IdentitySigned},
	})
	sharedRefresh := "refresh_shared_12345678901234567890"
	raw := mustImportJSON(t, []ImportEntry{
		{AccessToken: accessA, RefreshToken: sharedRefresh, IDToken: idA},
		{AccessToken: accessB, RefreshToken: sharedRefresh, IDToken: idB},
	})

	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Create != 1 || preview.Summary.Skip != 1 || preview.Rows[1].Action != ImportSkip {
		t.Fatalf("same identity and refresh token was not deduplicated: %#v", preview)
	}
}

func TestImportSameAccessTokenDifferentVerifiedIdentitiesConflicts(t *testing.T) {
	manager, _ := newImportTestManager(t, fakeIdentityVerifier{
		idA: {ChatGPTAccountID: "acct_a", Level: IdentitySigned},
		idB: {ChatGPTAccountID: "acct_b", Level: IdentitySigned},
	})
	raw := mustImportJSON(t, []ImportEntry{
		{AccessToken: accessA, IDToken: idA},
		{AccessToken: accessA, IDToken: idB},
	})

	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Create != 1 || preview.Summary.Conflict != 1 || preview.Rows[1].Action != ImportConflict {
		t.Fatalf("same access token with different identities was not a conflict: %#v", preview)
	}
}

func TestImportSameCredentialsWithoutVerifiedIdentitySkipsWithWarning(t *testing.T) {
	manager, _ := newImportTestManager(t, fakeIdentityVerifier{})
	raw := mustImportJSON(t, []ImportEntry{{AccessToken: accessA}, {AccessToken: accessA}})

	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Create != 1 || preview.Summary.Skip != 1 || preview.Rows[1].Action != ImportSkip {
		t.Fatalf("unverified duplicate credentials were not conservatively deduplicated: %#v", preview)
	}
	if !warningsContain(preview.Rows[1].Warnings, "neither row has a verified identity") {
		t.Fatalf("missing conservative deduplication warning: %#v", preview.Rows[1].Warnings)
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

func TestImportMixedOAuthAndAPIKeyAccounts(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{})
	raw := mustImportJSON(t, []ImportEntry{
		{AccessToken: accessA, Email: "oauth@example.com"},
		{AccountType: string(store.AccountTypeAPIKey), BaseURL: "https://api.example.com/v1/responses", APIKey: "sk-mixed", Email: "Example API"},
	})
	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Create != 2 || preview.Rows[0].AccountType != string(store.AccountTypeOAuth) || preview.Rows[1].AccountType != string(store.AccountTypeAPIKey) {
		t.Fatalf("unexpected mixed preview summary: %#v", preview.Summary)
	}
	if _, err := manager.CommitImport(context.Background(), raw, preview.ContentSHA256, true); err != nil {
		t.Fatal(err)
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("account count = %d, want 2", len(accounts))
	}
	if accounts[0].AccountType != store.AccountTypeOAuth || accounts[0].Status != store.AccountPending {
		t.Fatalf("unexpected OAuth account: %#v", accounts[0])
	}
	if accounts[1].AccountType != store.AccountTypeAPIKey || accounts[1].Status != store.AccountActive || accounts[1].APIKey != "sk-mixed" {
		t.Fatal("API-key account was not imported as active")
	}
}

func TestImportAPIKeyDeduplicatesNormalizedBaseURLAndKey(t *testing.T) {
	manager, _ := newImportTestManager(t, fakeIdentityVerifier{})
	raw := mustImportJSON(t, []ImportEntry{
		{AccountType: string(store.AccountTypeAPIKey), BaseURL: "https://API.Example.com/v1/responses/", APIKey: "sk-shared"},
		{AccountType: string(store.AccountTypeAPIKey), BaseURL: "https://api.example.com/v1/responses", APIKey: "sk-shared"},
	})
	preview, err := manager.PreviewImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Create != 1 || preview.Summary.Skip != 1 || preview.Rows[1].Action != ImportSkip {
		t.Fatalf("normalized API-key duplicate was not skipped: %#v", preview.Summary)
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

func warningsContain(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, fragment) {
			return true
		}
	}
	return false
}

func unsignedIdentityToken(t *testing.T, email, accountID, plan string) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "untrusted"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"email": email,
		"https://api.openai.com/auth": map[string]string{
			"chatgpt_account_id": accountID,
			"chatgpt_plan_type":  plan,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	return encode(header) + "." + encode(payload) + ".invalidsig"
}
