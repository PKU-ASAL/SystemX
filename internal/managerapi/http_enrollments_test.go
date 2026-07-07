package managerapi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
