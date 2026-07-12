package daemon

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
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
