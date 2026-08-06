package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localstore"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/sensors/runtime"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/telemetry"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/packages/policy"
)

type policyController struct {
	runner  *AgentRuntime
	runtime sensorruntime.Runtime
	batcher *telemetry.Batcher
}

func newPolicyController(runner *AgentRuntime, runtime sensorruntime.Runtime, batcher *telemetry.Batcher) *policyController {
	return &policyController{runner: runner, runtime: runtime, batcher: batcher}
}

func (c *policyController) ApplyPolicy(ctx context.Context, command agentcontrol.PolicyCommand) agentcontrol.Result {
	req := applyPolicyRequest(command)
	if err := c.runner.validateControlContext(req.GetContext()); err != nil {
		return controlResult(rejectedAck(c.runner.Config, req.GetContext(), "policy", err.Error()))
	}
	policyType := strings.TrimSpace(command.PolicyType)
	if policyType == "" {
		policyType = "endpoint"
	}
	if command.Source == agentcontrol.PolicySourceManaged {
		return controlResult(c.applyEndpointPolicyInternal(ctx, req, localstore.PolicySourceManaged))
	}
	return controlResult(c.applyStandalonePolicy(ctx, req, policyType))
}

func (c *policyController) applyStandalonePolicy(ctx context.Context, req *controlplanev1.ApplyPolicyRequest, policyType string) *controlplanev1.ControlAck {
	release, err := c.runner.beginLocalPolicyMutation(ctx, !req.GetDryRun())
	if err != nil {
		return rejectedAck(c.runner.Config, req.GetContext(), policyType, err.Error())
	}
	defer release()
	switch policyType {
	case "collection":
		return c.applyCollectionPolicy(ctx, req)
	case "detection":
		return c.applyDetectionPolicy(ctx, req)
	case "telemetry":
		return c.applyTelemetryPolicy(ctx, req, nil)
	case "endpoint":
		return c.applyEndpointPolicyInternal(ctx, req, localstore.PolicySourceStandalone)
	default:
		return rejectedAck(c.runner.Config, req.GetContext(), "policy", fmt.Sprintf("unsupported policy type %q", policyType))
	}
}

func (c *policyController) CurrentPolicy(ctx context.Context) (agentcontrol.PolicySnapshot, error) {
	policy := policymodel.Normalize(c.runner.activePolicy())
	document := any(policy)
	if endpoint := c.runner.currentEndpointPolicy(); endpoint.PolicyID != "" {
		document = endpoint
		policy.PolicyID = endpoint.PolicyID
		policy.Version = endpoint.Version
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return agentcontrol.PolicySnapshot{}, err
	}
	snapshot := agentcontrol.PolicySnapshot{PolicyID: policy.PolicyID, Version: policy.Version, RawJSON: string(raw)}
	pending, err := c.runner.pendingPolicyStatus(ctx)
	if err != nil {
		return agentcontrol.PolicySnapshot{}, err
	}
	if pending.Status != "" {
		snapshot.Pending = &agentcontrol.PendingPolicy{PolicyID: pending.PolicyID, Version: pending.Version, Status: pending.Status}
	}
	return snapshot, nil
}

func applyPolicyRequest(command agentcontrol.PolicyCommand) *controlplanev1.ApplyPolicyRequest {
	return &controlplanev1.ApplyPolicyRequest{
		Context: &controlplanev1.RequestContext{
			RequestId: command.Context.RequestID,
			TenantId:  command.Context.TenantID,
			AgentId:   command.Context.AgentID,
		},
		PolicyType: command.PolicyType,
		PolicyJson: command.Document,
		DryRun:     command.DryRun,
	}
}

func controlResult(ack *controlplanev1.ControlAck) agentcontrol.Result {
	if ack == nil {
		return agentcontrol.Result{Status: "rejected", Message: "control result is nil"}
	}
	result := agentcontrol.Result{
		RequestID: ack.GetRequestId(), TenantID: ack.GetTenantId(), AgentID: ack.GetAgentId(),
		Status: ack.GetStatus(), Message: ack.GetMessage(), PolicyID: ack.GetPolicyId(),
		Version: ack.GetPolicyVersion(), Details: append([]string(nil), ack.GetDetails()...), ReportJSON: ack.GetReportJson(),
	}
	for _, section := range ack.GetSections() {
		if section.GetRequiresRestart() {
			result.RequiresRestart = true
		}
		result.Sections = append(result.Sections, agentcontrol.SectionResult{
			Name: section.GetName(), Status: section.GetStatus(), Message: section.GetMessage(),
			RequiresRestart: section.GetRequiresRestart(), ReportJSON: section.GetReportJson(),
		})
	}
	return result
}
