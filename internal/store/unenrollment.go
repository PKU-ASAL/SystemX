package store

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	UnenrollmentRevokedEndpointPending = "revoked_endpoint_pending"
	UnenrollmentEndpointCompleted      = "endpoint_completed"
	UnenrollmentUnknownLegacy          = "unknown_legacy"
)

type UnenrollmentRecord struct {
	TenantID            string    `json:"tenant_id"`
	AgentID             string    `json:"agent_id"`
	EnrollmentID        string    `json:"enrollment_id"`
	CertificateSerial   string    `json:"certificate_serial"`
	RevocationReceipt   string    `json:"revocation_receipt"`
	CompletionTokenHash string    `json:"completion_token_hash,omitempty"`
	Status              string    `json:"status"`
	RevokedAt           time.Time `json:"revoked_at"`
	EndpointCompletedAt time.Time `json:"endpoint_completed_at,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (s *Store) AuthorizeAgentUnenrollment(tenantID, agentID, enrollmentID, serial, tokenHash string, revokedAt time.Time) (UnenrollmentRecord, bool, error) {
	identity, err := normalizeUnenrollmentIdentity(tenantID, agentID, enrollmentID, serial, tokenHash)
	if err != nil {
		return UnenrollmentRecord{}, false, err
	}
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	receipt := certificateRevocationReceipt(identity.TenantID, identity.AgentID, identity.EnrollmentID, identity.Serial)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	if backend, ctx := s.backendCtx(); backend != nil {
		cert, record, ok, err := backend.AuthorizeAgentUnenrollment(ctx, identity.TenantID, identity.AgentID, identity.EnrollmentID, identity.Serial, identity.TokenHash, revokedAt, receipt)
		if err == nil && ok {
			s.cacheAuthorizedUnenrollment(cert, record)
		}
		return record, ok, err
	}
	return s.authorizeAgentUnenrollmentInMemory(identity, revokedAt, receipt)
}

func (s *Store) CompleteAgentUnenrollment(tenantID, agentID, enrollmentID, serial, receipt, tokenHash string, completedAt time.Time) (UnenrollmentRecord, bool, error) {
	identity, err := normalizeUnenrollmentIdentity(tenantID, agentID, enrollmentID, serial, tokenHash)
	if err != nil || strings.TrimSpace(receipt) == "" {
		return UnenrollmentRecord{}, false, fmt.Errorf("unenrollment completion identity is incomplete")
	}
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	if backend, ctx := s.backendCtx(); backend != nil {
		record, ok, err := backend.CompleteAgentUnenrollment(ctx, identity.TenantID, identity.AgentID, identity.EnrollmentID, identity.Serial, receipt, identity.TokenHash, completedAt)
		if err == nil && ok {
			s.cacheUnenrollment(record)
		}
		return record, ok, err
	}
	return s.completeAgentUnenrollmentInMemory(identity, receipt, completedAt)
}

func (s *Store) GetUnenrollmentWithError(tenantID, enrollmentID string) (UnenrollmentRecord, bool, error) {
	tenantID, enrollmentID = defaultTenant(tenantID), strings.TrimSpace(enrollmentID)
	if backend, ctx := s.backendCtx(); backend != nil {
		record, ok, err := backend.GetUnenrollment(ctx, tenantID, enrollmentID)
		if err != nil {
			return UnenrollmentRecord{}, false, fmt.Errorf("get agent unenrollment: %w", err)
		}
		return record, ok, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return findUnenrollment(s.Unenrollments, tenantID, enrollmentID)
}

func (s *Store) ListUnenrollmentsWithError(tenantID string) ([]UnenrollmentRecord, error) {
	tenantID = defaultTenant(tenantID)
	if backend, ctx := s.backendCtx(); backend != nil {
		records, err := backend.ListUnenrollments(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list agent unenrollments: %w", err)
		}
		return records, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]UnenrollmentRecord, 0, len(s.Unenrollments))
	for _, record := range s.Unenrollments {
		if record.TenantID == tenantID {
			items = append(items, record)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].EnrollmentID < items[j].EnrollmentID })
	return items, nil
}

type unenrollmentIdentity struct {
	TenantID, AgentID, EnrollmentID, Serial, TokenHash string
}

func normalizeUnenrollmentIdentity(tenantID, agentID, enrollmentID, serial, tokenHash string) (unenrollmentIdentity, error) {
	identity := unenrollmentIdentity{defaultTenant(tenantID), strings.TrimSpace(agentID), strings.TrimSpace(enrollmentID), strings.TrimSpace(serial), strings.ToLower(strings.TrimSpace(tokenHash))}
	decoded, err := hex.DecodeString(identity.TokenHash)
	if identity.AgentID == "" || identity.EnrollmentID == "" || identity.Serial == "" || err != nil || len(decoded) != 32 {
		return unenrollmentIdentity{}, fmt.Errorf("unenrollment identity or completion token hash is invalid")
	}
	return identity, nil
}

func (s *Store) authorizeAgentUnenrollmentInMemory(identity unenrollmentIdentity, revokedAt time.Time, receipt string) (UnenrollmentRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cert, certIndex, ok := findCertificate(s.Certificates, identity.TenantID, identity.Serial)
	if !ok {
		return UnenrollmentRecord{}, false, nil
	}
	if cert.AgentID != identity.AgentID || cert.EnrollmentID != identity.EnrollmentID {
		return UnenrollmentRecord{}, false, fmt.Errorf("%w: certificate identity mismatch", ErrConflict)
	}
	if existing, _, found := findUnenrollmentIndex(s.Unenrollments, identity.TenantID, identity.EnrollmentID); found {
		if cert.RevokedAt.IsZero() || cert.RevocationReceipt != existing.RevocationReceipt || !unenrollmentMatches(existing, identity, receipt) {
			return UnenrollmentRecord{}, false, fmt.Errorf("%w: unenrollment identity mismatch", ErrConflict)
		}
		return existing, true, nil
	}
	if cert.RevokedAt.IsZero() {
		cert.RevokedAt = revokedAt.UTC()
	}
	if cert.RevocationReceipt == "" {
		cert.RevocationReceipt = receipt
	}
	record := newUnenrollmentRecord(identity, cert.RevocationReceipt, cert.RevokedAt)
	oldCerts, oldRecords := s.Certificates, s.Unenrollments
	s.Certificates = replaceCertificate(oldCerts, certIndex, cert)
	s.Unenrollments = appendCopy(oldRecords, record)
	if err := s.persistFileLocked(); err != nil {
		s.Certificates, s.Unenrollments = oldCerts, oldRecords
		return UnenrollmentRecord{}, false, fmt.Errorf("persist unenrollment authorization: %w", err)
	}
	return record, true, nil
}

func (s *Store) completeAgentUnenrollmentInMemory(identity unenrollmentIdentity, receipt string, completedAt time.Time) (UnenrollmentRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, index, ok := findUnenrollmentIndex(s.Unenrollments, identity.TenantID, identity.EnrollmentID)
	if !ok {
		return UnenrollmentRecord{}, false, nil
	}
	if !unenrollmentMatches(record, identity, receipt) {
		return UnenrollmentRecord{}, false, fmt.Errorf("%w: unenrollment completion mismatch", ErrConflict)
	}
	if record.Status == UnenrollmentEndpointCompleted {
		return record, true, nil
	}
	record.Status = UnenrollmentEndpointCompleted
	record.EndpointCompletedAt, record.UpdatedAt = completedAt.UTC(), completedAt.UTC()
	old := s.Unenrollments
	s.Unenrollments = replaceUnenrollment(old, index, record)
	if err := s.persistFileLocked(); err != nil {
		s.Unenrollments = old
		return UnenrollmentRecord{}, false, fmt.Errorf("persist unenrollment completion: %w", err)
	}
	return record, true, nil
}

func newUnenrollmentRecord(identity unenrollmentIdentity, receipt string, revokedAt time.Time) UnenrollmentRecord {
	return UnenrollmentRecord{TenantID: identity.TenantID, AgentID: identity.AgentID, EnrollmentID: identity.EnrollmentID,
		CertificateSerial: identity.Serial, RevocationReceipt: receipt, CompletionTokenHash: identity.TokenHash,
		Status: UnenrollmentRevokedEndpointPending, RevokedAt: revokedAt.UTC(), CreatedAt: revokedAt.UTC(), UpdatedAt: revokedAt.UTC()}
}

func unenrollmentMatches(record UnenrollmentRecord, identity unenrollmentIdentity, receipt string) bool {
	want, wantErr := hex.DecodeString(identity.TokenHash)
	got, gotErr := hex.DecodeString(record.CompletionTokenHash)
	return wantErr == nil && gotErr == nil && subtle.ConstantTimeCompare(want, got) == 1 &&
		record.TenantID == identity.TenantID && record.AgentID == identity.AgentID && record.EnrollmentID == identity.EnrollmentID &&
		record.CertificateSerial == identity.Serial && record.RevocationReceipt == receipt
}

func findCertificate(certs []AgentCertificate, tenantID, serial string) (AgentCertificate, int, bool) {
	for i, cert := range certs {
		if cert.TenantID == tenantID && cert.SerialNumber == serial {
			return cert, i, true
		}
	}
	return AgentCertificate{}, -1, false
}

func findUnenrollment(records []UnenrollmentRecord, tenantID, enrollmentID string) (UnenrollmentRecord, bool, error) {
	record, _, ok := findUnenrollmentIndex(records, tenantID, enrollmentID)
	return record, ok, nil
}

func findUnenrollmentIndex(records []UnenrollmentRecord, tenantID, enrollmentID string) (UnenrollmentRecord, int, bool) {
	for i, record := range records {
		if record.TenantID == tenantID && record.EnrollmentID == enrollmentID {
			return record, i, true
		}
	}
	return UnenrollmentRecord{}, -1, false
}

func replaceCertificate(items []AgentCertificate, index int, value AgentCertificate) []AgentCertificate {
	out := append([]AgentCertificate(nil), items...)
	out[index] = value
	return out
}

func replaceUnenrollment(items []UnenrollmentRecord, index int, value UnenrollmentRecord) []UnenrollmentRecord {
	out := append([]UnenrollmentRecord(nil), items...)
	out[index] = value
	return out
}

func appendCopy(items []UnenrollmentRecord, value UnenrollmentRecord) []UnenrollmentRecord {
	out := append([]UnenrollmentRecord(nil), items...)
	return append(out, value)
}

func (s *Store) cacheAuthorizedUnenrollment(cert AgentCertificate, record UnenrollmentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Certificates = upsertAgentCertificateSnapshot(s.Certificates, cert)
	s.Unenrollments = upsertUnenrollmentSnapshot(s.Unenrollments, record)
}

func (s *Store) cacheUnenrollment(record UnenrollmentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Unenrollments = upsertUnenrollmentSnapshot(s.Unenrollments, record)
}

func upsertUnenrollmentSnapshot(records []UnenrollmentRecord, record UnenrollmentRecord) []UnenrollmentRecord {
	if _, index, ok := findUnenrollmentIndex(records, record.TenantID, record.EnrollmentID); ok {
		return replaceUnenrollment(records, index, record)
	}
	return appendCopy(records, record)
}

func defaultTenant(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return value
}
