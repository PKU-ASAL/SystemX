package daemon

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxEnrollmentResponseBytes = 1 << 20

type enrollmentCertificate struct {
	TenantID       string `json:"tenant_id"`
	AgentID        string `json:"agent_id"`
	CertificatePEM string `json:"certificate_pem"`
	CAPEM          string `json:"ca_pem"`
}

type credentialPaths struct {
	CA, Certificate, Key string
}

func requestEnrollmentCertificate(ctx context.Context, managerURL, token, tenantID, agentID string) (enrollmentCertificate, []byte, error) {
	key, csr, keyPEM, err := createEnrollmentCSR(tenantID, agentID)
	if err != nil {
		return enrollmentCertificate{}, nil, err
	}
	endpoint, err := enrollmentEndpoint(managerURL)
	if err != nil {
		return enrollmentCertificate{}, nil, err
	}
	body, _ := json.Marshal(map[string]string{"token": token, "csr": string(csr)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return enrollmentCertificate{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return enrollmentCertificate{}, nil, fmt.Errorf("request enrollment certificate: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxEnrollmentResponseBytes+1))
	if err != nil || len(raw) > maxEnrollmentResponseBytes {
		return enrollmentCertificate{}, nil, fmt.Errorf("read enrollment response")
	}
	if resp.StatusCode != http.StatusOK {
		return enrollmentCertificate{}, nil, fmt.Errorf("manager rejected enrollment: HTTP %d", resp.StatusCode)
	}
	var certificate enrollmentCertificate
	if err := json.Unmarshal(raw, &certificate); err != nil {
		return enrollmentCertificate{}, nil, fmt.Errorf("decode enrollment response: %w", err)
	}
	if certificate.TenantID != tenantID || certificate.AgentID != agentID {
		return enrollmentCertificate{}, nil, fmt.Errorf("enrollment identity mismatch")
	}
	if err := validateEnrollmentCertificate(certificate, key); err != nil {
		return enrollmentCertificate{}, nil, err
	}
	return certificate, keyPEM, nil
}

func createEnrollmentCSR(tenantID, agentID string) (*ecdsa.PrivateKey, []byte, []byte, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(agentID) == "" {
		return nil, nil, nil, fmt.Errorf("tenant_id and agent_id are required")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cn := fmt.Sprintf("tenant_id:%s,agent_id:%s", tenantID, agentID)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: cn}}, key)
	if err != nil {
		return nil, nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}

func enrollmentEndpoint(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("manager_url must be an absolute HTTP(S) URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/enrollment-certificate"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func validateEnrollmentCertificate(response enrollmentCertificate, key *ecdsa.PrivateKey) error {
	caBlock, _ := pem.Decode([]byte(response.CAPEM))
	certBlock, _ := pem.Decode([]byte(response.CertificatePEM))
	if caBlock == nil || certBlock == nil {
		return fmt.Errorf("enrollment response contains invalid PEM")
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse enrollment CA: %w", err)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse enrollment certificate: %w", err)
	}
	public, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok || !public.Equal(&key.PublicKey) {
		return fmt.Errorf("enrollment certificate does not match private key")
	}
	wantCN := fmt.Sprintf("tenant_id:%s,agent_id:%s", response.TenantID, response.AgentID)
	if cert.Subject.CommonName != wantCN {
		return fmt.Errorf("enrollment certificate identity mismatch")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, err = cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	if err != nil {
		return fmt.Errorf("verify enrollment certificate: %w", err)
	}
	return nil
}

func writeEnrollmentCredentials(statePath string, certificate enrollmentCertificate, keyPEM []byte) (credentialPaths, error) {
	dir := filepath.Join(statePath, "credentials")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return credentialPaths{}, err
	}
	paths := credentialPaths{CA: filepath.Join(dir, "ca.pem"), Certificate: filepath.Join(dir, "agent.pem"), Key: filepath.Join(dir, "agent-key.pem")}
	for _, file := range []struct {
		path string
		data []byte
		mode os.FileMode
	}{{paths.CA, []byte(certificate.CAPEM), 0o644}, {paths.Certificate, []byte(certificate.CertificatePEM), 0o644}, {paths.Key, keyPEM, 0o600}} {
		if err := writeAtomicFile(file.path, file.data, file.mode); err != nil {
			removeCredentials(paths)
			return credentialPaths{}, err
		}
	}
	return paths, nil
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credential-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
