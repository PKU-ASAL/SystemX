package managerapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/store"
)

type enrollmentCreation struct {
	Enrollment      store.Enrollment
	Token           string
	BootstrapTicket string
	Artifact        *store.Artifact
}

type enrollmentCreationError struct {
	statusCode int
	err        error
}

var enrollmentIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func (e *enrollmentCreationError) Error() string { return e.err.Error() }

func (s *Server) createEnrollment(r *http.Request, req enrollmentRequest, actor string, deployRequest bool) (enrollmentCreation, error) {
	enrollment, token, bootstrapTicket, err := newEnrollment(req, actor)
	if err != nil {
		return enrollmentCreation{}, creationError(http.StatusBadRequest, err)
	}
	artifactID := strings.TrimSpace(req.ArtifactID)
	if channelName := strings.TrimSpace(req.Channel); channelName != "" {
		channel, ok, err := s.store.GetChannelWithError(enrollment.TenantID, channelName)
		if err != nil {
			return enrollmentCreation{}, creationError(http.StatusInternalServerError, fmt.Errorf("read channel: %w", err))
		}
		if !ok && (!deployRequest || artifactID == "") {
			return enrollmentCreation{}, creationError(http.StatusBadRequest, fmt.Errorf("channel not found"))
		}
		if ok {
			if deployRequest && !deployChannelMatchesProfile(enrollment.Profile, channel.Channel) {
				return enrollmentCreation{}, creationError(http.StatusBadRequest, fmt.Errorf("channel profile mismatch"))
			}
			artifactID = channel.ArtifactID
			enrollment.Channel = channel.Channel
		}
	}
	var artifact *store.Artifact
	if artifactID != "" {
		resolved, ok, err := s.store.GetArtifactWithError(enrollment.TenantID, artifactID)
		if err != nil {
			return enrollmentCreation{}, creationError(http.StatusInternalServerError, fmt.Errorf("read artifact: %w", err))
		}
		if !ok || resolved.Status != "active" {
			return enrollmentCreation{}, creationError(http.StatusBadRequest, fmt.Errorf("active artifact not found"))
		}
		enrollment.ArtifactID = resolved.ArtifactID
		enrollment.ArtifactSHA256 = resolved.SHA256
		enrollment.ArtifactURL = artifactInstallURLForProfile(r, resolved, enrollment.Profile)
		artifact = &resolved
	}
	enrollment = s.store.CreateEnrollment(enrollment)
	if enrollment.EnrollmentID == "" {
		return enrollmentCreation{}, creationError(http.StatusBadRequest, fmt.Errorf("create enrollment failed"))
	}
	if err := s.store.Save(); err != nil {
		return enrollmentCreation{}, creationError(http.StatusInternalServerError, fmt.Errorf("save store: %w", err))
	}
	return enrollmentCreation{
		Enrollment:      enrollment,
		Token:           token,
		BootstrapTicket: bootstrapTicket,
		Artifact:        artifact,
	}, nil
}

func creationError(statusCode int, err error) error {
	return &enrollmentCreationError{statusCode: statusCode, err: err}
}

func writeEnrollmentCreationError(w http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	if creationErr, ok := err.(*enrollmentCreationError); ok {
		statusCode = creationErr.statusCode
	}
	http.Error(w, err.Error(), statusCode)
}

func validateEnrollmentGateway(address, serverName string) error {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || strings.TrimSpace(host) == "" {
		return fmt.Errorf("gateway_addr must be host:port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("gateway_addr port must be between 1 and 65535")
	}
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return nil
	}
	if strings.ContainsAny(serverName, ":/\\ \t") {
		return fmt.Errorf("gateway_sni must be a host name without a port")
	}
	return nil
}

func newEnrollment(req enrollmentRequest, actor string) (store.Enrollment, string, string, error) {
	tenantID := defaultString(strings.TrimSpace(req.TenantID), "default")
	agentID := strings.TrimSpace(req.AgentID)
	if !enrollmentIdentityPattern.MatchString(tenantID) {
		return store.Enrollment{}, "", "", fmt.Errorf("tenant_id must match %s", enrollmentIdentityPattern.String())
	}
	if !enrollmentIdentityPattern.MatchString(agentID) {
		return store.Enrollment{}, "", "", fmt.Errorf("agent_id must match %s", enrollmentIdentityPattern.String())
	}
	if strings.TrimSpace(req.GatewayAddr) == "" {
		return store.Enrollment{}, "", "", fmt.Errorf("gateway_addr is required")
	}
	if err := validateEnrollmentGateway(req.GatewayAddr, req.GatewaySNI); err != nil {
		return store.Enrollment{}, "", "", err
	}
	profile, err := normalizeInstallProfile(req.Profile)
	if err != nil {
		return store.Enrollment{}, "", "", err
	}
	ttl := 24 * time.Hour
	if strings.TrimSpace(req.TTL) != "" {
		ttl, err = time.ParseDuration(req.TTL)
		if err != nil {
			return store.Enrollment{}, "", "", fmt.Errorf("ttl: %w", err)
		}
		if ttl <= 0 {
			return store.Enrollment{}, "", "", fmt.Errorf("ttl must be positive")
		}
	}
	token, err := newEnrollmentToken()
	if err != nil {
		return store.Enrollment{}, "", "", err
	}
	bootstrapTicket, err := newEnrollmentToken()
	if err != nil {
		return store.Enrollment{}, "", "", err
	}
	now := time.Now().UTC()
	hostID := defaultString(req.HostID, agentID)
	return store.Enrollment{
		EnrollmentID:          "enr-" + now.Format("20060102T150405Z") + "-" + token[len(token)-8:],
		TenantID:              tenantID,
		AgentID:               agentID,
		HostID:                hostID,
		TokenHash:             enrollmentTokenHash(token),
		TokenPreview:          tokenPreview(token),
		BootstrapTokenHash:    enrollmentTokenHash(bootstrapTicket),
		BootstrapTokenPreview: tokenPreview(bootstrapTicket),
		GatewayAddr:           req.GatewayAddr,
		GatewaySNI:            req.GatewaySNI,
		Profile:               profile,
		Channel:               req.Channel,
		ArtifactID:            req.ArtifactID,
		ArtifactURL:           req.ArtifactURL,
		Labels:                cloneStringMap(req.Labels),
		Status:                "active",
		CreatedAt:             now,
		ExpiresAt:             now.Add(ttl),
		CreatedBy:             actor,
	}, token, bootstrapTicket, nil
}

func normalizeInstallProfile(profile string) (string, error) {
	switch strings.TrimSpace(profile) {
	case "", "linux-systemd":
		return "linux-systemd", nil
	case "linux-container":
		return "linux-container", nil
	default:
		return "", fmt.Errorf("unsupported install profile %q", profile)
	}
}

func newEnrollmentToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate enrollment token: %w", err)
	}
	return "enr_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func enrollmentTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func tokenPreview(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:8] + "..." + token[len(token)-4:]
}
