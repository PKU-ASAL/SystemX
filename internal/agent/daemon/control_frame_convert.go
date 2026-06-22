package daemon

import (
	"encoding/json"
	"fmt"
	"strings"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

func policyFromControlFrame(in *controlplanev1.CurrentPolicyResponse) (policymodel.Policy, error) {
	if in == nil {
		return policymodel.Policy{}, fmt.Errorf("control frame missing policy_update")
	}
	if strings.TrimSpace(in.GetRawJson()) != "" {
		var policy policymodel.Policy
		if err := json.Unmarshal([]byte(in.GetRawJson()), &policy); err != nil {
			return policymodel.Policy{}, fmt.Errorf("decode control frame policy raw_json: %w", err)
		}
		if policy.PolicyID == "" {
			return policymodel.Policy{}, fmt.Errorf("control frame policy missing policy_id")
		}
		return policymodel.Normalize(policy), nil
	}
	policy := policymodel.Policy{
		PolicyID:      in.GetPolicyId(),
		Version:       in.GetVersion(),
		TenantID:      in.GetTenantId(),
		Scope:         policymodel.ScopeSelector{Type: in.GetScope().GetType(), Selector: in.GetScope().GetSelector()},
		EndpointRules: append([]string(nil), in.GetEndpointRules()...),
		CloudRules:    append([]string(nil), in.GetCloudRules()...),
		Mode:          in.GetMode(),
		Published:     in.GetPublished(),
	}
	if policy.PolicyID == "" {
		return policymodel.Policy{}, fmt.Errorf("control frame policy missing policy_id")
	}
	return policymodel.Normalize(policy), nil
}

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
		Scenario:          in.GetScenario(),
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
		Scenario:   in.GetScenario(),
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
