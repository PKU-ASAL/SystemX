package store

import (
	"strings"
	"time"
)

func (s *Store) GetEnrollmentByBootstrapTokenHash(tokenHash string) (Enrollment, bool) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Enrollment{}, false
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		enrollment, ok, err := backend.GetEnrollmentByBootstrapTokenHash(ctx, tokenHash)
		return enrollment, ok && err == nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, enrollment := range s.Enrollments {
		if enrollment.BootstrapTokenHash == tokenHash {
			return cloneEnrollment(enrollment), true
		}
	}
	return Enrollment{}, false
}

func (s *Store) ConsumeEnrollmentBootstrap(bootstrapHash, enrollmentHash, enrollmentPreview string, fetchedAt time.Time) (Enrollment, bool, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		got, ok, err := backend.ConsumeEnrollmentBootstrap(ctx, bootstrapHash, enrollmentHash, enrollmentPreview, fetchedAt)
		if err == nil && ok {
			s.replaceEnrollmentInMemory(got)
		}
		return got, ok, err
	}
	if fetchedAt.IsZero() {
		fetchedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, enrollment := range s.Enrollments {
		if enrollment.BootstrapTokenHash != bootstrapHash {
			continue
		}
		if enrollment.Status != "active" || !enrollment.BootstrapFetchedAt.IsZero() ||
			(!enrollment.ExpiresAt.IsZero() && fetchedAt.After(enrollment.ExpiresAt)) {
			return Enrollment{}, false, nil
		}
		enrollment.TokenHash = enrollmentHash
		enrollment.TokenPreview = enrollmentPreview
		enrollment.BootstrapFetchedAt = fetchedAt
		s.Enrollments[i] = enrollment
		return cloneEnrollment(enrollment), true, nil
	}
	return Enrollment{}, false, nil
}
