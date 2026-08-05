package daemon

import (
	"context"
	"fmt"
	"os"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func (s *localControlServer) Enroll(ctx context.Context, req *controlplanev1.EnrollRequest) (*controlplanev1.ControlAck, error) {
	result := s.enrollmentCoordinator(ctx).Enroll(ctx, req.GetManagerUrl(), req.GetEnrollmentToken(), req.GetUploadHistory())
	return enrollmentAck(req.GetContext(), result.Status, result.Message, result.Identity), nil
}

func rollbackEnrollmentFailure(paths credentialPaths, created bool, cause error) error {
	if err := rollbackEnrollmentCredentials(paths, created); err != nil {
		return fmt.Errorf("%w; rollback credentials: %v", cause, err)
	}
	return cause
}

func (s *localControlServer) Unenroll(ctx context.Context, req *controlplanev1.UnenrollRequest) (*controlplanev1.ControlAck, error) {
	result := s.enrollmentCoordinator(ctx).Unenroll(ctx)
	return enrollmentAck(req.GetContext(), result.Status, result.Message, result.Identity), nil
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
