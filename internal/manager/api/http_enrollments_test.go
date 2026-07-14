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
	handler := newAdminTestServer(st).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-a",
		"host_id":"host-a",
		"gateway_addr":"127.0.0.1:19444",
		"gateway_sni":"localhost",
		"artifact_url":"https://example.invalid/sysarmor-agent.tar.gz",
		"ttl":"1h"
	}`))
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
	if created.Enrollment.Profile != "linux-systemd" || !strings.Contains(body, "systemctl enable --now sysarmor-agent") {
		t.Fatalf("default install profile/script mismatch: profile=%q body=%s", created.Enrollment.Profile, body)
	}
}

func TestContainerEnrollmentInstallScriptUsesEntrypointAndNamespaceScope(t *testing.T) {
	st := &store.Store{}
	handler := newAdminTestServer(st).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-container",
		"host_id":"container-host",
		"gateway_addr":"gateway:9444",
		"gateway_sni":"localhost",
		"artifact_url":"https://example.invalid/sysarmor-agent.tar.gz",
		"profile":"linux-container",
		"labels":{"scenario":"namespace-self-container"},
		"ttl":"1h"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create container enrollment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token      string           `json:"token"`
		InstallURL string           `json:"install_url"`
		Enrollment store.Enrollment `json:"enrollment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Enrollment.Profile != "linux-container" {
		t.Fatalf("container enrollment profile = %q", created.Enrollment.Profile)
	}

	rec = get(t, handler, "/api/v1/agent-install.sh?token="+created.Token)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("container install script status = %d body=%s", rec.Code, body)
	}
	for _, want := range []string{
		`SYSARMOR_INSTALL_PROFILE="${SYSARMOR_INSTALL_PROFILE:-linux-container}"`,
		`label.scenario: "namespace-self-container"`,
		"scope:",
		"    type: namespace",
		"    selector: self",
		"sha256sum -c",
		`sysarmor-agent" run --config`,
		"sysarmorctl\" --socket /run/sysarmor/agent/control.sock",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("container install script missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "systemctl enable --now sysarmor-agent") {
		t.Fatalf("container install script should not enable systemd:\n%s", body)
	}
	if strings.Contains(body, "python3") {
		t.Fatalf("container install script should not require python3:\n%s", body)
	}
}

func TestArtifactUploadDownloadAndEnrollmentBinding(t *testing.T) {
	t.Setenv("SYSARMOR_ARTIFACT_DIR", t.TempDir())
	st := &store.Store{}
	handler := newAdminTestServer(st).Handler()
	distribution := testAgentDistribution(t)

	req := multipartArtifactRequest(t, "/api/v1/artifacts", map[string]string{
		"name":    "sysarmor-agent",
		"kind":    "agent",
		"version": "v-test",
		"os":      "linux",
		"arch":    "amd64",
		"status":  "active",
	}, distribution)
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
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create artifact enrollment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token      string `json:"token"`
		InstallURL string `json:"install_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = get(t, handler, "/api/v1/agent-install.sh?token="+created.Token)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), uploaded.Artifact.SHA256) || !strings.Contains(rec.Body.String(), "/api/v1/enrollment-artifact?token=") {
		t.Fatalf("artifact install script status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/enrollment-artifact?token="+created.Token)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), distribution) {
		t.Fatalf("enrollment artifact status = %d size=%d", rec.Code, rec.Body.Len())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/enrollment-artifact?token=invalid", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("invalid enrollment artifact status = %d", rec.Code)
	}
}

func TestArtifactFeedSeedsExternalArtifactAndChannel(t *testing.T) {
	st := &store.Store{}
	srv := newAdminTestServer(st)
	if err := srv.SeedArtifactFeedData([]byte(`{
		"schema_version":"sysarmor.artifact.feed/v1",
		"artifacts":[{
			"artifact_id":"art-feed-linux-amd64-dev",
			"tenant_id":"default",
			"name":"sysarmor-agent",
			"kind":"agent",
			"version":"dev",
			"os":"linux",
			"arch":"amd64",
			"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"size_bytes":1234,
			"status":"active",
			"download_url":"https://artifacts.example/sysarmor-agent-linux-amd64-dev.tar.gz",
			"channels":["linux-container-dev"]
		}]
	}`)); err != nil {
		t.Fatalf("seed artifact feed: %v", err)
	}
	handler := srv.Handler()

	channel, ok := st.GetChannel("default", "linux-container-dev")
	if !ok || channel.ArtifactID != "art-feed-linux-amd64-dev" {
		t.Fatalf("seeded channel = %+v ok=%v", channel, ok)
	}
	artifact, ok := st.GetArtifact("default", channel.ArtifactID)
	if !ok || artifact.Metadata["download_url"] != "https://artifacts.example/sysarmor-agent-linux-amd64-dev.tar.gz" {
		t.Fatalf("seeded artifact = %+v ok=%v", artifact, ok)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-feed",
		"gateway_addr":"gateway:9444",
		"channel":"linux-container-dev",
		"profile":"linux-container",
		"ttl":"1h"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create feed enrollment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = get(t, handler, "/api/v1/agent-install.sh?token="+created.Token)
	body := rec.Body.String()
	if rec.Code != http.StatusOK ||
		!strings.Contains(body, "/api/v1/enrollment-artifact?token=") ||
		strings.Contains(body, "/api/v1/artifacts/art-feed-linux-amd64-dev/download") {
		t.Fatalf("feed install script status = %d body=%s", rec.Code, body)
	}
}

func TestArtifactFeedFromEnvUsesAgentPackageIndexURL(t *testing.T) {
	st := &store.Store{}
	srv := newAdminTestServer(st)
	index := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"schema_version":"sysarmor.artifact.feed/v1",
			"artifacts":[{
				"artifact_id":"pkg-linux-amd64-dev",
				"tenant_id":"default",
				"name":"sysarmor-agent",
				"kind":"agent",
				"version":"dev",
				"os":"linux",
				"arch":"amd64",
				"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"size_bytes":1234,
				"status":"active",
				"download_url":"https://packages.example/sysarmor-agent-linux-amd64-dev.tar.gz",
				"channels":["linux-container-dev"]
			}]
		}`)
	}))
	defer index.Close()
	t.Setenv("SYSARMOR_AGENT_PACKAGE_INDEX_URL", index.URL)
	t.Setenv("SYSARMOR_ARTIFACT_FEED_URL", "")

	if err := srv.SeedArtifactFeedFromEnv(t.Context()); err != nil {
		t.Fatalf("seed package index from env: %v", err)
	}
	artifact, ok := st.GetArtifact("default", "pkg-linux-amd64-dev")
	if !ok || artifact.Metadata["download_url"] != "https://packages.example/sysarmor-agent-linux-amd64-dev.tar.gz" {
		t.Fatalf("seeded package artifact = %+v ok=%v", artifact, ok)
	}
}

func TestEnrollmentUsesPackageDownloadBaseURLForSystemdProfile(t *testing.T) {
	t.Setenv("SYSARMOR_AGENT_PACKAGE_DOWNLOAD_BASE_URL", "http://127.0.0.1:18080/releases")
	st := &store.Store{}
	srv := newAdminTestServer(st)
	if err := srv.SeedArtifactFeedData([]byte(`{
		"schema_version":"sysarmor.artifact.feed/v1",
		"artifacts":[{
			"artifact_id":"release-linux-amd64-dev",
			"tenant_id":"default",
			"name":"sysarmor-agent",
			"kind":"agent",
			"version":"dev",
			"os":"linux",
			"arch":"amd64",
			"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"size_bytes":1234,
			"status":"active",
			"download_url":"http://packages/sysarmor-agent-linux-amd64-dev.tar.gz",
			"channels":["linux-systemd-dev"]
		}]
	}`)); err != nil {
		t.Fatalf("seed package index: %v", err)
	}
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-systemd",
		"gateway_addr":"gateway:9444",
		"channel":"linux-systemd-dev",
		"profile":"linux-systemd",
		"ttl":"1h"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create systemd enrollment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token      string           `json:"token"`
		Enrollment store.Enrollment `json:"enrollment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if got, want := created.Enrollment.ArtifactURL, "http://127.0.0.1:18080/releases/sysarmor-agent-linux-amd64-dev.tar.gz"; got != want {
		t.Fatalf("systemd artifact url = %q want %q", got, want)
	}

	rec = get(t, handler, "/api/v1/agent-install.sh?token="+created.Token)
	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), "/api/v1/enrollment-artifact?token=") ||
		strings.Contains(rec.Body.String(), "http://127.0.0.1:18080/releases/sysarmor-agent-linux-amd64-dev.tar.gz") {
		t.Fatalf("systemd install script status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEnrollmentKeepsPackageInternalURLForContainerProfile(t *testing.T) {
	t.Setenv("SYSARMOR_AGENT_PACKAGE_DOWNLOAD_BASE_URL", "http://127.0.0.1:18080")
	st := &store.Store{}
	srv := newAdminTestServer(st)
	if err := srv.SeedArtifactFeedData([]byte(`{
		"schema_version":"sysarmor.artifact.feed/v1",
		"artifacts":[{
			"artifact_id":"release-linux-amd64-dev",
			"tenant_id":"default",
			"name":"sysarmor-agent",
			"kind":"agent",
			"version":"dev",
			"os":"linux",
			"arch":"amd64",
			"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"size_bytes":1234,
			"status":"active",
			"download_url":"http://packages/sysarmor-agent-linux-amd64-dev.tar.gz",
			"channels":["linux-container-dev"]
		}]
	}`)); err != nil {
		t.Fatalf("seed package index: %v", err)
	}
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-container",
		"gateway_addr":"gateway:9444",
		"channel":"linux-container-dev",
		"profile":"linux-container",
		"ttl":"1h"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create container enrollment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Enrollment store.Enrollment `json:"enrollment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if got, want := created.Enrollment.ArtifactURL, "http://packages/sysarmor-agent-linux-amd64-dev.tar.gz"; got != want {
		t.Fatalf("container artifact url = %q want %q", got, want)
	}
}

func TestEnrollmentCertificateConsumesToken(t *testing.T) {
	st := &store.Store{}
	srv := newAdminTestServer(st)
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
	srv := newAdminTestServer(st)
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
    "config_path": "/etc/sysarmor/agent/agent.yaml",
    "policy_path": "/etc/sysarmor/agent/policy.json",
    "runtime_socket": "/run/sysarmor/agent/control.sock"
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
