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

type recordingReadServices struct {
	controlplanev1.UnimplementedAgentControlPlaneServiceServer
	statusCalls    int
	telemetryCalls int
	debugCalls     int
}

func (r *recordingReadServices) Health(context.Context, *controlplanev1.HealthRequest) (*controlplanev1.HealthResponse, error) {
	r.statusCalls++
	return &controlplanev1.HealthResponse{Status: "healthy"}, nil
}

func (r *recordingReadServices) GetEvent(context.Context, *controlplanev1.GetEventRequest) (*controlplanev1.EventGetResponse, error) {
	r.telemetryCalls++
	return &controlplanev1.EventGetResponse{}, nil
}

func (r *recordingReadServices) DebugProfile(context.Context, *controlplanev1.DebugProfileRequest) (*controlplanev1.DebugProfileResponse, error) {
	r.debugCalls++
	return &controlplanev1.DebugProfileResponse{ProfileType: "runtime"}, nil
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

func TestHandlerRoutesReadsToNarrowServices(t *testing.T) {
	services := &recordingReadServices{}
	handler := NewHandler(Dependencies{Status: services, Telemetry: services, Debug: services})
	if _, err := handler.Health(t.Context(), &controlplanev1.HealthRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.GetEvent(t.Context(), &controlplanev1.GetEventRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.DebugProfile(t.Context(), &controlplanev1.DebugProfileRequest{}); err != nil {
		t.Fatal(err)
	}
	if services.statusCalls != 1 || services.telemetryCalls != 1 || services.debugCalls != 1 {
		t.Fatalf("status=%d telemetry=%d debug=%d", services.statusCalls, services.telemetryCalls, services.debugCalls)
	}
}
