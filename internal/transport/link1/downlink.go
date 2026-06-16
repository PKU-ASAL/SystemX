package link1

import (
	"encoding/json"

	link1model "github.com/sysarmor/sysarmor-next-project/internal/link1"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

const (
	DownlinkPolicyUpdate     = "policy_update"
	DownlinkResponseCommand  = "response_command"
	DownlinkEvidencePullback = "evidence_pullback"
	UplinkUpload             = "upload"
	UplinkHealth             = "health"
	UplinkAck                = "ack"
	UplinkError              = "error"
)

type DownlinkFrame struct {
	Type    string                 `json:"type"`
	Version uint64                 `json:"version"`
	Payload map[string]interface{} `json:"payload"`
}

type UplinkFrame struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type UplinkFrameResult struct {
	Type            string `json:"type"`
	OK              bool   `json:"ok"`
	Message         string `json:"message,omitempty"`
	BatchID         string `json:"batch_id,omitempty"`
	AcceptedEvents  int    `json:"accepted_events,omitempty"`
	AcceptedSignals int    `json:"accepted_signals,omitempty"`
}

type ResumeCursor struct {
	TenantID     string `json:"tenant_id"`
	AgentID      string `json:"agent_id"`
	SessionID    string `json:"session_id,omitempty"`
	ResumeCursor string `json:"resume_cursor,omitempty"`
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

func evidencePullbackFrame(req link1model.EvidencePullbackRequest) DownlinkFrame {
	return DownlinkFrame{
		Type:    DownlinkEvidencePullback,
		Version: 1,
		Payload: map[string]interface{}{
			"request_id":  req.RequestID,
			"tenant_id":   req.TenantID,
			"agent_id":    req.AgentID,
			"incident_id": req.IncidentID,
			"scenario":    req.Scenario,
			"target":      req.Target,
			"reason":      req.Reason,
		},
	}
}
