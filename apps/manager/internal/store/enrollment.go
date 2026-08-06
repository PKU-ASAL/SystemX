package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (s *Store) CreateEnrollment(enrollment Enrollment) Enrollment {
	enrollment = normalizeEnrollment(enrollment)
	if enrollment.EnrollmentID == "" || enrollment.TokenHash == "" {
		return Enrollment{}
	}
	now := time.Now().UTC()
	if enrollment.CreatedAt.IsZero() {
		enrollment.CreatedAt = now
	}
	if enrollment.Status == "" {
		enrollment.Status = "active"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Enrollments {
		if existing.TenantID == enrollment.TenantID && existing.EnrollmentID == enrollment.EnrollmentID {
			s.Enrollments[i] = enrollment
			return cloneEnrollment(enrollment)
		}
	}
	s.Enrollments = append(s.Enrollments, enrollment)
	return cloneEnrollment(enrollment)
}

func (s *Store) ListEnrollments(tenantID, status string) []Enrollment {
	if backend, ctx := s.backendCtx(); backend != nil {
		if enrollments, err := backend.ListEnrollments(ctx, tenantID, status); err == nil {
			return enrollments
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Enrollment, 0, len(s.Enrollments))
	for _, enrollment := range s.Enrollments {
		if tenantID != "" && enrollment.TenantID != tenantID {
			continue
		}
		if status != "" && enrollment.Status != status {
			continue
		}
		out = append(out, cloneEnrollment(enrollment))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) ListEnrollmentsWithError(tenantID, status string) ([]Enrollment, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		enrollments, err := backend.ListEnrollments(ctx, tenantID, status)
		if err != nil {
			return nil, fmt.Errorf("list enrollments: %w", err)
		}
		return enrollments, nil
	}
	return s.ListEnrollments(tenantID, status), nil
}

func (s *Store) GetEnrollmentByTokenHash(tokenHash string) (Enrollment, bool) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Enrollment{}, false
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		if enrollment, ok, err := backend.GetEnrollmentByTokenHash(ctx, tokenHash); err == nil {
			return enrollment, ok
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, enrollment := range s.Enrollments {
		if enrollment.TokenHash == tokenHash {
			return cloneEnrollment(enrollment), true
		}
	}
	return Enrollment{}, false
}

func (s *Store) GetEnrollmentByTokenHashWithError(tokenHash string) (Enrollment, bool, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Enrollment{}, false, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		enrollment, ok, err := backend.GetEnrollmentByTokenHash(ctx, tokenHash)
		if err != nil {
			return Enrollment{}, false, fmt.Errorf("get enrollment by token: %w", err)
		}
		return enrollment, ok, nil
	}
	enrollment, ok := s.GetEnrollmentByTokenHash(tokenHash)
	return enrollment, ok, nil
}

func (s *Store) MarkEnrollmentUsed(tokenHash string, usedAt time.Time) (Enrollment, bool) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Enrollment{}, false
	}
	if usedAt.IsZero() {
		usedAt = time.Now().UTC()
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		enrollment, ok, err := backend.GetEnrollmentByTokenHash(ctx, tokenHash)
		if err == nil && ok && enrollment.Status == "active" {
			enrollment.Status = "used"
			enrollment.UsedAt = usedAt
			if err := backend.WriteEnrollment(ctx, enrollment); err == nil {
				s.replaceEnrollmentInMemory(enrollment)
				return cloneEnrollment(enrollment), true
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, enrollment := range s.Enrollments {
		if enrollment.TokenHash != tokenHash {
			continue
		}
		if enrollment.Status != "active" {
			return Enrollment{}, false
		}
		enrollment.Status = "used"
		enrollment.UsedAt = usedAt
		s.Enrollments[i] = enrollment
		return cloneEnrollment(enrollment), true
	}
	return Enrollment{}, false
}

func (s *Store) replaceEnrollmentInMemory(enrollment Enrollment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Enrollments {
		if existing.TenantID == enrollment.TenantID && existing.EnrollmentID == enrollment.EnrollmentID {
			s.Enrollments[i] = enrollment
			return
		}
	}
	s.Enrollments = append(s.Enrollments, enrollment)
}

func normalizeEnrollment(enrollment Enrollment) Enrollment {
	enrollment.EnrollmentID = strings.TrimSpace(enrollment.EnrollmentID)
	enrollment.TenantID = strings.TrimSpace(enrollment.TenantID)
	if enrollment.TenantID == "" {
		enrollment.TenantID = "default"
	}
	enrollment.AgentID = strings.TrimSpace(enrollment.AgentID)
	enrollment.HostID = strings.TrimSpace(enrollment.HostID)
	enrollment.TokenHash = strings.TrimSpace(enrollment.TokenHash)
	enrollment.TokenPreview = strings.TrimSpace(enrollment.TokenPreview)
	enrollment.GatewayAddr = strings.TrimSpace(enrollment.GatewayAddr)
	enrollment.GatewaySNI = strings.TrimSpace(enrollment.GatewaySNI)
	enrollment.Profile = strings.TrimSpace(enrollment.Profile)
	enrollment.Channel = strings.TrimSpace(enrollment.Channel)
	enrollment.ArtifactID = strings.TrimSpace(enrollment.ArtifactID)
	enrollment.ArtifactSHA256 = strings.TrimSpace(enrollment.ArtifactSHA256)
	enrollment.ArtifactURL = strings.TrimSpace(enrollment.ArtifactURL)
	enrollment.Status = strings.TrimSpace(enrollment.Status)
	enrollment.CreatedBy = strings.TrimSpace(enrollment.CreatedBy)
	enrollment.Labels = cloneStringMap(enrollment.Labels)
	return enrollment
}

func (s *Store) RecordAgentCertificate(cert AgentCertificate) AgentCertificate {
	cert = normalizeAgentCertificate(cert)
	if cert.TenantID == "" || cert.AgentID == "" || cert.SerialNumber == "" {
		return AgentCertificate{}
	}
	now := time.Now().UTC()
	if cert.CreatedAt.IsZero() {
		cert.CreatedAt = now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Certificates {
		if existing.TenantID == cert.TenantID && existing.SerialNumber == cert.SerialNumber {
			s.Certificates[i] = cert
			return cert
		}
	}
	s.Certificates = append(s.Certificates, cert)
	return cert
}

func (s *Store) RevokeAgentCertificate(tenantID, agentID, enrollmentID, serial string, revokedAt time.Time) (AgentCertificate, bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	agentID, enrollmentID, serial = strings.TrimSpace(agentID), strings.TrimSpace(enrollmentID), strings.TrimSpace(serial)
	if agentID == "" || enrollmentID == "" || serial == "" {
		return AgentCertificate{}, false, fmt.Errorf("certificate revocation identity is incomplete")
	}
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	receipt := certificateRevocationReceipt(tenantID, agentID, enrollmentID, serial)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	if backend, ctx := s.backendCtx(); backend != nil {
		cert, ok, err := backend.RevokeAgentCertificate(ctx, tenantID, agentID, enrollmentID, serial, revokedAt, receipt)
		if err != nil || !ok {
			return AgentCertificate{}, ok, err
		}
		s.mu.Lock()
		s.Certificates = upsertAgentCertificateSnapshot(s.Certificates, cert)
		s.Unenrollments = insertLegacyUnenrollmentSnapshot(s.Unenrollments, cert)
		s.mu.Unlock()
		return cert, true, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, cert := range s.Certificates {
		if cert.TenantID != tenantID || cert.SerialNumber != serial {
			continue
		}
		if cert.AgentID != agentID || cert.EnrollmentID != enrollmentID {
			return AgentCertificate{}, false, fmt.Errorf("%w: certificate identity mismatch", ErrConflict)
		}
		if cert.RevokedAt.IsZero() {
			cert.RevokedAt, cert.RevocationReceipt = revokedAt.UTC(), receipt
		} else if cert.RevocationReceipt == "" {
			cert.RevocationReceipt = receipt
		}
		oldCerts, oldRecords := s.Certificates, s.Unenrollments
		s.Certificates = replaceCertificate(oldCerts, i, cert)
		s.Unenrollments = insertLegacyUnenrollmentSnapshot(oldRecords, cert)
		if err := s.persistFileLocked(); err != nil {
			s.Certificates, s.Unenrollments = oldCerts, oldRecords
			return AgentCertificate{}, false, fmt.Errorf("persist certificate revocation: %w", err)
		}
		return cert, true, nil
	}
	return AgentCertificate{}, false, nil
}

func (s *Store) GetAgentCertificateWithError(tenantID, serial string) (AgentCertificate, bool, error) {
	tenantID, serial = strings.TrimSpace(tenantID), strings.TrimSpace(serial)
	if tenantID == "" {
		tenantID = "default"
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		cert, ok, err := backend.GetAgentCertificate(ctx, tenantID, serial)
		if err != nil {
			return AgentCertificate{}, false, fmt.Errorf("get agent certificate: %w", err)
		}
		return cert, ok, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, cert := range s.Certificates {
		if cert.TenantID == tenantID && cert.SerialNumber == serial {
			return cert, true, nil
		}
	}
	return AgentCertificate{}, false, nil
}

func certificateRevocationReceipt(tenantID, agentID, enrollmentID, serial string) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + agentID + "\x00" + enrollmentID + "\x00" + serial))
	return "revoke-" + hex.EncodeToString(sum[:16])
}

func upsertAgentCertificateSnapshot(certs []AgentCertificate, cert AgentCertificate) []AgentCertificate {
	out := append([]AgentCertificate(nil), certs...)
	for i, existing := range out {
		if existing.TenantID == cert.TenantID && existing.SerialNumber == cert.SerialNumber {
			out[i] = cert
			return out
		}
	}
	return append(out, cert)
}

func cloneEnrollment(enrollment Enrollment) Enrollment {
	enrollment.Labels = cloneStringMap(enrollment.Labels)
	return enrollment
}

func normalizeAgentCertificate(cert AgentCertificate) AgentCertificate {
	cert.TenantID = strings.TrimSpace(cert.TenantID)
	if cert.TenantID == "" {
		cert.TenantID = "default"
	}
	cert.AgentID = strings.TrimSpace(cert.AgentID)
	cert.EnrollmentID = strings.TrimSpace(cert.EnrollmentID)
	cert.SerialNumber = strings.TrimSpace(cert.SerialNumber)
	cert.UnenrollmentProtocol = strings.TrimSpace(cert.UnenrollmentProtocol)
	cert.Subject = strings.TrimSpace(cert.Subject)
	return cert
}
