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
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func TestEnrollmentQueryExposesCompletionWithoutTokenHash(t *testing.T) {
	st := &store.Store{}
	now := time.Unix(100, 0).UTC()
	st.CreateEnrollment(store.Enrollment{
		EnrollmentID: "enroll-completed", TenantID: "default", AgentID: "agent-completed", Status: "issued", TokenHash: strings.Repeat("b", 64),
	})
	st.RecordAgentCertificate(store.AgentCertificate{
		TenantID: "default", AgentID: "agent-completed", EnrollmentID: "enroll-completed", SerialNumber: "42",
	})
	tokenHash := strings.Repeat("a", 64)
	record, ok, err := st.AuthorizeAgentUnenrollment("default", "agent-completed", "enroll-completed", "42", tokenHash, now)
	if err != nil || !ok {
		t.Fatalf("authorize unenrollment ok=%t err=%v", ok, err)
	}
	completedAt := now.Add(time.Minute)
	if _, ok, err = st.CompleteAgentUnenrollment("default", "agent-completed", "enroll-completed", "42", record.RevocationReceipt, tokenHash, completedAt); err != nil || !ok {
		t.Fatalf("complete unenrollment ok=%t err=%v", ok, err)
	}

	rec := get(t, newAdminTestServer(st).Handler(), "/api/v1/enrollments?tenant_id=default")
	if rec.Code != http.StatusOK {
		t.Fatalf("list enrollments status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Enrollments []struct {
			EnrollmentID       string     `json:"enrollment_id"`
			UnenrollmentStatus string     `json:"unenrollment_status"`
			RevokedAt          *time.Time `json:"revoked_at"`
			EndpointCompleted  *time.Time `json:"endpoint_completed_at"`
		} `json:"enrollments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Enrollments) != 1 || response.Enrollments[0].EnrollmentID != "enroll-completed" ||
		response.Enrollments[0].UnenrollmentStatus != store.UnenrollmentEndpointCompleted ||
		response.Enrollments[0].RevokedAt == nil || !response.Enrollments[0].RevokedAt.Equal(now) ||
		response.Enrollments[0].EndpointCompleted == nil || !response.Enrollments[0].EndpointCompleted.Equal(completedAt) {
		t.Fatalf("enrollment projection=%+v", response.Enrollments)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"completion_token", tokenHash, record.RevocationReceipt} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("enrollment response leaked %q: %s", forbidden, body)
		}
	}
}

func TestEnrollmentQueryFailsWhenUnenrollmentProjectionFails(t *testing.T) {
	st := &failingUnenrollmentListStore{Store: &store.Store{}}
	st.CreateEnrollment(store.Enrollment{EnrollmentID: "enroll-a", TenantID: "default", Status: "issued", TokenHash: strings.Repeat("b", 64)})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollments?tenant_id=default", nil)
	rec := httptest.NewRecorder()
	newAdminTestServer(st).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("list enrollments status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "unenrollment backend unavailable") {
		t.Fatalf("list enrollments leaked backend error: %s", rec.Body.String())
	}
}

type failingUnenrollmentListStore struct {
	*store.Store
}

func (*failingUnenrollmentListStore) ListUnenrollmentsWithError(string) ([]store.UnenrollmentRecord, error) {
	return nil, fmt.Errorf("unenrollment backend unavailable")
}

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
	if strings.Contains(created.InstallURL, created.Token) {
		t.Fatalf("install URL leaked enrollment token: %s", created.InstallURL)
	}

	rec = get(t, handler, "/api/v1/enrollments?tenant_id=default")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"agent_id":"agent-a"`) || strings.Contains(rec.Body.String(), "token_hash") {
		t.Fatalf("list enrollments response = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = getInstallScript(t, handler, created.InstallURL)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "manifest.json") || !strings.Contains(body, "--token-file") || !strings.Contains(body, "--timeout 60s") {
		t.Fatalf("install script response = %d body=%s", rec.Code, body)
	}
	if created.Enrollment.Profile != "linux-systemd" || !strings.Contains(body, `"$tmp/install.sh" --profile "$SYSARMOR_INSTALL_PROFILE"`) {
		t.Fatalf("default install profile/script mismatch: profile=%q body=%s", created.Enrollment.Profile, body)
	}
	if strings.Contains(body, `install -m 0755 "$tmp/$DIST_ENTRYPOINT"`) {
		t.Fatalf("install script duplicates the distribution installer: %s", body)
	}
	for _, unsupported := range []string{
		"SYSARMOR_AGENT_HOME",
		"SYSARMOR_CONFIG_DST",
		"SYSARMOR_POLICY_DST",
		"SYSARMOR_SERVICE_DST",
	} {
		if strings.Contains(body, unsupported) {
			t.Fatalf("install script exposes unsupported path override %q: %s", unsupported, body)
		}
	}
}

func TestBootstrapTicketCanFetchInstallScriptOnce(t *testing.T) {
	st := &store.Store{}
	handler := newAdminTestServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-ticket",
		"gateway_addr":"gateway:9444",
		"artifact_url":"https://example.invalid/agent.tar.gz"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var created struct {
		Token      string `json:"token"`
		InstallURL string `json:"install_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	installURL, err := url.Parse(created.InstallURL)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		req = httptest.NewRequest(http.MethodGet, installURL.RequestURI(), nil)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if attempt == 1 && rec.Code != http.StatusOK {
			t.Fatalf("first ticket fetch status=%d body=%s", rec.Code, rec.Body.String())
		}
		if attempt == 1 {
			body := rec.Body.String()
			if !strings.Contains(body, "--token-file") || strings.Contains(body, "--tenant") ||
				strings.Contains(body, "--agent-id") || strings.Contains(body, "--gateway") {
				t.Fatalf("install script uses legacy enrollment arguments: %s", body)
			}
			if strings.Contains(body, "/api/v1/enrollment-artifact?token=") {
				t.Fatalf("install script exposes artifact enrollment token in URL: %s", body)
			}
		}
		if attempt == 2 && rec.Code == http.StatusOK {
			t.Fatalf("bootstrap ticket was reusable: %s", rec.Body.String())
		}
	}
}

func TestEnrollmentTokenIsRejectedInURLs(t *testing.T) {
	st := &store.Store{}
	handler := newAdminTestServer(st).Handler()
	token := createTestEnrollment(t, handler, "agent-no-url-token")
	artifact := st.UpsertArtifact(store.Artifact{
		ArtifactID: "art-no-url-token", TenantID: "default", Kind: "agent", Status: "active",
	})
	enrollment, ok := st.GetEnrollmentByTokenHash(enrollmentTokenHash(token))
	if !ok {
		t.Fatal("created enrollment not found")
	}
	enrollment.ArtifactID = artifact.ArtifactID
	st.CreateEnrollment(enrollment)

	for _, target := range []string{
		"/api/v1/agent-install.sh?token=" + url.QueryEscape(token),
		"/api/v1/enrollment-artifact?token=" + url.QueryEscape(token),
	} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				t.Fatalf("enrollment token was accepted in URL %s", target)
			}
		})
	}
}

func TestEnrollmentInstallURLUsesConfiguredPublicURL(t *testing.T) {
	t.Setenv("SYSARMOR_PUBLIC_URL", "https://manager.public.example/base")
	st := &store.Store{}
	handler := newAdminTestServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-public-url",
		"gateway_addr":"gateway:9444"
	}`))
	req.Host = "manager.internal"
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	req.Header.Set("X-Forwarded-Proto", "http")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var created struct {
		InstallURL string `json:"install_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.InstallURL, "https://manager.public.example/base/") {
		t.Fatalf("install URL=%q", created.InstallURL)
	}
}

func TestNewEnrollmentRejectsInvalidGateway(t *testing.T) {
	tests := []struct {
		name    string
		address string
		sni     string
	}{
		{name: "missing port", address: "gateway.example"},
		{name: "invalid port", address: "gateway.example:70000"},
		{name: "sni contains port", address: "gateway.example:9444", sni: "gateway.example:9444"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := newEnrollment(enrollmentRequest{
				TenantID: "default", AgentID: "agent-a", GatewayAddr: tt.address, GatewaySNI: tt.sni,
			}, "tester")
			if err == nil {
				t.Fatalf("newEnrollment() accepted address=%q sni=%q", tt.address, tt.sni)
			}
		})
	}
}

func TestNewEnrollmentRejectsInvalidCertificateIdentity(t *testing.T) {
	tests := []enrollmentRequest{
		{TenantID: "tenant-a/agent/victim", AgentID: "agent-a", GatewayAddr: "gateway.example:9444"},
		{TenantID: "default", AgentID: "agent-a?tenant=victim", GatewayAddr: "gateway.example:9444"},
		{TenantID: "default", AgentID: "agent-a,tenant_id:victim", GatewayAddr: "gateway.example:9444"},
	}
	for _, req := range tests {
		if _, _, _, err := newEnrollment(req, "tester"); err == nil {
			t.Fatalf("newEnrollment() accepted tenant=%q agent=%q", req.TenantID, req.AgentID)
		}
	}
}

func TestPublicEnrollmentRedactsInternalBindingData(t *testing.T) {
	public := publicEnrollment(store.Enrollment{
		TokenHash:            "token-hash",
		BootstrapTokenHash:   "bootstrap-hash",
		IssuedKeySHA256:      "key-hash",
		IssuedCertificatePEM: "certificate",
		IssuedCAPEM:          "ca",
	})
	if public.TokenHash != "" || public.BootstrapTokenHash != "" ||
		public.IssuedKeySHA256 != "" || public.IssuedCertificatePEM != "" || public.IssuedCAPEM != "" {
		t.Fatalf("public enrollment leaked internal binding data: %+v", public)
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

	rec = getInstallScript(t, handler, created.InstallURL)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("container install script status = %d body=%s", rec.Code, body)
	}
	for _, want := range []string{
		`SYSARMOR_INSTALL_PROFILE="${SYSARMOR_INSTALL_PROFILE:-linux-container}"`,
		`label.scenario: "namespace-self-container"`,
		`enrollment_config_source="$tmp/configs/standalone-container.yaml"`,
		"sha256sum -c",
		`"/opt/sysarmor/agent/bin/sysarmor-agent" run --config "/etc/sysarmor/agent/agent.yaml"`,
		`>"/opt/sysarmor/agent/runtime/agent.log"`,
		`> "/opt/sysarmor/agent/runtime/agent.pid"`,
		"/usr/local/bin/sysarmorctl --socket /run/sysarmor/agent/control.sock",
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
	for _, unsupported := range []string{
		"SYSARMOR_AGENT_HOME",
		"SYSARMOR_CONFIG_DST",
		"SYSARMOR_POLICY_DST",
		"SYSARMOR_SERVICE_DST",
	} {
		if strings.Contains(body, unsupported) {
			t.Fatalf("container install script exposes unsupported path override %q:\n%s", unsupported, body)
		}
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/enrollment-artifact", nil)
	req.Header.Set("Authorization", "Enrollment "+created.Token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), distribution) {
		t.Fatalf("enrollment artifact status = %d size=%d", rec.Code, rec.Body.Len())
	}
	rec = getInstallScript(t, handler, created.InstallURL)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), uploaded.Artifact.SHA256) ||
		!strings.Contains(rec.Body.String(), `Authorization: Enrollment`) ||
		strings.Contains(rec.Body.String(), "/api/v1/enrollment-artifact?token=") {
		t.Fatalf("artifact install script status = %d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/enrollment-artifact", nil)
	req.Header.Set("Authorization", "Enrollment invalid")
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
		InstallURL string `json:"install_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = getInstallScript(t, handler, created.InstallURL)
	body := rec.Body.String()
	if rec.Code != http.StatusOK ||
		!strings.Contains(body, `Authorization: Enrollment`) ||
		strings.Contains(body, "/api/v1/enrollment-artifact?token=") ||
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
		InstallURL string           `json:"install_url"`
		Enrollment store.Enrollment `json:"enrollment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if got, want := created.Enrollment.ArtifactURL, "http://127.0.0.1:18080/releases/sysarmor-agent-linux-amd64-dev.tar.gz"; got != want {
		t.Fatalf("systemd artifact url = %q want %q", got, want)
	}

	rec = getInstallScript(t, handler, created.InstallURL)
	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `Authorization: Enrollment`) ||
		strings.Contains(rec.Body.String(), "/api/v1/enrollment-artifact?token=") ||
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

func TestEnrollmentCertificateIsIdempotentForSameCSR(t *testing.T) {
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
	if rec.Code != http.StatusOK {
		t.Fatalf("certificate status = %d body=%s", rec.Code, rec.Body.String())
	}
	var first struct {
		SchemaVersion  string `json:"schema_version"`
		SerialNumber   string `json:"serial_number"`
		GatewayAddress string `json:"gateway_address"`
		CertificatePEM string `json:"certificate_pem"`
		CAPEM          string `json:"ca_pem"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.SchemaVersion != "sysarmor.enrollment/v2" || first.SerialNumber == "" ||
		first.GatewayAddress != "127.0.0.1:19444" || first.CertificatePEM == "" || first.CAPEM == "" {
		t.Fatalf("first enrollment bundle = %+v", first)
	}

	srv.caCertPEM, srv.caCert, srv.caKey = testCA(t)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-certificate", strings.NewReader(body))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second certificate status = %d body=%s", rec.Code, rec.Body.String())
	}
	var second struct {
		SerialNumber   string `json:"serial_number"`
		CertificatePEM string `json:"certificate_pem"`
		CAPEM          string `json:"ca_pem"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.SerialNumber != first.SerialNumber || second.CertificatePEM != first.CertificatePEM || second.CAPEM != first.CAPEM {
		t.Fatalf("retry bundle differs after CA rotation: first=%+v second=%+v", first, second)
	}
	enrollments := st.ListEnrollments("default", "issued")
	if len(enrollments) != 1 || enrollments[0].IssuedAt.IsZero() {
		t.Fatalf("issued enrollments = %+v", enrollments)
	}
}

func TestEnrollmentCertificateRejectsDifferentKeyAfterIssue(t *testing.T) {
	st := &store.Store{}
	srv := newAdminTestServer(st)
	srv.caCertPEM, srv.caCert, srv.caKey = testCA(t)
	handler := srv.Handler()

	token := createTestEnrollment(t, handler, "agent-cert")
	csr := testCSR(t, "client-supplied-subject-is-ignored")
	body := `{"token":` + strconvQuote(token) + `,"csr":` + strconvQuote(csr) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-certificate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"agent_id":"agent-cert"`) {
		t.Fatalf("first csr status = %d body=%s", rec.Code, rec.Body.String())
	}

	otherCSR := testCSR(t, "tenant_id:default,agent_id:agent-cert")
	otherBody := `{"token":` + strconvQuote(token) + `,"csr":` + strconvQuote(otherCSR) + `}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-certificate", strings.NewReader(otherBody))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("different key status = %d body=%s", rec.Code, rec.Body.String())
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

func getInstallScript(t *testing.T, handler http.Handler, installURL string) *httptest.ResponseRecorder {
	t.Helper()
	parsed, err := url.Parse(installURL)
	if err != nil {
		t.Fatal(err)
	}
	return get(t, handler, parsed.RequestURI())
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
