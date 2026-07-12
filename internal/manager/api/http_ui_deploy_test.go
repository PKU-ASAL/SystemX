package managerapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func TestUIDeployOptionsReturnsArtifactsAndEnrollments(t *testing.T) {
	st := &store.Store{}
	now := time.Now().UTC()
	st.UpsertArtifact(store.Artifact{
		ArtifactID: "art-linux-amd64",
		TenantID:   "default",
		Name:       "sysarmor-agent",
		Kind:       "agent",
		Version:    "0.8.0",
		OS:         "linux",
		Arch:       "amd64",
		SHA256:     "abc123",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	st.CreateEnrollment(store.Enrollment{
		EnrollmentID: "enr-existing",
		TenantID:     "default",
		AgentID:      "agent-existing",
		HostID:       "host-existing",
		TokenHash:    "hash",
		TokenPreview: "enr_...abcd",
		GatewayAddr:  "127.0.0.1:19444",
		Labels:       map[string]string{"env": "prod"},
		Status:       "active",
		CreatedAt:    now,
		ExpiresAt:    now.Add(time.Hour),
	})
	handler := newAdminTestServer(st).Handler()

	rec := get(t, handler, "/api/v1/ui/deploy/options?tenant_id=default")

	for _, want := range []string{
		`"tenant_id":"default"`,
		`"gateway_addr":"127.0.0.1:19444"`,
		`"os":"linux"`,
		`"arch":"amd64"`,
		`"artifact_id":"art-linux-amd64"`,
		`"enrollment_id":"enr-existing"`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("deploy options missing %s: %s", want, rec.Body.String())
		}
	}
	if strings.Contains(rec.Body.String(), "token_hash") {
		t.Fatalf("deploy options leaked token hash: %s", rec.Body.String())
	}
}

func TestUIDeployAgentCommandCreatesEnrollmentCommand(t *testing.T) {
	st := &store.Store{}
	now := time.Now().UTC()
	artifact := st.UpsertArtifact(store.Artifact{
		ArtifactID: "art-linux-amd64",
		TenantID:   "default",
		Name:       "sysarmor-agent",
		Kind:       "agent",
		Version:    "0.8.0",
		OS:         "linux",
		Arch:       "amd64",
		SHA256:     "abc123",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	handler := newAdminTestServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/deploy/agent-command", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-prod-001",
		"host_id":"prod-api-01",
		"gateway_addr":"127.0.0.1:19444",
		"gateway_sni":"localhost",
		"artifact_id":"art-linux-amd64",
		"ttl":"1h",
		"labels":{"env":"prod","role":"api"}
	}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("deploy command status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		EnrollmentID   string `json:"enrollment_id"`
		InstallCommand string `json:"install_command"`
		ScriptURL      string `json:"script_url"`
		Artifact       struct {
			DownloadURL string `json:"download_url"`
			SHA256      string `json:"sha256"`
		} `json:"artifact"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.EnrollmentID == "" || !strings.Contains(body.InstallCommand, "/api/v1/agent-install.sh?token=") || body.ScriptURL == "" {
		t.Fatalf("deploy command missing command fields: %+v", body)
	}
	if body.Artifact.SHA256 != artifact.SHA256 || !strings.Contains(body.Artifact.DownloadURL, "/api/v1/artifacts/art-linux-amd64/download") {
		t.Fatalf("deploy command artifact mismatch: %+v", body.Artifact)
	}
	enrollments := st.ListEnrollments("default", "active")
	if len(enrollments) != 1 || enrollments[0].AgentID != "agent-prod-001" || enrollments[0].Labels["role"] != "api" {
		t.Fatalf("created enrollments = %+v", enrollments)
	}
}

func TestUIDeployAgentCommandCreatesContainerProfileFromChannel(t *testing.T) {
	st := &store.Store{}
	now := time.Now().UTC()
	artifact := st.UpsertArtifact(store.Artifact{
		ArtifactID: "release-linux-amd64-dev",
		TenantID:   "default",
		Name:       "sysarmor-agent",
		Kind:       "agent",
		Version:    "dev",
		OS:         "linux",
		Arch:       "amd64",
		SHA256:     "abc123",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata: map[string]string{
			"download_url": "http://packages/sysarmor-agent-linux-amd64-dev.tar.gz",
		},
	})
	st.UpsertChannel(store.ArtifactChannel{
		TenantID:   "default",
		Channel:    "linux-container-dev",
		ArtifactID: artifact.ArtifactID,
		UpdatedAt:  now,
	})
	handler := newAdminTestServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/deploy/agent-command", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-container-001",
		"host_id":"container-agent",
		"gateway_addr":"gateway:9444",
		"channel":"linux-container-dev",
		"profile":"linux-container",
		"ttl":"1h",
		"labels":{"env":"dev"}
	}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("deploy container command status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		InstallCommand    string                `json:"install_command"`
		EntrypointCommand string                `json:"entrypoint_command"`
		Artifact          deployCommandArtifact `json:"artifact"`
		Enrollment        store.Enrollment      `json:"enrollment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Enrollment.Profile != "linux-container" || body.Enrollment.Channel != "linux-container-dev" {
		t.Fatalf("container enrollment mismatch: %+v", body.Enrollment)
	}
	if body.Artifact.DownloadURL != "http://packages/sysarmor-agent-linux-amd64-dev.tar.gz" {
		t.Fatalf("container artifact URL = %q", body.Artifact.DownloadURL)
	}
	if strings.Contains(body.InstallCommand, "sudo bash") {
		t.Fatalf("container install command should not require sudo: %s", body.InstallCommand)
	}
	if body.EntrypointCommand != "/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent.yaml" {
		t.Fatalf("container entrypoint command = %q", body.EntrypointCommand)
	}
}

func TestUIDeployAgentCommandFallsBackToArtifactWhenChannelMissing(t *testing.T) {
	st := &store.Store{}
	now := time.Now().UTC()
	artifact := st.UpsertArtifact(store.Artifact{
		ArtifactID: "art-linux-amd64",
		TenantID:   "default",
		Name:       "sysarmor-agent",
		Kind:       "agent",
		Version:    "0.8.0",
		OS:         "linux",
		Arch:       "amd64",
		SHA256:     "abc123",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	handler := newAdminTestServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/deploy/agent-command", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-host-001",
		"gateway_addr":"127.0.0.1:19444",
		"channel":"linux-systemd-dev",
		"profile":"linux-systemd",
		"artifact_id":"art-linux-amd64",
		"ttl":"1h"
	}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("deploy fallback command status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Artifact deployCommandArtifact `json:"artifact"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Artifact.ArtifactID != artifact.ArtifactID {
		t.Fatalf("fallback artifact = %+v", body.Artifact)
	}
}

func TestUIDeployAgentCommandRejectsProfileChannelMismatch(t *testing.T) {
	st := &store.Store{}
	now := time.Now().UTC()
	artifact := st.UpsertArtifact(store.Artifact{
		ArtifactID: "release-linux-container-dev",
		TenantID:   "default",
		Name:       "sysarmor-agent",
		Kind:       "agent",
		Version:    "dev",
		OS:         "linux",
		Arch:       "amd64",
		SHA256:     "abc123",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	st.UpsertChannel(store.ArtifactChannel{
		TenantID:   "default",
		Channel:    "linux-container-dev",
		ArtifactID: artifact.ArtifactID,
		UpdatedAt:  now,
	})
	handler := newAdminTestServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/deploy/agent-command", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-host-001",
		"gateway_addr":"127.0.0.1:19444",
		"channel":"linux-container-dev",
		"profile":"linux-systemd",
		"ttl":"1h"
	}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "channel profile mismatch") {
		t.Fatalf("mismatch status = %d body=%s", rec.Code, rec.Body.String())
	}
}
