package account

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"sub2api-desktop/core/internal/openai"
)

const (
	openAIIssuer  = "https://auth.openai.com"
	openAIJWKSURL = "https://auth.openai.com/.well-known/jwks.json"

	WarningJWKSUnreachable  = "jwks_unreachable"
	WarningSignatureInvalid = "signature_invalid"
)

var errJWKSUnreachable = errors.New("JWKS unreachable")

type IdentityLevel string

const (
	IdentityUnparsed IdentityLevel = "unparsed"
	IdentityDecoded  IdentityLevel = "decoded"
	IdentitySigned   IdentityLevel = "signed"
)

type VerifiedIdentity struct {
	Email            string
	ChatGPTAccountID string
	PlanType         string
	Level            IdentityLevel
	Expired          bool
}

type IdentityVerifier interface {
	VerifyIDToken(context.Context, string) (VerifiedIdentity, error)
}

type JWTIdentityVerifier struct {
	client     *http.Client
	jwksURL    string
	mu         sync.RWMutex
	keys       map[string]*rsa.PublicKey
	expires    time.Time
	lastErr    error
	retryAfter time.Time
}

func NewJWTIdentityVerifier(client *http.Client, jwksURL string) *JWTIdentityVerifier {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	if jwksURL == "" {
		jwksURL = openAIJWKSURL
	}
	return &JWTIdentityVerifier{client: client, jwksURL: jwksURL, keys: map[string]*rsa.PublicKey{}}
}

type verifiedClaims struct {
	jwt.RegisteredClaims
	Email      string                   `json:"email"`
	OpenAIAuth *openai.OpenAIAuthClaims `json:"https://api.openai.com/auth,omitempty"`
}

func (v *JWTIdentityVerifier) VerifyIDToken(ctx context.Context, raw string) (VerifiedIdentity, error) {
	claims := &verifiedClaims{}
	keyFunc := func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("JWT header is missing kid")
		}
		return v.key(ctx, kid)
	}
	token, err := jwt.ParseWithClaims(raw, claims, keyFunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(openAIIssuer),
		jwt.WithAudience(openai.ClientID),
		jwt.WithExpirationRequired(),
	)
	if errors.Is(err, jwt.ErrTokenExpired) {
		expiredClaims := &verifiedClaims{}
		expiredToken, signatureErr := jwt.ParseWithClaims(raw, expiredClaims, keyFunc,
			jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}), jwt.WithoutClaimsValidation())
		if signatureErr == nil && expiredToken.Valid && validIssuerAudience(expiredClaims) {
			identity, identityErr := identityFromClaims(expiredClaims)
			identity.Expired = true
			return identity, identityErr
		}
	}
	if err != nil || !token.Valid {
		if errors.Is(err, errJWKSUnreachable) {
			return VerifiedIdentity{}, fmt.Errorf("%w: %v", errJWKSUnreachable, err)
		}
		return VerifiedIdentity{}, fmt.Errorf("verify ID token signature or claims: %w", err)
	}
	return identityFromClaims(claims)
}

func identityFromClaims(claims *verifiedClaims) (VerifiedIdentity, error) {
	identity := VerifiedIdentity{Email: claims.Email, Level: IdentitySigned}
	if claims.OpenAIAuth != nil {
		identity.ChatGPTAccountID = claims.OpenAIAuth.ChatGPTAccountID
		identity.PlanType = claims.OpenAIAuth.ChatGPTPlanType
	}
	if identity.ChatGPTAccountID == "" {
		return VerifiedIdentity{}, errors.New("verified ID token has no chatgpt_account_id")
	}
	return identity, nil
}

func validIssuerAudience(claims *verifiedClaims) bool {
	if claims.Issuer != openAIIssuer {
		return false
	}
	for _, audience := range claims.Audience {
		if audience == openai.ClientID {
			return true
		}
	}
	return false
}

func decodeIdentity(raw string) VerifiedIdentity {
	claims, err := openai.DecodeIDToken(raw)
	if err != nil {
		return VerifiedIdentity{Level: IdentityUnparsed}
	}
	info := claims.GetUserInfo()
	return VerifiedIdentity{
		Email: info.Email, ChatGPTAccountID: info.ChatGPTAccountID, PlanType: info.PlanType,
		Level: IdentityDecoded, Expired: claims.Exp > 0 && claims.Exp < time.Now().Unix(),
	}
}

func (v *JWTIdentityVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, fresh := v.keys[kid], time.Now().Before(v.expires)
	lastErr, retryAfter := v.lastErr, v.retryAfter
	v.mu.RUnlock()
	if fresh {
		if key != nil {
			return key, nil
		}
		return nil, fmt.Errorf("JWKS key %q not found", kid)
	}
	if lastErr != nil && time.Now().Before(retryAfter) {
		return nil, lastErr
	}
	if err := v.refresh(ctx); err != nil {
		v.mu.Lock()
		v.lastErr = err
		v.retryAfter = time.Now().Add(30 * time.Second)
		v.mu.Unlock()
		if key != nil {
			return key, nil
		}
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	key = v.keys[kid]
	if key == nil {
		return nil, fmt.Errorf("JWKS key %q not found", kid)
	}
	return key, nil
}

type jsonWebKeySet struct {
	Keys []struct {
		KTY string `json:"kty"`
		KID string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (v *JWTIdentityVerifier) refresh(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	response, err := v.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: fetch JWKS: %v", errJWKSUnreachable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: fetch JWKS: HTTP %d", errJWKSUnreachable, response.StatusCode)
	}
	var set jsonWebKeySet
	if err := json.NewDecoder(response.Body).Decode(&set); err != nil {
		return fmt.Errorf("%w: decode JWKS: %v", errJWKSUnreachable, err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, item := range set.Keys {
		if item.KTY != "RSA" || item.KID == "" || item.N == "" || item.E == "" {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(item.N)
		if err != nil {
			continue
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(item.E)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			continue
		}
		exponent := 0
		for _, value := range exponentBytes {
			exponent = exponent<<8 | int(value)
		}
		if exponent < 3 {
			continue
		}
		keys[item.KID] = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
	}
	if len(keys) == 0 {
		return fmt.Errorf("%w: JWKS contained no usable RSA keys", errJWKSUnreachable)
	}
	v.mu.Lock()
	v.keys = keys
	v.expires = time.Now().Add(cacheMaxAge(response.Header.Get("Cache-Control")))
	v.lastErr = nil
	v.retryAfter = time.Time{}
	v.mu.Unlock()
	return nil
}

func identityVerificationWarningCode(err error) string {
	if errors.Is(err, errJWKSUnreachable) {
		return WarningJWKSUnreachable
	}
	return WarningSignatureInvalid
}

func cacheMaxAge(header string) time.Duration {
	for _, directive := range strings.Split(header, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(directive), "=")
		if ok && strings.EqualFold(name, "max-age") {
			seconds, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}
	return time.Hour
}
