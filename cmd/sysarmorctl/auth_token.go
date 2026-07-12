package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	managerauth "github.com/sysarmor/sysarmor-next-project/internal/manager/auth"
)

func issueLocalToken(args []string) (string, error) {
	flags := flag.NewFlagSet("auth token", flag.ContinueOnError)
	privateKeyFile := flags.String("private-key", "", "RSA private key PEM")
	subject := flags.String("subject", "", "JWT subject")
	tenant := flags.String("tenant", "", "tenant ID")
	rolesRaw := flags.String("roles", "", "comma-separated roles")
	issuer := flags.String("issuer", "", "JWT issuer")
	audience := flags.String("audience", "", "JWT audience")
	ttl := flags.Duration("ttl", 8*time.Hour, "token lifetime")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	roles, err := localTokenRoles(*rolesRaw)
	if err != nil {
		return "", err
	}
	if *ttl <= 0 || *ttl > 24*time.Hour {
		return "", fmt.Errorf("TTL must be greater than zero and at most 24h")
	}
	if strings.TrimSpace(*privateKeyFile) == "" || strings.TrimSpace(*subject) == "" || strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*issuer) == "" || strings.TrimSpace(*audience) == "" {
		return "", fmt.Errorf("private-key, subject, tenant, roles, issuer, and audience are required")
	}
	key, err := readRSAPrivateKey(*privateKeyFile)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	claims := managerauth.Claims{TenantID: strings.TrimSpace(*tenant), Roles: roles, RegisteredClaims: jwt.RegisteredClaims{
		Subject: strings.TrimSpace(*subject), Issuer: strings.TrimSpace(*issuer),
		Audience: jwt.ClaimStrings{strings.TrimSpace(*audience)}, IssuedAt: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(*ttl)),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
}

func localTokenRoles(raw string) ([]string, error) {
	var roles []string
	for _, role := range strings.Split(raw, ",") {
		role = strings.TrimSpace(role)
		switch role {
		case "viewer", "operator", "admin":
			roles = append(roles, role)
		case "":
		default:
			return nil, fmt.Errorf("unsupported role %q", role)
		}
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("at least one recognized role is required")
	}
	return roles, nil
}

func readRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("decode private key PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key must be RSA")
	}
	return key, nil
}
