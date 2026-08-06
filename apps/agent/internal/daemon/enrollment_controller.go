package daemon

import (
	"context"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
)

type enrollmentController struct {
	coordinator *enrollmentCoordinator
}

func newEnrollmentController(coordinator *enrollmentCoordinator) *enrollmentController {
	return &enrollmentController{coordinator: coordinator}
}

func (c *enrollmentController) Enroll(ctx context.Context, command agentcontrol.EnrollmentCommand) agentcontrol.Result {
	result := c.coordinator.Enroll(ctx, command.ManagerURL, command.Token, command.UploadHistory)
	return enrollmentControlResult(command.Context, result)
}

func (c *enrollmentController) Unenroll(ctx context.Context, command agentcontrol.UnenrollmentCommand) agentcontrol.Result {
	result := c.coordinator.Unenroll(ctx)
	return enrollmentControlResult(command.Context, result)
}

func enrollmentControlResult(request agentcontrol.RequestContext, result enrollmentResult) agentcontrol.Result {
	return agentcontrol.Result{
		RequestID: request.RequestID,
		TenantID:  result.Identity.TenantID,
		AgentID:   result.Identity.AgentID,
		Status:    result.Status,
		Message:   result.Message,
	}
}
