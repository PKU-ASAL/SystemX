package daemon

import (
	"context"
	"strings"

	agentcontent "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/content"
	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

type contentController struct {
	runner *AgentRuntime
}

func newContentController(runner *AgentRuntime) *contentController {
	return &contentController{runner: runner}
}

func (c *contentController) ApplyContent(ctx context.Context, command agentcontrol.ContentCommand) agentcontrol.Result {
	req := applyContentRequest(command)
	if command.Source != agentcontrol.PolicySourceManaged {
		release, err := c.runner.beginLocalPolicyMutation(ctx, !command.DryRun)
		if err != nil {
			return controlResult(rejectedAck(c.runner.Config, req.GetContext(), "content", err.Error()))
		}
		defer release()
	}
	return controlResult(c.runner.applyContentUpdate(req))
}

func (c *contentController) ListContent(_ context.Context, kind string) ([]agentcontent.Record, error) {
	return c.runner.contentStore().List(strings.TrimSpace(kind)), nil
}

func (c *contentController) GetContent(_ context.Context, ref string) (agentcontent.Record, bool, error) {
	record, ok := c.runner.contentStore().Get(strings.TrimSpace(ref))
	return record, ok, nil
}

func applyContentRequest(command agentcontrol.ContentCommand) *controlplanev1.ApplyContentRequest {
	return &controlplanev1.ApplyContentRequest{
		Context: &controlplanev1.RequestContext{
			RequestId: command.Context.RequestID,
			TenantId:  command.Context.TenantID,
			AgentId:   command.Context.AgentID,
		},
		ContentJson:   command.Document,
		DryRun:        command.DryRun,
		AllowUnsigned: command.AllowUnsigned,
	}
}
