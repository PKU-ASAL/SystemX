package managerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

type deployOptionsResponse struct {
	TenantID           string                   `json:"tenant_id"`
	GatewayAddr        string                   `json:"gateway_addr"`
	GatewaySNI         string                   `json:"gateway_sni,omitempty"`
	SupportedPlatforms []deployPlatformResponse `json:"supported_platforms"`
	Artifacts          []deployArtifactResponse `json:"artifacts"`
	Enrollments        []store.Enrollment       `json:"enrollments"`
}

type deployPlatformResponse struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type deployArtifactResponse struct {
	ArtifactID  string    `json:"artifact_id"`
	Version     string    `json:"version"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	SHA256      string    `json:"sha256"`
	Status      string    `json:"status"`
	DownloadURL string    `json:"download_url"`
	CreatedAt   time.Time `json:"created_at"`
}

type deployAgentCommandRequest struct {
	TenantID    string            `json:"tenant_id,omitempty"`
	AgentID     string            `json:"agent_id,omitempty"`
	HostID      string            `json:"host_id,omitempty"`
	GatewayAddr string            `json:"gateway_addr,omitempty"`
	GatewaySNI  string            `json:"gateway_sni,omitempty"`
	ArtifactID  string            `json:"artifact_id,omitempty"`
	TTL         string            `json:"ttl,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Actor       string            `json:"actor,omitempty"`
}

type deployAgentCommandResponse struct {
	EnrollmentID   string                `json:"enrollment_id"`
	TokenExpiresAt time.Time             `json:"token_expires_at"`
	InstallCommand string                `json:"install_command"`
	ScriptURL      string                `json:"script_url"`
	Artifact       deployCommandArtifact `json:"artifact"`
	Enrollment     store.Enrollment      `json:"enrollment"`
}

type deployCommandArtifact struct {
	ArtifactID  string `json:"artifact_id,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

func (s *Server) uiDeployOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID := defaultString(r.URL.Query().Get("tenant_id"), "default")
	artifacts := s.store.ListArtifacts(tenantID, "agent", "")
	enrollments := s.store.ListEnrollments(tenantID, "")
	for i := range enrollments {
		enrollments[i] = publicEnrollment(enrollments[i])
	}
	writeJSON(w, deployOptionsResponse{
		TenantID:           tenantID,
		GatewayAddr:        defaultDeployGatewayAddr(),
		GatewaySNI:         defaultDeployGatewaySNI(),
		SupportedPlatforms: supportedDeployPlatforms(artifacts),
		Artifacts:          deployArtifacts(r, artifacts),
		Enrollments:        enrollments,
	})
}

func (s *Server) uiDeployAgentCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireOperator(w, r, "admin") {
		return
	}
	var req deployAgentCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode deploy command: %v", err), http.StatusBadRequest)
		return
	}
	enrollmentReq := enrollmentRequest{
		TenantID:    defaultString(req.TenantID, "default"),
		AgentID:     req.AgentID,
		HostID:      req.HostID,
		GatewayAddr: defaultString(req.GatewayAddr, defaultDeployGatewayAddr()),
		GatewaySNI:  defaultString(req.GatewaySNI, defaultDeployGatewaySNI()),
		ArtifactID:  req.ArtifactID,
		Labels:      cloneStringMap(req.Labels),
		TTL:         req.TTL,
		Actor:       req.Actor,
	}
	enrollment, token, err := newEnrollment(enrollmentReq, s.actorFromRequest(r, req.Actor))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	artifactResponse := deployCommandArtifact{}
	if strings.TrimSpace(req.ArtifactID) != "" {
		artifact, ok := s.store.GetArtifact(enrollment.TenantID, req.ArtifactID)
		if !ok || artifact.Status != "active" {
			http.Error(w, "active artifact not found", http.StatusBadRequest)
			return
		}
		enrollment.ArtifactID = artifact.ArtifactID
		enrollment.ArtifactSHA256 = artifact.SHA256
		enrollment.ArtifactURL = artifactDownloadURL(r, artifact.ArtifactID)
		artifactResponse = deployCommandArtifact{
			ArtifactID:  artifact.ArtifactID,
			DownloadURL: enrollment.ArtifactURL,
			SHA256:      artifact.SHA256,
		}
	}
	enrollment = s.store.CreateEnrollment(enrollment)
	if enrollment.EnrollmentID == "" {
		http.Error(w, "create enrollment failed", http.StatusBadRequest)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	scriptURL := installURL(r, token)
	writeJSON(w, deployAgentCommandResponse{
		EnrollmentID:   enrollment.EnrollmentID,
		TokenExpiresAt: enrollment.ExpiresAt,
		InstallCommand: "curl -fsSL " + shellQuote(scriptURL) + " | sudo bash",
		ScriptURL:      scriptURL,
		Artifact:       artifactResponse,
		Enrollment:     publicEnrollment(enrollment),
	})
}

func deployArtifacts(r *http.Request, artifacts []store.Artifact) []deployArtifactResponse {
	out := make([]deployArtifactResponse, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Kind != "agent" {
			continue
		}
		out = append(out, deployArtifactResponse{
			ArtifactID:  artifact.ArtifactID,
			Version:     artifact.Version,
			OS:          artifact.OS,
			Arch:        artifact.Arch,
			SHA256:      artifact.SHA256,
			Status:      artifact.Status,
			DownloadURL: artifactDownloadURL(r, artifact.ArtifactID),
			CreatedAt:   artifact.CreatedAt,
		})
	}
	return out
}

func supportedDeployPlatforms(artifacts []store.Artifact) []deployPlatformResponse {
	seen := map[string]bool{}
	out := []deployPlatformResponse{{OS: "linux", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}}
	for _, item := range out {
		seen[item.OS+"/"+item.Arch] = true
	}
	for _, artifact := range artifacts {
		key := artifact.OS + "/" + artifact.Arch
		if artifact.OS == "" || artifact.Arch == "" || seen[key] {
			continue
		}
		out = append(out, deployPlatformResponse{OS: artifact.OS, Arch: artifact.Arch})
		seen[key] = true
	}
	return out
}

func defaultDeployGatewayAddr() string {
	return defaultString(os.Getenv("SYSARMOR_DEPLOY_GATEWAY_ADDR"), "127.0.0.1:19444")
}

func defaultDeployGatewaySNI() string {
	return os.Getenv("SYSARMOR_DEPLOY_GATEWAY_SNI")
}
