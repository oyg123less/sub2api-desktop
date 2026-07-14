package account

import (
	"context"
	"errors"
	"io"
	"net"
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

func TestRefreshNetworkFailurePreservesAccountStatus(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{})
	account, err := st.CreateAccount(&store.Account{
		AccessToken: "expired-access", RefreshToken: "refresh_12345678901234567890",
		ExpiresAt: time.Now().Add(-time.Hour), Status: store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	})}

	if err := manager.Refresh(context.Background(), client, account.ID); err == nil {
		t.Fatal("expected refresh network error")
	}
	updated, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != store.AccountActive || updated.ConsecutiveFailures != 0 {
		t.Fatalf("status = %q, consecutive failures = %d; want active and zero", updated.Status, updated.ConsecutiveFailures)
	}
}

func TestExchangePreservesExistingRefreshTokenWhenResponseOmitsIt(t *testing.T) {
	manager, st := newImportTestManager(t, fakeIdentityVerifier{})
	idToken := unsignedIdentityToken(t, "exchange@example.com", "acct_exchange", "plus")
	existing, err := st.CreateAccount(&store.Account{
		Email: "exchange@example.com", ChatGPTAccountID: "acct_exchange",
		AccessToken: "old-access", RefreshToken: "refresh_existing_12345678901234567890",
		ExpiresAt: time.Now().Add(time.Hour), Status: store.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := `{"access_token":"new-access","id_token":` + quoteJSON(idToken) + `,"expires_in":3600}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}

	updated, err := manager.Exchange(context.Background(), client, &LoginFlow{CodeVerifier: "verifier", RedirectURI: "http://localhost/callback"}, "code")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != existing.ID || updated.AccessToken != "new-access" || updated.RefreshToken != "refresh_existing_12345678901234567890" {
		t.Fatal("exchange did not preserve the existing refresh token")
	}
	persisted, err := st.GetAccount(existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RefreshToken != "refresh_existing_12345678901234567890" {
		t.Fatal("exchange cleared the persisted refresh token")
	}
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
