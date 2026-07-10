package account

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"sub2api-desktop/core/internal/openai"
)

func TestJWTIdentityVerifier(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-key"
	exponent := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Cache-Control", "public, max-age=600")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(exponent),
		}}})
	}))
	defer server.Close()

	verifier := NewJWTIdentityVerifier(server.Client(), server.URL)
	valid := signedIdentityToken(t, privateKey, kid, openai.ClientID, time.Now().Add(time.Hour))
	identity, err := verifier.VerifyIDToken(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Level != IdentitySigned || identity.Email != "user@example.com" || identity.ChatGPTAccountID != "acct_verified" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if _, err := verifier.VerifyIDToken(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("JWKS requests = %d, want cache hit after first request", requests.Load())
	}

	wrongAudience := signedIdentityToken(t, privateKey, kid, "wrong-client", time.Now().Add(time.Hour))
	if _, err := verifier.VerifyIDToken(context.Background(), wrongAudience); err == nil {
		t.Fatal("wrong audience token was accepted")
	}
	expired := signedIdentityToken(t, privateKey, kid, openai.ClientID, time.Now().Add(-time.Hour))
	expiredIdentity, err := verifier.VerifyIDToken(context.Background(), expired)
	if err != nil || !expiredIdentity.Expired || expiredIdentity.Level != IdentitySigned {
		t.Fatalf("expired signed token was not retained as trusted identity: identity=%#v err=%v", expiredIdentity, err)
	}
}

func signedIdentityToken(t *testing.T, key *rsa.PrivateKey, kid, audience string, expires time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":   openAIIssuer,
		"aud":   []string{audience},
		"sub":   "user-subject",
		"exp":   expires.Unix(),
		"iat":   time.Now().Add(-time.Minute).Unix(),
		"email": "user@example.com",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct_verified",
			"chatgpt_plan_type":  "plus",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}
