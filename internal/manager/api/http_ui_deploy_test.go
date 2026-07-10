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
	handler := NewServer(st).Handler()

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
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()
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
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "admin")
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
