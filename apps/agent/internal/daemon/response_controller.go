package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	controlmodel "github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/incident/v1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/packages/response"
	"github.com/sysarmor/sysarmor-next-project/packages/sensor-sdk/contract"
	"google.golang.org/protobuf/encoding/protojson"
)

type responseController struct {
	runner *AgentRuntime
}

func newResponseController(runner *AgentRuntime) *responseController {
	return &responseController{runner: runner}
}

func (c *responseController) ExecuteResponse(ctx context.Context, cmd responsemodel.Command) responsemodel.Ack {
	cmd = responsemodel.NormalizeCommand(cmd)
	if cmd.Mode == "" {
		cmd.Mode = responsemodel.DefaultMode
	}
	if cmd.Mode != "enforce" {
		return responsemodel.Ack{
			ResponseID:  cmd.ResponseID,
			TenantID:    c.runner.Config.Agent.TenantID,
			AgentID:     c.runner.Config.Agent.ID,
			Accepted:    true,
			ObserveOnly: true,
			Executed:    false,
			Message:     fmt.Sprintf("observe-only response accepted; would execute action=%s target=%s", cmd.Action, cmd.Target),
			ObservedAt:  time.Now().UTC(),
		}
	}
	ack, err := c.runner.Sensor.Enforce(ctx, responsemodel.ToEnforcement(cmd))
	if err != nil {
		ack = contract.UnsupportedAck(responsemodel.ToEnforcement(cmd), err.Error())
	}
	out := responsemodel.FromEnforcementAck(cmd, ack)
	out.TenantID = c.runner.Config.Agent.TenantID
	out.AgentID = c.runner.Config.Agent.ID
	return out
}

func (c *responseController) CollectEvidence(_ context.Context, req controlmodel.EvidencePullbackRequest) controlmodel.EvidencePullbackResult {
	result := controlmodel.EvidencePullbackResult{
		RequestID:  req.RequestID,
		TenantID:   c.runner.Config.Agent.TenantID,
		AgentID:    c.runner.Config.Agent.ID,
		OK:         true,
		Message:    "collected target evidence",
		ObservedAt: time.Now().UTC(),
	}
	if req.Target == "" {
		result.Message = "collected no target evidence"
		return result
	}
	evidence := &incidentv1.EvidenceSubgraph{
		Nodes: []*incidentv1.GraphNode{{
			Id:    req.Target,
			Kind:  evidenceKindFromTarget(req.Target),
			Label: req.Target,
		}},
	}
	data, err := protojson.Marshal(evidence)
	if err != nil {
		result.OK = false
		result.Message = fmt.Sprintf("encode evidence: %v", err)
		return result
	}
	result.Evidence = json.RawMessage(data)
	return result
}

func evidenceKindFromTarget(target string) string {
	if idx := strings.Index(target, ":"); idx > 0 {
		return target[:idx]
	}
	return "entity"
}
