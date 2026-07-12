package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	managerauth "github.com/sysarmor/sysarmor-next-project/internal/manager/auth"
)

func TestIssueLocalTokenProducesVerifiableJWT(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	keyFile := filepath.Join(t.TempDir(), "jwt.pem")
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyFile, privatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := issueLocalToken([]string{"--private-key", keyFile, "--subject", "local-admin", "--tenant", "default", "--roles", "admin", "--issuer", "sysarmor-local", "--audience", "sysarmor-manager", "--ttl", "8h"})
	if err != nil {
		t.Fatal(err)
	}
	publicDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	verifier, err := managerauth.NewVerifierPEM(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), "sysarmor-local", "sysarmor-manager")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := verifier.Verify(raw)
	if err != nil || principal.Subject != "local-admin" || !principal.HasRole("admin") {
		t.Fatalf("principal=%+v err=%v", principal, err)
	}
}

func TestIssueLocalTokenRejectsInvalidTTLAndRole(t *testing.T) {
	for _, args := range [][]string{
		{"--ttl", "0s"}, {"--ttl", "25h"}, {"--roles", "root"},
	} {
		if _, err := issueLocalToken(args); err == nil {
			t.Fatalf("issueLocalToken(%v) error=nil", args)
		}
	}
}
