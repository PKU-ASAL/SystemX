package daemon

import (
	"encoding/json"
	"fmt"
	"strings"

	controlmodel "github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/packages/response"
)

func responseCommandFromControl(in *controlplanev1.ResponseCommand) (responsemodel.Command, error) {
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
		ResponseID:        in.GetResponseId(),
		TenantID:          in.GetTenantId(),
		AgentID:           in.GetAgentId(),
		PolicyID:          in.GetPolicyId(),
		PolicyVersion:     in.GetPolicyVersion(),
		SignalID:          in.GetSignalId(),
		Labels:            cloneControlLabels(in.GetLabels()),
		Scope:             responsemodel.Scope{Type: in.GetScope().GetType(), Selector: in.GetScope().GetSelector()},
		Action:            in.GetAction(),
		Mode:              in.GetMode(),
		Target:            in.GetTarget(),
		Reason:            in.GetReason(),
		Status:            in.GetStatus(),
		Actor:             in.GetActor(),
		ApprovalRequired:  in.GetApprovalRequired(),
		ApprovalStatus:    in.GetApprovalStatus(),
		ApprovalThreshold: in.GetApprovalThreshold(),
		ApprovalRoles:     append([]string(nil), in.GetApprovalRoles()...),
	}, nil
}

func evidencePullbackFromControl(in *controlplanev1.EvidencePullbackRequest) (controlmodel.EvidencePullbackRequest, error) {
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
		RequestID:  in.GetRequestId(),
		TenantID:   in.GetTenantId(),
		AgentID:    in.GetAgentId(),
		IncidentID: in.GetIncidentId(),
		Labels:     cloneControlLabels(in.GetLabels()),
		Target:     in.GetTarget(),
		Reason:     in.GetReason(),
		Status:     in.GetStatus(),
		Actor:      in.GetActor(),
	}, nil
}

func normalizeGRPCAddress(manager string) string {
	manager = strings.TrimPrefix(manager, "http://")
	manager = strings.TrimPrefix(manager, "https://")
	return strings.TrimRight(manager, "/")
}

func cloneControlLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
