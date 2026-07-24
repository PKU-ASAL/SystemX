package daemon

import (
	"context"
	"fmt"
	"os"
	"strings"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
)

func (s *localControlServer) Enroll(ctx context.Context, req *controlplanev1.EnrollRequest) (*controlplanev1.ControlAck, error) {
	if s.runner.localStore == nil {
		return enrollmentAck(req.GetContext(), "rejected", "local store is unavailable", runtimeIdentity{}), nil
	}
	certificate, keyPEM, pendingKeyPath, err := requestEnrollmentCertificate(
		ctx, req.GetManagerUrl(), req.GetEnrollmentToken(), s.runner.Config.Local.StatePath,
	)
	if err != nil {
		return enrollmentAck(req.GetContext(), "rejected", err.Error(), s.runner.currentIdentity()), nil
	}
	paths, credentialsCreated, err := writeEnrollmentCredentials(s.runner.Config.Local.StatePath, certificate, keyPEM)
	if err != nil {
		return enrollmentAck(req.GetContext(), "rejected", fmt.Sprintf("write credentials: %v", err), s.runner.currentIdentity()), nil
	}
	stats, err := s.runner.localStore.Stats(ctx)
	if err != nil {
		err = rollbackEnrollmentFailure(paths, credentialsCreated, err)
		return enrollmentAck(req.GetContext(), "rejected", err.Error(), s.runner.currentIdentity()), nil
	}
	fromSequence := stats.LatestEventSequence + 1
	if req.GetUploadHistory() {
		fromSequence = stats.OldestEventSequence
	}
	enrollment := localstore.Enrollment{State: localstore.StateManaged, TenantID: certificate.TenantID, AgentID: certificate.AgentID, GatewayAddress: certificate.GatewayAddress, TLSCAPath: paths.CA, TLSCertPath: paths.Certificate, TLSKeyPath: paths.Key, TLSServerName: certificate.GatewayServerName, UploadHistory: req.GetUploadHistory(), ManagedFromSequence: fromSequence}
	if err := s.runner.localStore.SetManaged(ctx, enrollment); err != nil {
		err = rollbackEnrollmentFailure(paths, credentialsCreated, err)
		return enrollmentAck(req.GetContext(), "rejected", err.Error(), s.runner.currentIdentity()), nil
	}
	s.runner.applyEnrollmentIdentity(enrollment)
	if s.runner.network != nil {
		s.runner.network.ApplyEnrollment(enrollment)
	}
	if err := os.Remove(pendingKeyPath); err != nil && !os.IsNotExist(err) && s.runner.Out != nil {
		fmt.Fprintf(s.runner.Out, "remove pending enrollment key: %v\n", err)
	}
	return enrollmentAck(req.GetContext(), "applied", "agent enrolled", s.runner.currentIdentity()), nil
}

func rollbackEnrollmentFailure(paths credentialPaths, created bool, cause error) error {
	if err := rollbackEnrollmentCredentials(paths, created); err != nil {
		return fmt.Errorf("%w; rollback credentials: %v", cause, err)
	}
	return cause
}

func (s *localControlServer) Unenroll(ctx context.Context, req *controlplanev1.UnenrollRequest) (*controlplanev1.ControlAck, error) {
	if s.runner.localStore == nil {
		return enrollmentAck(req.GetContext(), "rejected", "local store is unavailable", runtimeIdentity{}), nil
	}
	current, err := s.runner.localStore.Enrollment(ctx)
	if err != nil {
		return enrollmentAck(req.GetContext(), "rejected", err.Error(), s.runner.currentIdentity()), nil
	}
	if err := s.runner.localStore.SetStandalone(ctx); err != nil {
		return enrollmentAck(req.GetContext(), "rejected", err.Error(), s.runner.currentIdentity()), nil
	}
	if s.runner.network != nil {
		s.runner.network.StopManaged()
	}
	s.runner.applyEnrollmentIdentity(localstore.Enrollment{State: localstore.StateStandalone})
	cleanupErrors := removeCredentials(credentialPaths{CA: current.TLSCAPath, Certificate: current.TLSCertPath, Key: current.TLSKeyPath})
	message := "agent returned to standalone mode"
	if len(cleanupErrors) > 0 {
		message += "; credential cleanup failed: " + strings.Join(cleanupErrors, "; ")
	}
	return enrollmentAck(req.GetContext(), "applied", message, s.runner.currentIdentity()), nil
}

func removeCredentials(paths credentialPaths) []string {
	var failures []string
	for _, path := range []string{paths.CA, paths.Certificate, paths.Key} {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
		}
	}
	return failures
}

func enrollmentAck(req *controlplanev1.RequestContext, status, message string, identity runtimeIdentity) *controlplanev1.ControlAck {
	return &controlplanev1.ControlAck{RequestId: requestID(req), TenantId: identity.TenantID, AgentId: identity.AgentID, Status: status, Message: message}
}
