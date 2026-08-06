package remoteapi

import (
	"encoding/json"
	"fmt"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlmodel "github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/packages/response"
	"google.golang.org/protobuf/proto"
)

func policyCommand(frame *controlplanev1.ControlFrame) agentcontrol.PolicyCommand {
	return agentcontrol.PolicyCommand{
		Context: controlRequestContext(frame), PolicyType: "endpoint",
		Document: frame.GetPolicyUpdate().GetRawJson(), Source: agentcontrol.PolicySourceManaged,
	}
}

func contentCommand(frame *controlplanev1.ControlFrame) agentcontrol.ContentCommand {
	req := contentUpdateRequest(frame)
	return agentcontrol.ContentCommand{
		Context: controlRequestContextFromRequest(req.GetContext()), Document: req.GetContentJson(),
		DryRun: req.GetDryRun(), AllowUnsigned: req.GetAllowUnsigned(), Source: agentcontrol.PolicySourceManaged,
	}
}

func contentUpdateRequest(frame *controlplanev1.ControlFrame) *controlplanev1.ApplyContentRequest {
	req := &controlplanev1.ApplyContentRequest{}
	if frame.GetContentUpdate() != nil {
		req = proto.Clone(frame.GetContentUpdate()).(*controlplanev1.ApplyContentRequest)
	}
	if req.Context == nil {
		if frame.GetContext() != nil {
			req.Context = proto.Clone(frame.GetContext()).(*controlplanev1.RequestContext)
		}
	}
	if req.Context == nil {
		req.Context = &controlplanev1.RequestContext{}
	}
	if req.Context.RequestId == "" {
		req.Context.RequestId = frame.GetRequestId()
	}
	if req.Context.TenantId == "" {
		req.Context.TenantId = frame.GetContext().GetTenantId()
	}
	if req.Context.AgentId == "" {
		req.Context.AgentId = frame.GetContext().GetAgentId()
	}
	if req.Context.Scope == nil {
		req.Context.Scope = frame.GetContext().GetScope()
	}
	return req
}

func controlRequestContext(frame *controlplanev1.ControlFrame) agentcontrol.RequestContext {
	req := frame.GetContext()
	requestID := req.GetRequestId()
	if requestID == "" {
		requestID = frame.GetRequestId()
	}
	return agentcontrol.RequestContext{RequestID: requestID, TenantID: req.GetTenantId(), AgentID: req.GetAgentId()}
}

func controlRequestContextFromRequest(req *controlplanev1.RequestContext) agentcontrol.RequestContext {
	return agentcontrol.RequestContext{RequestID: req.GetRequestId(), TenantID: req.GetTenantId(), AgentID: req.GetAgentId()}
}

func controlAck(result agentcontrol.Result) *controlplanev1.ControlAck {
	ack := &controlplanev1.ControlAck{
		RequestId: result.RequestID, TenantId: result.TenantID, AgentId: result.AgentID,
		Status: result.Status, Message: result.Message, PolicyId: result.PolicyID,
		PolicyVersion: result.Version, Details: append([]string(nil), result.Details...), ReportJson: result.ReportJSON,
	}
	for _, section := range result.Sections {
		ack.Sections = append(ack.Sections, &controlplanev1.AppliedSection{
			Name: section.Name, Status: section.Status, Message: section.Message,
			RequiresRestart: section.RequiresRestart, ReportJson: section.ReportJSON,
		})
	}
	return ack
}

func responseCommand(in *controlplanev1.ResponseCommand) (responsemodel.Command, error) {
	if in == nil {
		return responsemodel.Command{}, fmt.Errorf("control frame missing response_command")
	}
	if in.GetRawJson() != "" {
		var cmd responsemodel.Command
		if err := json.Unmarshal([]byte(in.GetRawJson()), &cmd); err != nil {
			return responsemodel.Command{}, fmt.Errorf("decode control frame response command raw_json: %w", err)
		}
		return cmd, nil
	}
	return responsemodel.Command{
		ResponseID: in.GetResponseId(), TenantID: in.GetTenantId(), AgentID: in.GetAgentId(),
		PolicyID: in.GetPolicyId(), PolicyVersion: in.GetPolicyVersion(), SignalID: in.GetSignalId(),
		Labels: cloneLabels(in.GetLabels()), Scope: responsemodel.Scope{Type: in.GetScope().GetType(), Selector: in.GetScope().GetSelector()},
		Action: in.GetAction(), Mode: in.GetMode(), Target: in.GetTarget(), Reason: in.GetReason(), Status: in.GetStatus(), Actor: in.GetActor(),
		ApprovalRequired: in.GetApprovalRequired(), ApprovalStatus: in.GetApprovalStatus(), ApprovalThreshold: in.GetApprovalThreshold(),
		ApprovalRoles: append([]string(nil), in.GetApprovalRoles()...),
	}, nil
}

func evidencePullback(in *controlplanev1.EvidencePullbackRequest) (controlmodel.EvidencePullbackRequest, error) {
	if in == nil {
		return controlmodel.EvidencePullbackRequest{}, fmt.Errorf("control frame missing evidence_pullback")
	}
	if in.GetRawJson() != "" {
		var req controlmodel.EvidencePullbackRequest
		if err := json.Unmarshal([]byte(in.GetRawJson()), &req); err != nil {
			return controlmodel.EvidencePullbackRequest{}, fmt.Errorf("decode control frame evidence pullback raw_json: %w", err)
		}
		return req, nil
	}
	return controlmodel.EvidencePullbackRequest{
		RequestID: in.GetRequestId(), TenantID: in.GetTenantId(), AgentID: in.GetAgentId(), IncidentID: in.GetIncidentId(),
		Labels: cloneLabels(in.GetLabels()), Target: in.GetTarget(), Reason: in.GetReason(), Status: in.GetStatus(), Actor: in.GetActor(),
	}, nil
}

func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
