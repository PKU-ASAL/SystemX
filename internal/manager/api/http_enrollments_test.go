package managerapi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func TestEnrollmentCreateListAndInstallScript(t *testing.T) {
	st := &store.Store{}
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-a",
		"host_id":"host-a",
		"gateway_addr":"127.0.0.1:19444",
		"gateway_sni":"localhost",
		"artifact_url":"https://example.invalid/sysarmor-agent.tar.gz",
		"ttl":"1h"
	}`))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create enrollment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token      string           `json:"token"`
		InstallURL string           `json:"install_url"`
		Enrollment store.Enrollment `json:"enrollment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.InstallURL == "" || created.Enrollment.TokenHash != "" {
		t.Fatalf("create enrollment response leaked or missed fields: %+v", created)
	}

	rec = get(t, handler, "/api/v1/enrollments?tenant_id=default")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"agent_id":"agent-a"`) || strings.Contains(rec.Body.String(), "token_hash") {
		t.Fatalf("list enrollments response = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/agent-install.sh?token="+created.Token)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "manifest.json") || !strings.Contains(body, "127.0.0.1:19444") {
		t.Fatalf("install script response = %d body=%s", rec.Code, body)
	}
}

func TestArtifactUploadDownloadAndEnrollmentBinding(t *testing.T) {
	t.Setenv("SYSARMOR_ARTIFACT_DIR", t.TempDir())
	st := &store.Store{}
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()

	req := multipartArtifactRequest(t, "/api/v1/artifacts", map[string]string{
		"name":    "sysarmor-agent",
		"kind":    "agent",
		"version": "v-test",
		"os":      "linux",
		"arch":    "amd64",
		"status":  "active",
	}, testAgentDistribution(t))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("artifact upload status = %d body=%s", rec.Code, rec.Body.String())
	}
	var uploaded struct {
		Artifact store.Artifact `json:"artifact"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.Artifact.ArtifactID == "" || uploaded.Artifact.SHA256 == "" || uploaded.Artifact.SizeBytes == 0 {
		t.Fatalf("uploaded artifact missing fields: %+v", uploaded.Artifact)
	}

	rec = get(t, handler, "/api/v1/artifacts/"+uploaded.Artifact.ArtifactID+"/download")
	if rec.Code != http.StatusOK || rec.Header().Get("X-SysArmor-Artifact-SHA256") != uploaded.Artifact.SHA256 {
		t.Fatalf("artifact download status = %d sha=%q body=%q", rec.Code, rec.Header().Get("X-SysArmor-Artifact-SHA256"), rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-artifact",
		"gateway_addr":"127.0.0.1:19444",
		"artifact_id":"`+uploaded.Artifact.ArtifactID+`",
		"ttl":"1h"
	}`))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "admin")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create artifact enrollment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = get(t, handler, "/api/v1/agent-install.sh?token="+created.Token)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), uploaded.Artifact.SHA256) || !strings.Contains(rec.Body.String(), "/api/v1/artifacts/"+uploaded.Artifact.ArtifactID+"/download") {
		t.Fatalf("artifact install script status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEnrollmentCertificateConsumesToken(t *testing.T) {
	st := &store.Store{}
	srv := NewServerWithOperatorToken(st, "operator-token")
	srv.caCertPEM, srv.caCert, srv.caKey = testCA(t)
	handler := srv.Handler()

	token := createTestEnrollment(t, handler, "agent-cert")
	csr := testCSR(t, "tenant_id:default,agent_id:agent-cert")
	body := `{"token":` + strconvQuote(token) + `,"csr":` + strconvQuote(csr) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-certificate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"certificate_pem"`) {
		t.Fatalf("certificate status = %d body=%s", rec.Code, rec.Body.String())
	}
	enrollments := st.ListEnrollments("default", "used")
	if len(enrollments) != 1 || enrollments[0].UsedAt.IsZero() {
		t.Fatalf("used enrollments = %+v", enrollments)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-certificate", strings.NewReader(body))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second certificate status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEnrollmentCertificateRejectsMismatchedCSR(t *testing.T) {
	st := &store.Store{}
	srv := NewServerWithOperatorToken(st, "operator-token")
	srv.caCertPEM, srv.caCert, srv.caKey = testCA(t)
	handler := srv.Handler()

	token := createTestEnrollment(t, handler, "agent-cert")
	csr := testCSR(t, "tenant_id:default,agent_id:other-agent")
	body := `{"token":` + strconvQuote(token) + `,"csr":` + strconvQuote(csr) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-certificate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "does not match enrollment") {
		t.Fatalf("mismatched csr status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := st.ListEnrollments("default", "used"); len(got) != 0 {
		t.Fatalf("mismatched CSR consumed enrollment: %+v", got)
	}
}

func createTestEnrollment(t *testing.T, handler http.Handler, agentID string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"`+agentID+`",
		"gateway_addr":"127.0.0.1:19444",
		"ttl":"1h"
	}`))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create enrollment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return created.Token
}

func testCA(t *testing.T) ([]byte, *x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "sysarmor-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert, key
}

func testCSR(t *testing.T, commonName string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: commonName}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}

func strconvQuote(v string) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func multipartArtifactRequest(t *testing.T, target string, fields map[string]string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "sysarmor-agent.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func testAgentDistribution(t *testing.T) []byte {
	t.Helper()
	files := map[string][]byte{
		"bin/sysarmor-agent":             []byte("#!/bin/sh\n"),
		"systemd/sysarmor-agent.service": []byte("[Service]\nExecStart=/opt/sysarmor/agent/bin/sysarmor-agent\n"),
		"sensors/tetragon/bin/tetragon":  []byte("#!/bin/sh\n"),
		"sensors/tetragon/bin/tetra":     []byte("#!/bin/sh\n"),
		"sensors/tetragon/manifest.json": []byte(`{"name":"tetragon"}`),
		"manifest.sig":                   []byte("unsigned-test-signature"),
	}
	manifest := fmt.Sprintf(`{
  "schema_version": "sysarmor.agent.distribution/v1",
  "name": "sysarmor-agent",
  "version": "v-test",
  "os": "linux",
  "arch": "amd64",
  "entrypoint": "bin/sysarmor-agent",
  "systemd_unit": "systemd/sysarmor-agent.service",
  "install": {
    "agent_home": "/opt/sysarmor/agent",
    "config_path": "/etc/sysarmor/agent.yaml",
    "policy_dir": "/etc/sysarmor/policies",
    "runtime_socket": "/run/sysarmor/agent.sock"
  },
  "sensors": [{"name":"tetragon","backend":"tetragon","bundle_dir":"sensors/tetragon","install_dir":"sensors"}],
  "files": [
    {"path":"bin/sysarmor-agent","mode":"0755","sha256":"%s"},
    {"path":"systemd/sysarmor-agent.service","mode":"0644","sha256":"%s"},
    {"path":"sensors/tetragon/bin/tetragon","mode":"0755","sha256":"%s"},
    {"path":"sensors/tetragon/bin/tetra","mode":"0755","sha256":"%s"},
    {"path":"sensors/tetragon/manifest.json","mode":"0644","sha256":"%s"}
  ]
}`, shaHex(files["bin/sysarmor-agent"]), shaHex(files["systemd/sysarmor-agent.service"]), shaHex(files["sensors/tetragon/bin/tetragon"]), shaHex(files["sensors/tetragon/bin/tetra"]), shaHex(files["sensors/tetragon/manifest.json"]))
	files["manifest.json"] = []byte(manifest)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func shaHex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
