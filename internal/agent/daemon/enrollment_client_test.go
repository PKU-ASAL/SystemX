package daemon

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnrollmentCSRUsesRequiredIdentity(t *testing.T) {
	_, csrPEM, _, err := createEnrollmentCSR("tenant-a", "agent-a")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(csrPEM)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if csr.Subject.CommonName != "tenant_id:tenant-a,agent_id:agent-a" || csr.CheckSignature() != nil {
		t.Fatalf("invalid CSR: %+v", csr.Subject)
	}
}

func TestValidateEnrollmentCertificateRejectsWrongSubject(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour)}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	ca, _ := x509.ParseCertificate(caDER)
	certTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "tenant_id:other,agent_id:other"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour)}
	certDER, _ := x509.CreateCertificate(rand.Reader, certTemplate, ca, &key.PublicKey, caKey)
	response := enrollmentCertificate{TenantID: "tenant-a", AgentID: "agent-a", CAPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})), CertificatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))}
	if err := validateEnrollmentCertificate(response, key); err == nil {
		t.Fatal("certificate with wrong subject accepted")
	}
}

func TestWriteEnrollmentCredentialsProtectsPrivateKey(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state")
	paths, err := writeEnrollmentCredentials(statePath, enrollmentCertificate{CAPEM: "ca", CertificatePEM: "cert"}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(paths.Key)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode=%o", info.Mode().Perm())
	}
	if info, err := os.Stat(filepath.Dir(paths.Key)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("credentials directory mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestEnrollmentEndpointRejectsRelativeURL(t *testing.T) {
	if _, err := enrollmentEndpoint("manager.local"); err == nil {
		t.Fatal("relative manager URL accepted")
	}
}
