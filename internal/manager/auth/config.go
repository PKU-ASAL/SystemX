package auth

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Mode          string
	PublicKeyFile string
	IssuerURL     string
	Issuer        string
	Audience      string
}

func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	switch strings.TrimSpace(cfg.Mode) {
	case "local":
		if strings.TrimSpace(cfg.IssuerURL) != "" {
			return nil, fmt.Errorf("OIDC issuer URL is not valid in local auth mode")
		}
		if strings.TrimSpace(cfg.PublicKeyFile) == "" {
			return nil, fmt.Errorf("JWT public key file is required in local auth mode")
		}
		publicKey, err := os.ReadFile(cfg.PublicKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read JWT public key: %w", err)
		}
		return NewVerifierPEM(publicKey, cfg.Issuer, cfg.Audience)
	case "oidc":
		if strings.TrimSpace(cfg.PublicKeyFile) != "" || strings.TrimSpace(cfg.Issuer) != "" {
			return nil, fmt.Errorf("local JWT key and issuer are not valid in OIDC auth mode")
		}
		if strings.TrimSpace(cfg.Audience) == "" {
			return nil, fmt.Errorf("JWT audience is required in OIDC auth mode")
		}
		return NewOIDCVerifier(ctx, cfg.IssuerURL, cfg.Audience)
	default:
		return nil, fmt.Errorf("SYSARMOR_AUTH_MODE must be local or oidc")
	}
}
