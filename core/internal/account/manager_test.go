package account

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"sub2api-desktop/core/internal/store"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRefreshBackfillsMissingIdentity(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{})
	idToken := unsignedIdentityToken(t, "refresh@example.com", "acct_refresh", "pro")
	client := refreshTestClient(idToken)

	account, err := st.CreateAccount(&store.Account{
		AccessToken: "expired-access", RefreshToken: "refresh_12345678901234567890",
		ExpiresAt: time.Now().Add(-time.Hour), Status: store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Refresh(context.Background(), client, account.ID); err != nil {
		t.Fatal(err)
	}

	updated, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertAccountIdentity(t, updated, "refresh@example.com", "acct_refresh", "pro")
}

func TestValidAccessTokenBackfillsMissingIdentityWithoutOverwriting(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{})
	idToken := unsignedIdentityToken(t, "new@example.com", "acct_new", "team")
	client := refreshTestClient(idToken)

	account, err := st.CreateAccount(&store.Account{
		Email: "existing@example.com", AccessToken: "expired-access", RefreshToken: "refresh_12345678901234567890",
		ExpiresAt: time.Now().Add(-time.Hour), Status: store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ValidAccessToken(context.Background(), client, account); err != nil {
		t.Fatal(err)
	}

	assertAccountIdentity(t, account, "existing@example.com", "acct_new", "team")
}

func refreshTestClient(idToken string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := `{"access_token":"fresh-access","refresh_token":"fresh-refresh","id_token":` + quoteJSON(idToken) + `,"expires_in":3600}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
}

func quoteJSON(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func assertAccountIdentity(t *testing.T, account *store.Account, email, accountID, plan string) {
	t.Helper()
	if account.Email != email || account.ChatGPTAccountID != accountID || account.PlanType != plan {
		t.Fatalf("identity = (%q, %q, %q), want (%q, %q, %q)",
			account.Email, account.ChatGPTAccountID, account.PlanType, email, accountID, plan)
	}
}
