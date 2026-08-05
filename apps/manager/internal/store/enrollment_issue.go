package store

import (
	"strings"
	"time"
)

type EnrollmentIssueResult string

const (
	EnrollmentIssued        EnrollmentIssueResult = "issued"
	EnrollmentIssueReplay   EnrollmentIssueResult = "replay"
	EnrollmentIssueConflict EnrollmentIssueResult = "conflict"
	EnrollmentIssueMissing  EnrollmentIssueResult = "missing"
)

func (s *Store) CommitEnrollmentIssue(tokenHash, keyHash string, proposed Enrollment, cert AgentCertificate) (Enrollment, EnrollmentIssueResult, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	keyHash = strings.TrimSpace(keyHash)
	if tokenHash == "" || keyHash == "" {
		return Enrollment{}, EnrollmentIssueMissing, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		got, result, err := backend.CommitEnrollmentIssue(ctx, tokenHash, keyHash, proposed, cert)
		if err != nil {
			return Enrollment{}, EnrollmentIssueMissing, err
		}
		if result == EnrollmentIssued {
			s.replaceEnrollmentInMemory(got)
		}
		return got, result, nil
	}
	got, result := s.commitEnrollmentIssueInMemory(tokenHash, keyHash, proposed, cert)
	return got, result, nil
}

func (s *Store) commitEnrollmentIssueInMemory(tokenHash, keyHash string, proposed Enrollment, cert AgentCertificate) (Enrollment, EnrollmentIssueResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, current := range s.Enrollments {
		if current.TokenHash != tokenHash {
			continue
		}
		if current.Status == "issued" {
			if current.IssuedKeySHA256 == keyHash {
				return cloneEnrollment(current), EnrollmentIssueReplay
			}
			return Enrollment{}, EnrollmentIssueConflict
		}
		if current.Status != "active" || (!current.ExpiresAt.IsZero() && time.Now().UTC().After(current.ExpiresAt)) {
			return Enrollment{}, EnrollmentIssueConflict
		}
		proposed.TokenHash = current.TokenHash
		proposed.Status = "issued"
		proposed.IssuedKeySHA256 = keyHash
		s.Enrollments[i] = proposed
		s.upsertCertificateLocked(cert)
		return cloneEnrollment(proposed), EnrollmentIssued
	}
	return Enrollment{}, EnrollmentIssueMissing
}

func (s *Store) upsertCertificateLocked(cert AgentCertificate) {
	cert = normalizeAgentCertificate(cert)
	for i, current := range s.Certificates {
		if current.TenantID == cert.TenantID && current.SerialNumber == cert.SerialNumber {
			s.Certificates[i] = cert
			return
		}
	}
	s.Certificates = append(s.Certificates, cert)
}
