package auth

import (
	"context"
	"strings"
	"testing"
)

func TestAuthConfigRejectsMissingAndConflictingModes(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{Mode: "disabled"},
		{Mode: "local", Issuer: "local", Audience: "manager"},
		{Mode: "oidc", IssuerURL: "https://id.example", Audience: "manager", PublicKeyFile: "public.pem"},
	} {
		if _, err := NewVerifier(context.Background(), cfg); err == nil {
			t.Fatalf("NewVerifier(%+v) error=nil", cfg)
		}
	}
}

func TestAuthConfigRejectsOIDCWithoutAudience(t *testing.T) {
	_, err := NewVerifier(context.Background(), Config{Mode: "oidc", IssuerURL: "https://id.example"})
	if err == nil || !strings.Contains(err.Error(), "audience") {
		t.Fatalf("error=%v", err)
	}
}
