package daemon

import (
	"context"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
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

func enrollmentCommand(req *controlplanev1.EnrollRequest) agentcontrol.EnrollmentCommand {
	return agentcontrol.EnrollmentCommand{
		Context:       controlRequestContext(req.GetContext()),
		ManagerURL:    req.GetManagerUrl(),
		Token:         req.GetEnrollmentToken(),
		UploadHistory: req.GetUploadHistory(),
	}
}

func unenrollmentCommand(req *controlplanev1.UnenrollRequest) agentcontrol.UnenrollmentCommand {
	return agentcontrol.UnenrollmentCommand{Context: controlRequestContext(req.GetContext())}
}

func controlRequestContext(req *controlplanev1.RequestContext) agentcontrol.RequestContext {
	return agentcontrol.RequestContext{
		RequestID: req.GetRequestId(),
		TenantID:  req.GetTenantId(),
		AgentID:   req.GetAgentId(),
	}
}
