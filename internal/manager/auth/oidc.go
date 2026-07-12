package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const oidcResponseLimit = 1 << 20
const oidcKeyCacheTTL = 5 * time.Minute

type oidcKeyProvider struct {
	mu                 sync.RWMutex
	keys               map[string]*rsa.PublicKey
	jwksURL            string
	client             *http.Client
	lastUnknownRefresh time.Time
	expiresAt          time.Time
}

type discoveryDocument struct {
	Issuer  string `json:"issuer"`
	JWKSURL string `json:"jwks_uri"`
}

type jwksDocument struct {
	Keys []jsonWebKey `json:"keys"`
}

type jsonWebKey struct {
	KTY string `json:"kty"`
	KID string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func NewOIDCVerifier(ctx context.Context, issuerURL, audience string) (*Verifier, error) {
	issuerURL = strings.TrimRight(strings.TrimSpace(issuerURL), "/")
	if issuerURL == "" || strings.TrimSpace(audience) == "" {
		return nil, fmt.Errorf("OIDC issuer URL and JWT audience are required")
	}
	if err := validateOIDCURL(issuerURL); err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return validateOIDCURL(req.URL.String())
		},
	}
	var discovery discoveryDocument
	if err := getJSON(ctx, client, issuerURL+"/.well-known/openid-configuration", &discovery); err != nil {
		return nil, fmt.Errorf("load OIDC discovery: %w", err)
	}
	if strings.TrimRight(discovery.Issuer, "/") != issuerURL {
		return nil, fmt.Errorf("OIDC discovery issuer %q does not match %q", discovery.Issuer, issuerURL)
	}
	if err := validateOIDCURL(discovery.JWKSURL); err != nil {
		return nil, fmt.Errorf("invalid OIDC jwks_uri: %w", err)
	}
	provider := &oidcKeyProvider{jwksURL: discovery.JWKSURL, client: client}
	if err := provider.refresh(ctx); err != nil {
		return nil, fmt.Errorf("load OIDC JWKS: %w", err)
	}
	return &Verifier{keys: provider, issuer: issuerURL, audience: strings.TrimSpace(audience)}, nil
}

func (p *oidcKeyProvider) Key(token *jwt.Token) (any, error) {
	kid, _ := token.Header["kid"].(string)
	if strings.TrimSpace(kid) == "" {
		return nil, fmt.Errorf("OIDC JWT kid is required")
	}
	p.mu.RLock()
	key := p.keys[kid]
	expired := time.Now().After(p.expiresAt)
	p.mu.RUnlock()
	if key != nil && !expired {
		return key, nil
	}
	if key != nil {
		if err := p.refresh(context.Background()); err != nil {
			return key, nil
		}
		p.mu.RLock()
		key = p.keys[kid]
		p.mu.RUnlock()
		if key != nil {
			return key, nil
		}
	}
	p.mu.Lock()
	if time.Since(p.lastUnknownRefresh) < time.Second {
		p.mu.Unlock()
		return nil, fmt.Errorf("OIDC JWT kid %q is unknown", kid)
	}
	p.lastUnknownRefresh = time.Now()
	p.mu.Unlock()
	if err := p.refresh(context.Background()); err != nil {
		return nil, err
	}
	p.mu.RLock()
	key = p.keys[kid]
	p.mu.RUnlock()
	if key == nil {
		return nil, fmt.Errorf("OIDC JWT kid %q is unknown", kid)
	}
	return key, nil
}

func (p *oidcKeyProvider) refresh(ctx context.Context) error {
	var document jwksDocument
	if err := getJSON(ctx, p.client, p.jwksURL, &document); err != nil {
		return fmt.Errorf("refresh OIDC JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, candidate := range document.Keys {
		if candidate.KTY != "RSA" || candidate.KID == "" || (candidate.Use != "" && candidate.Use != "sig") || (candidate.Alg != "" && candidate.Alg != "RS256") {
			continue
		}
		key, err := candidate.rsaKey()
		if err == nil {
			keys[candidate.KID] = key
		}
	}
	if len(keys) == 0 {
		return fmt.Errorf("OIDC JWKS contains no usable RS256 keys")
	}
	p.mu.Lock()
	p.keys = keys
	p.expiresAt = time.Now().Add(oidcKeyCacheTTL)
	p.mu.Unlock()
	return nil
}

func (k jsonWebKey) rsaKey() (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	modulus := new(big.Int).SetBytes(n)
	exponent := int(new(big.Int).SetBytes(e).Int64())
	if modulus.BitLen() < 2048 || exponent < 3 {
		return nil, fmt.Errorf("OIDC RSA key is too weak")
	}
	return &rsa.PublicKey{N: modulus, E: exponent}, nil
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s status %s", endpoint, resp.Status)
	}
	limited := io.LimitReader(resp.Body, oidcResponseLimit+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(raw) > oidcResponseLimit {
		return fmt.Errorf("OIDC response exceeds %d bytes", oidcResponseLimit)
	}
	return json.Unmarshal(raw, target)
}

func validateOIDCURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("OIDC URL must be absolute")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := parsed.Hostname()
	if parsed.Scheme == "http" && (host == "127.0.0.1" || host == "::1" || host == "localhost") {
		return nil
	}
	return fmt.Errorf("OIDC URL must use HTTPS")
}
