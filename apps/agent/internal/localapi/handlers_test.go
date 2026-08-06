package localapi

import (
	"context"
	"testing"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

type recordingPolicyController struct {
	command agentcontrol.PolicyCommand
}

func (r *recordingPolicyController) ApplyPolicy(_ context.Context, command agentcontrol.PolicyCommand) agentcontrol.Result {
	r.command = command
	return agentcontrol.Result{RequestID: command.Context.RequestID, Status: "applied", PolicyID: "policy-a", Version: 7}
}

func (*recordingPolicyController) CurrentPolicy(context.Context) (agentcontrol.PolicySnapshot, error) {
	return agentcontrol.PolicySnapshot{}, nil
}

func TestHandlerApplyPolicyUsesStandaloneSource(t *testing.T) {
	controller := &recordingPolicyController{}
	handler := NewHandler(Dependencies{Policy: controller})
	ack, err := handler.ApplyPolicy(t.Context(), &controlplanev1.ApplyPolicyRequest{
		Context:    &controlplanev1.RequestContext{RequestId: "request-a", TenantId: "tenant-a", AgentId: "agent-a"},
		PolicyType: "endpoint", PolicyJson: `{"policy_id":"policy-a"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if controller.command.Source != agentcontrol.PolicySourceStandalone || controller.command.Context.RequestID != "request-a" || ack.GetPolicyVersion() != 7 {
		t.Fatalf("command=%+v ack=%+v", controller.command, ack)
	}
}
