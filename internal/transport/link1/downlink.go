package link1

import (
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

const (
	DownlinkPolicyUpdate    = "policy_update"
	DownlinkResponseCommand = "response_command"
)

type DownlinkFrame struct {
	Type    string                 `json:"type"`
	Version uint64                 `json:"version"`
	Payload map[string]interface{} `json:"payload"`
}

func policyUpdateFrame(policy policymodel.Policy) DownlinkFrame {
	return DownlinkFrame{
		Type:    DownlinkPolicyUpdate,
		Version: policy.Version,
		Payload: map[string]interface{}{
			"tenant_id":      policy.TenantID,
			"policy_id":      policy.PolicyID,
			"policy_version": policy.Version,
			"mode":           policy.Mode,
		},
	}
}

func responseCommandFrame(cmd responsemodel.Command) DownlinkFrame {
	return DownlinkFrame{
		Type:    DownlinkResponseCommand,
		Version: 1,
		Payload: map[string]interface{}{
			"response_id": cmd.ResponseID,
			"tenant_id":   cmd.TenantID,
			"agent_id":    cmd.AgentID,
			"action":      cmd.Action,
			"mode":        cmd.Mode,
			"target":      cmd.Target,
			"scope":       cmd.Scope,
			"policy_id":   cmd.PolicyID,
		},
	}
}
