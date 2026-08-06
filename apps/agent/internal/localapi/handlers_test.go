package localapi

import (
	"context"
	"testing"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

type recordingPolicyController struct {
	command  agentcontrol.PolicyCommand
	snapshot agentcontrol.PolicySnapshot
}

type recordingReadServices struct {
	controlplanev1.UnimplementedAgentControlPlaneServiceServer
	statusCalls int
}

func (r *recordingReadServices) Health(context.Context, *controlplanev1.HealthRequest) (*controlplanev1.HealthResponse, error) {
	r.statusCalls++
	return &controlplanev1.HealthResponse{Status: "healthy"}, nil
}

func (r *recordingPolicyController) ApplyPolicy(_ context.Context, command agentcontrol.PolicyCommand) agentcontrol.Result {
	r.command = command
	return agentcontrol.Result{RequestID: command.Context.RequestID, Status: "applied", PolicyID: "policy-a", Version: 7}
}

func (r *recordingPolicyController) CurrentPolicy(context.Context) (agentcontrol.PolicySnapshot, error) {
	return r.snapshot, nil
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
	telemetry := &recordingTelemetryReader{}
	handler := NewHandler(Dependencies{Status: services, Telemetry: telemetry})
	if _, err := handler.Health(t.Context(), &controlplanev1.HealthRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.GetEvent(t.Context(), &controlplanev1.GetEventRequest{EventId: "event-a"}); err != nil {
		t.Fatal(err)
	}
	if services.statusCalls != 1 || telemetry.eventCalls != 1 {
		t.Fatalf("status=%d telemetry=%d", services.statusCalls, telemetry.eventCalls)
	}
}

func TestHandlerCurrentPolicyEncodesCompleteSnapshot(t *testing.T) {
	controller := &recordingPolicyController{snapshot: agentcontrol.PolicySnapshot{
		PolicyID: "policy-a", Version: 7, TenantID: "tenant-a",
		ScopeType: "host", ScopeSelector: "host-a", Mode: "observe",
		EndpointRules: []string{"endpoint-a"}, CloudRules: []string{"cloud-a"},
		Published: true, RawJSON: `{"policy_id":"policy-a"}`,
		Pending: &agentcontrol.PendingPolicy{
			PolicyID: "policy-b", Version: 8, Status: "pending",
			Source: agentcontrol.PolicySourceManaged, Digest: "sha256:pending",
		},
	}}
	handler := NewHandler(Dependencies{Policy: controller})

	response, err := handler.CurrentPolicy(t.Context(), &controlplanev1.CurrentPolicyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetPolicyId() != "policy-a" || response.GetVersion() != 7 || response.GetTenantId() != "tenant-a" ||
		response.GetScope().GetType() != "host" || response.GetScope().GetSelector() != "host-a" || response.GetMode() != "observe" ||
		len(response.GetEndpointRules()) != 1 || len(response.GetCloudRules()) != 1 || !response.GetPublished() || response.GetRawJson() == "" {
		t.Fatalf("response=%+v", response)
	}
	pending := response.GetPendingPolicy()
	if pending.GetPolicyId() != "policy-b" || pending.GetVersion() != 8 || pending.GetStatus() != "pending" || pending.GetSource() != "managed" || pending.GetDigest() != "sha256:pending" {
		t.Fatalf("pending=%+v", pending)
	}
}
