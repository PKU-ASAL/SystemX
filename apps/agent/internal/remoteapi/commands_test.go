package remoteapi

import (
	"context"
	"testing"

	agentcontent "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/content"
	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	"google.golang.org/protobuf/proto"
)

type recordingPolicyController struct {
	command agentcontrol.PolicyCommand
}

func (r *recordingPolicyController) ApplyPolicy(_ context.Context, command agentcontrol.PolicyCommand) agentcontrol.Result {
	r.command = command
	return agentcontrol.Result{RequestID: command.Context.RequestID, Status: "applied"}
}

func (*recordingPolicyController) CurrentPolicy(context.Context) (agentcontrol.PolicySnapshot, error) {
	return agentcontrol.PolicySnapshot{}, nil
}

type recordingContentController struct {
	command agentcontrol.ContentCommand
}

func (r *recordingContentController) ApplyContent(_ context.Context, command agentcontrol.ContentCommand) agentcontrol.Result {
	r.command = command
	return agentcontrol.Result{RequestID: command.Context.RequestID, Status: "applied"}
}

func (*recordingContentController) ListContent(context.Context, string) ([]agentcontent.Record, error) {
	return nil, nil
}

func (*recordingContentController) GetContent(context.Context, string) (agentcontent.Record, bool, error) {
	return agentcontent.Record{}, false, nil
}

func TestDispatcherBuildsManagedPolicyCommand(t *testing.T) {
	controller := &recordingPolicyController{}
	dispatcher := NewDispatcher(Dependencies{Policy: controller}, nil)
	result, handled, err := dispatcher.Dispatch(t.Context(), Identity{TenantID: "tenant-a", AgentID: "agent-a"}, &controlplanev1.ControlFrame{
		Type: "policy_update", RequestId: "request-a",
		Context:      &controlplanev1.RequestContext{TenantId: "tenant-a", AgentId: "agent-a"},
		PolicyUpdate: &controlplanev1.CurrentPolicyResponse{RawJson: `{"policy_id":"policy-a"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || controller.command.Source != agentcontrol.PolicySourceManaged || result.Status != "applied" || result.TenantID != "tenant-a" || result.AgentID != "agent-a" {
		t.Fatalf("command=%+v result=%+v handled=%t", controller.command, result, handled)
	}
}

func TestDispatcherBuildsManagedContentCommandWithoutMutatingFrame(t *testing.T) {
	controller := &recordingContentController{}
	dispatcher := NewDispatcher(Dependencies{Content: controller}, nil)
	frame := &controlplanev1.ControlFrame{
		Type: "content_update", RequestId: "request-a",
		Context: &controlplanev1.RequestContext{TenantId: "tenant-a", AgentId: "agent-a"},
		ContentUpdate: &controlplanev1.ApplyContentRequest{
			ContentJson: `{"content_id":"content-a"}`,
			DryRun:      true, AllowUnsigned: true,
		},
	}
	wantFrame := proto.Clone(frame).(*controlplanev1.ControlFrame)

	result, handled, err := dispatcher.Dispatch(t.Context(), Identity{TenantID: "tenant-a", AgentID: "agent-a"}, frame)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || controller.command.Source != agentcontrol.PolicySourceManaged || controller.command.Context.RequestID != "request-a" || result.TenantID != "tenant-a" || result.AgentID != "agent-a" {
		t.Fatalf("command=%+v result=%+v handled=%t", controller.command, result, handled)
	}
	if !proto.Equal(frame, wantFrame) {
		t.Fatalf("dispatcher mutated input frame: got=%v want=%v", frame, wantFrame)
	}
}

func TestControlAckPreservesControlResult(t *testing.T) {
	result := agentcontrol.Result{
		RequestID: "request-a", TenantID: "tenant-a", AgentID: "agent-a",
		Status: "applied", Message: "ok", PolicyID: "policy-a", Version: 7,
		Details: []string{"detail-a"}, ReportJSON: `{"status":"ok"}`,
		Sections: []agentcontrol.SectionResult{{
			Name: "detection", Status: "applied", Message: "ok",
			RequiresRestart: true, ReportJSON: `{"restarted":true}`,
		}},
	}
	want := &controlplanev1.ControlAck{
		RequestId: "request-a", TenantId: "tenant-a", AgentId: "agent-a",
		Status: "applied", Message: "ok", PolicyId: "policy-a", PolicyVersion: 7,
		Details: []string{"detail-a"}, ReportJson: `{"status":"ok"}`,
		Sections: []*controlplanev1.AppliedSection{{
			Name: "detection", Status: "applied", Message: "ok",
			RequiresRestart: true, ReportJson: `{"restarted":true}`,
		}},
	}
	if got := controlAck(result); !proto.Equal(got, want) {
		t.Fatalf("controlAck()=%v want=%v", got, want)
	}
}
