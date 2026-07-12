package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestOIDCVerifierCachesJWKSAndRefreshesUnknownKID(t *testing.T) {
	first, _ := rsa.GenerateKey(rand.Reader, 2048)
	second, _ := rsa.GenerateKey(rand.Reader, 2048)
	var mu sync.Mutex
	keys := map[string]*rsa.PublicKey{"key-1": &first.PublicKey}
	requests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": server.URL, "jwks_uri": server.URL + "/jwks"})
		case "/jwks":
			mu.Lock()
			requests++
			out := make([]map[string]string, 0, len(keys))
			for kid, key := range keys {
				out = append(out, jwk(kid, key))
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": out})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	verifier, err := NewOIDCVerifier(t.Context(), server.URL, "manager")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(oidcToken(t, first, "key-1", server.URL)); err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(oidcToken(t, first, "key-1", server.URL)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	keys["key-2"] = &second.PublicKey
	mu.Unlock()
	if _, err := verifier.Verify(oidcToken(t, second, "key-2", server.URL)); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("JWKS requests=%d, want 2", requests)
	}
}

func oidcToken(t *testing.T, key *rsa.PrivateKey, kid, issuer string) string {
	t.Helper()
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: "analyst", Issuer: issuer, Audience: jwt.ClaimStrings{"manager"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}, TenantID: "default", Roles: []string{"viewer"}}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func jwk(kid string, key *rsa.PublicKey) map[string]string {
	return map[string]string{
		"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}
