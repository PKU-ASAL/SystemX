package managerapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
	controlmodel "github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
)

func (s *Server) enrollments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		items, err := s.store.ListEnrollmentsWithError(q.Get("tenant_id"), q.Get("status"))
		if err != nil {
			http.Error(w, fmt.Sprintf("list enrollments: %v", err), http.StatusInternalServerError)
			return
		}
		views, err := s.enrollmentViews(items)
		if err != nil {
			http.Error(w, fmt.Sprintf("list unenrollment lifecycle: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"enrollments": views})
	case http.MethodPost:
		if !s.requireOperator(w, r, "admin") {
			return
		}
		var req enrollmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode enrollment: %v", err), http.StatusBadRequest)
			return
		}
		created, err := s.createEnrollment(r, req, s.actorFromRequest(r, req.Actor), false)
		if err != nil {
			writeEnrollmentCreationError(w, err)
			return
		}
		writeJSON(w, map[string]any{
			"enrollment":  publicEnrollment(created.Enrollment),
			"token":       created.Token,
			"install_url": installURL(r, created.BootstrapTicket),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) agentInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token, enrollment, ok, err := s.resolveInstallEnrollment(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok || enrollment.Status != "active" {
		http.Error(w, "enrollment not found", http.StatusNotFound)
		return
	}
	if !enrollment.ExpiresAt.IsZero() && time.Now().UTC().After(enrollment.ExpiresAt) {
		http.Error(w, "enrollment expired", http.StatusGone)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(s.renderAgentInstallScript(r, enrollment, token)))
}

func (s *Server) resolveInstallEnrollment(r *http.Request) (string, store.Enrollment, bool, error) {
	if ticket := strings.TrimSpace(r.URL.Query().Get("ticket")); ticket != "" {
		token, err := newEnrollmentToken()
		if err != nil {
			return "", store.Enrollment{}, false, err
		}
		enrollment, ok, err := s.store.ConsumeEnrollmentBootstrap(
			enrollmentTokenHash(ticket), enrollmentTokenHash(token), tokenPreview(token), time.Now().UTC(),
		)
		if err != nil || !ok {
			return "", enrollment, ok, err
		}
		if err := s.store.Save(); err != nil {
			return "", store.Enrollment{}, false, fmt.Errorf("save bootstrap redemption: %w", err)
		}
		return token, enrollment, true, nil
	}
	return "", store.Enrollment{}, false, nil
}

func (s *Server) enrollmentArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := enrollmentArtifactToken(r)
	enrollment, ok, err := s.store.GetEnrollmentByTokenHashWithError(enrollmentTokenHash(token))
	if err != nil {
		http.Error(w, fmt.Sprintf("read enrollment: %v", err), http.StatusInternalServerError)
		return
	}
	if token == "" || !ok || enrollment.Status != "active" || enrollment.ArtifactID == "" {
		http.NotFound(w, r)
		return
	}
	if !enrollment.ExpiresAt.IsZero() && time.Now().UTC().After(enrollment.ExpiresAt) {
		http.Error(w, "enrollment expired", http.StatusGone)
		return
	}
	artifact, ok, err := s.store.GetArtifactWithError(enrollment.TenantID, enrollment.ArtifactID)
	if err != nil {
		http.Error(w, fmt.Sprintf("read enrollment artifact: %v", err), http.StatusInternalServerError)
		return
	}
	if !ok || artifact.Status != "active" {
		http.NotFound(w, r)
		return
	}
	s.downloadArtifact(w, r, enrollment.TenantID, enrollment.ArtifactID)
}

func enrollmentArtifactToken(r *http.Request) string {
	const prefix = "Enrollment "
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authorization, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	}
	return ""
}

func (s *Server) enrollmentCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.caCert == nil || s.caKey == nil || len(s.caCertPEM) == 0 {
		http.Error(w, "agent certificate authority is not configured", http.StatusServiceUnavailable)
		return
	}
	var req certificateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode certificate request: %v", err), http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(req.Token)
	tokenHash := enrollmentTokenHash(token)
	enrollment, ok, err := s.store.GetEnrollmentByTokenHashWithError(tokenHash)
	if err != nil {
		http.Error(w, fmt.Sprintf("read enrollment: %v", err), http.StatusInternalServerError)
		return
	}
	if !ok || (enrollment.Status != "active" && enrollment.Status != "issued") {
		http.Error(w, "enrollment not found", http.StatusNotFound)
		return
	}
	if enrollment.Status == "active" && !enrollment.ExpiresAt.IsZero() && time.Now().UTC().After(enrollment.ExpiresAt) {
		http.Error(w, "enrollment expired", http.StatusGone)
		return
	}
	certPEM, cert, keyHash, err := s.signAgentCSR(enrollment, []byte(req.CSR))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	issuedAt := time.Now().UTC()
	proposed := enrollment
	proposed.Status = "issued"
	proposed.UsedAt = issuedAt
	proposed.IssuedAt = issuedAt
	proposed.IssuedKeySHA256 = keyHash
	proposed.IssuedCertificatePEM = string(certPEM)
	proposed.IssuedCAPEM = string(s.caCertPEM)
	proposed.IssuedSerialNumber = cert.SerialNumber.String()
	proposed.IssuedNotAfter = cert.NotAfter
	certificate := store.AgentCertificate{
		TenantID:             defaultString(enrollment.TenantID, "default"),
		AgentID:              enrollment.AgentID,
		EnrollmentID:         enrollment.EnrollmentID,
		SerialNumber:         cert.SerialNumber.String(),
		UnenrollmentProtocol: controlmodel.UnenrollmentProtocolCompletionV1,
		Subject:              cert.Subject.String(),
		NotBefore:            cert.NotBefore,
		NotAfter:             cert.NotAfter,
		CreatedAt:            issuedAt,
		CertificatePEM:       string(certPEM),
	}
	enrollment, result, err := s.store.CommitEnrollmentIssue(tokenHash, keyHash, proposed, certificate)
	if err != nil {
		http.Error(w, fmt.Sprintf("commit enrollment certificate: %v", err), http.StatusInternalServerError)
		return
	}
	if result == store.EnrollmentIssueConflict {
		http.Error(w, "enrollment token is already bound to another key", http.StatusConflict)
		return
	}
	if result == store.EnrollmentIssueMissing {
		http.Error(w, "enrollment not found", http.StatusNotFound)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"schema_version":      "sysarmor.enrollment/v2",
		"tenant_id":           defaultString(enrollment.TenantID, "default"),
		"agent_id":            enrollment.AgentID,
		"enrollment_id":       enrollment.EnrollmentID,
		"gateway_address":     enrollment.GatewayAddr,
		"gateway_server_name": enrollment.GatewaySNI,
		"certificate_pem":     enrollment.IssuedCertificatePEM,
		"ca_pem":              enrollment.IssuedCAPEM,
		"serial_number":       enrollment.IssuedSerialNumber,
		"not_after":           enrollment.IssuedNotAfter,
	})
}

func (s *Server) signAgentCSR(enrollment store.Enrollment, csrPEM []byte) ([]byte, *x509.Certificate, string, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, nil, "", fmt.Errorf("csr must be PEM encoded CERTIFICATE REQUEST")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, nil, "", fmt.Errorf("parse csr: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, nil, "", fmt.Errorf("verify csr signature: %w", err)
	}
	keySum := sha256.Sum256(csr.RawSubjectPublicKeyInfo)
	keyHash := hex.EncodeToString(keySum[:])
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("create serial: %w", err)
	}
	tenantID := defaultString(enrollment.TenantID, "default")
	trustDomain := defaultString(os.Getenv("SYSARMOR_TRUST_DOMAIN"), "sysarmor.local")
	uri, err := url.Parse(fmt.Sprintf("spiffe://%s/tenant/%s/agent/%s", trustDomain, tenantID, enrollment.AgentID))
	if err != nil {
		return nil, nil, "", fmt.Errorf("build agent uri san: %w", err)
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("tenant_id:%s,agent_id:%s", tenantID, enrollment.AgentID),
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{uri},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, s.caCert, csr.PublicKey, s.caKey)
	if err != nil {
		return nil, nil, "", fmt.Errorf("sign agent certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, "", fmt.Errorf("parse signed certificate: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert, keyHash, nil
}

func publicEnrollment(enrollment store.Enrollment) store.Enrollment {
	enrollment.TokenHash = ""
	enrollment.BootstrapTokenHash = ""
	enrollment.IssuedKeySHA256 = ""
	enrollment.IssuedCertificatePEM = ""
	enrollment.IssuedCAPEM = ""
	enrollment.Labels = cloneStringMap(enrollment.Labels)
	return enrollment
}

type enrollmentView struct {
	store.Enrollment
	UnenrollmentStatus string     `json:"unenrollment_status,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	EndpointCompleted  *time.Time `json:"endpoint_completed_at,omitempty"`
}

type enrollmentKey struct {
	tenantID, enrollmentID string
}

func (s *Server) enrollmentViews(enrollments []store.Enrollment) ([]enrollmentView, error) {
	records, err := s.unenrollmentRecords(enrollments)
	if err != nil {
		return nil, err
	}
	views := make([]enrollmentView, len(enrollments))
	for i, enrollment := range enrollments {
		record, ok := records[enrollmentKey{enrollment.TenantID, enrollment.EnrollmentID}]
		views[i] = newEnrollmentView(enrollment, record, ok)
	}
	return views, nil
}

func (s *Server) unenrollmentRecords(enrollments []store.Enrollment) (map[enrollmentKey]store.UnenrollmentRecord, error) {
	tenants := make(map[string]struct{})
	for _, enrollment := range enrollments {
		tenants[enrollment.TenantID] = struct{}{}
	}
	records := make(map[enrollmentKey]store.UnenrollmentRecord)
	for tenantID := range tenants {
		items, err := s.store.ListUnenrollmentsWithError(tenantID)
		if err != nil {
			return nil, err
		}
		for _, record := range items {
			records[enrollmentKey{record.TenantID, record.EnrollmentID}] = record
		}
	}
	return records, nil
}

func newEnrollmentView(enrollment store.Enrollment, record store.UnenrollmentRecord, ok bool) enrollmentView {
	view := enrollmentView{Enrollment: publicEnrollment(enrollment)}
	if !ok {
		return view
	}
	view.UnenrollmentStatus = record.Status
	view.RevokedAt = optionalTime(record.RevokedAt)
	view.EndpointCompleted = optionalTime(record.EndpointCompletedAt)
	return view
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

func installURL(r *http.Request, ticket string) string {
	return absoluteURL(r, "/api/v1/agent-install.sh?ticket="+url.QueryEscape(ticket))
}

func artifactDownloadURL(r *http.Request, artifactID string) string {
	return absoluteURL(r, "/api/v1/artifacts/"+artifactID+"/download")
}

func absoluteURL(r *http.Request, path string) string {
	if publicURL := strings.TrimSpace(os.Getenv("SYSARMOR_PUBLIC_URL")); publicURL != "" {
		if parsed, err := url.Parse(publicURL); err == nil && parsed.IsAbs() && parsed.Host != "" {
			return strings.TrimRight(publicURL, "/") + "/" + strings.TrimLeft(path, "/")
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}
