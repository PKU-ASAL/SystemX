package daemon

import (
	"context"
	"fmt"
	"os"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func (s *localControlServer) Enroll(ctx context.Context, req *controlplanev1.EnrollRequest) (*controlplanev1.ControlAck, error) {
	controller := newEnrollmentController(s.enrollmentCoordinator(ctx))
	return controlAck(controller.Enroll(ctx, enrollmentCommand(req))), nil
}

func rollbackEnrollmentFailure(paths credentialPaths, created bool, cause error) error {
	if err := rollbackEnrollmentCredentials(paths, created); err != nil {
		return fmt.Errorf("%w; rollback credentials: %v", cause, err)
	}
	return cause
}

func (s *localControlServer) Unenroll(ctx context.Context, req *controlplanev1.UnenrollRequest) (*controlplanev1.ControlAck, error) {
	controller := newEnrollmentController(s.enrollmentCoordinator(ctx))
	return controlAck(controller.Unenroll(ctx, unenrollmentCommand(req))), nil
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
