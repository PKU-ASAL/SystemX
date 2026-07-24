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
	Profile     string            `json:"profile,omitempty"`
	Channel     string            `json:"channel,omitempty"`
	ArtifactID  string            `json:"artifact_id,omitempty"`
	TTL         string            `json:"ttl,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Actor       string            `json:"actor,omitempty"`
}

type deployAgentCommandResponse struct {
	EnrollmentID      string                `json:"enrollment_id"`
	TokenExpiresAt    time.Time             `json:"token_expires_at"`
	InstallCommand    string                `json:"install_command"`
	EntrypointCommand string                `json:"entrypoint_command,omitempty"`
	ScriptURL         string                `json:"script_url"`
	Artifact          deployCommandArtifact `json:"artifact"`
	Enrollment        store.Enrollment      `json:"enrollment"`
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
		Profile:     req.Profile,
		Channel:     req.Channel,
		ArtifactID:  req.ArtifactID,
		Labels:      cloneStringMap(req.Labels),
		TTL:         req.TTL,
		Actor:       req.Actor,
	}
	created, err := s.createEnrollment(r, enrollmentReq, s.actorFromRequest(r, req.Actor), true)
	if err != nil {
		writeEnrollmentCreationError(w, err)
		return
	}
	enrollment := created.Enrollment
	artifactResponse := deployCommandArtifact{}
	if created.Artifact != nil {
		artifact := *created.Artifact
		artifactResponse = deployCommandArtifact{
			ArtifactID:  artifact.ArtifactID,
			DownloadURL: enrollment.ArtifactURL,
			SHA256:      artifact.SHA256,
		}
	}
	scriptURL := installURL(r, created.BootstrapTicket)
	installCommand := "curl -fsSL " + shellQuote(scriptURL) + " | sudo bash"
	entrypointCommand := ""
	if defaultString(enrollment.Profile, "linux-systemd") == "linux-container" {
		installCommand = "curl -fsSL " + shellQuote(scriptURL) + " | bash"
		entrypointCommand = defaultContainerEntrypointCommand()
	}
	writeJSON(w, deployAgentCommandResponse{
		EnrollmentID:      enrollment.EnrollmentID,
		TokenExpiresAt:    enrollment.ExpiresAt,
		InstallCommand:    installCommand,
		EntrypointCommand: entrypointCommand,
		ScriptURL:         scriptURL,
		Artifact:          artifactResponse,
		Enrollment:        publicEnrollment(enrollment),
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
			DownloadURL: artifactInstallURL(r, artifact),
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

func defaultContainerEntrypointCommand() string {
	return "/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent/agent.yaml"
}

func deployChannelMatchesProfile(profile string, channel string) bool {
	switch {
	case strings.HasPrefix(channel, "linux-container-"):
		return profile == "linux-container"
	case strings.HasPrefix(channel, "linux-systemd-"):
		return profile == "linux-systemd"
	default:
		return true
	}
}
