package daemon

import (
	"context"
	"fmt"
	"strings"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/telemetry"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/detection"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/sensors/contract"
	"github.com/sysarmor/sysarmor-next-project/internal/sensors/linux/tetragon"
)

type preparedEndpointPolicy struct {
	policy    agentpolicy.EndpointPolicy
	intent    contract.CollectionIntent
	runtime   policymodel.Policy
	detection *detection.Engine
	report    detection.ApplyReport
	telemetry config.EffectiveTelemetry
	compile   contract.CollectionCompileReport
}

func (s *localControlServer) applyEndpointPolicy(ctx context.Context, req *controlplanev1.ApplyPolicyRequest) *controlplanev1.ControlAck {
	s.runner.detectionUpdateMu.Lock()
	defer s.runner.detectionUpdateMu.Unlock()
	prepared, err := s.prepareEndpointPolicy(req.GetPolicyJson())
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", err.Error())
	}
	if req.GetDryRun() {
		return appliedAck(s.runner.Config, req.GetContext(), prepared.runtime, "validated", "endpoint policy accepted in dry-run", true)
	}
	previousIntent := s.runner.currentCollectionIntent()
	if err := s.runtime.Apply(ctx, prepared.intent); err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "collection", "apply collection policy: "+err.Error())
	}
	if err := s.runner.persistEndpointPolicy(ctx, prepared.policy); err != nil {
		_ = s.runtime.Apply(ctx, previousIntent)
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", "persist endpoint policy: "+err.Error())
	}
	s.commitEndpointPolicy(prepared)
	return appliedAck(s.runner.Config, req.GetContext(), prepared.runtime, prepared.report.Status, "endpoint policy applied", true)
}

func (s *localControlServer) prepareEndpointPolicy(document string) (preparedEndpointPolicy, error) {
	policy, err := agentpolicy.ParseEndpointPolicy([]byte(document))
	if err != nil {
		return preparedEndpointPolicy{}, err
	}
	collection, expansion, err := agentpolicy.ExpandCollectionPolicyRefs(policy.Collection, collectionContentSnapshot(s.runner.contentStore().Snapshot()))
	if err != nil {
		return preparedEndpointPolicy{}, err
	}
	policy.Collection = collection
	intent, err := agentpolicy.CollectionPolicyIntent(collection)
	if err != nil {
		return preparedEndpointPolicy{}, err
	}
	compile := tetragon.CompileReport(intent)
	compile.ResolvedRefs = expansion.ResolvedRefs
	if len(compile.UnsupportedSelectors) > 0 {
		return preparedEndpointPolicy{}, fmt.Errorf("unsupported collection selectors: %+v", compile.UnsupportedSelectors)
	}
	effective, err := config.ResolveTelemetry(s.runner.Config.Telemetry, &policy.Telemetry)
	if err != nil {
		return preparedEndpointPolicy{}, err
	}
	runtimePolicy := endpointRuntimePolicy(s.runner.Config.Agent.TenantID, policy)
	engine, report := detection.NewWithRuntimeLimits(runtimePolicy.Detection, s.runner.withCollectionCapabilities(intent), s.runner.detectionContentSnapshot(), s.runner.detectionLimits())
	if report.Status == "rejected" {
		return preparedEndpointPolicy{}, errRejectedDetection(report.Details)
	}
	return preparedEndpointPolicy{policy: policy, intent: intent, runtime: runtimePolicy, detection: engine, report: report, telemetry: effective, compile: compile}, nil
}

func (s *localControlServer) commitEndpointPolicy(prepared preparedEndpointPolicy) {
	s.runner.setCollectionIntent(prepared.intent)
	s.runner.setPolicy(prepared.runtime)
	s.runner.setDetection(prepared.detection)
	s.runner.setDetectionStatus(prepared.runtime, prepared.report, s.runner.contentStore().Snapshot())
	s.runner.setEffectiveTelemetry(prepared.telemetry)
	if s.batcher != nil {
		s.batcher.Reconfigure(telemetry.BatchSettings{MaxItems: prepared.telemetry.MaxBatchItems, MaxBytes: prepared.telemetry.MaxBatchBytes, FlushInterval: prepared.telemetry.FlushInterval})
	}
}

func endpointRuntimePolicy(tenantID string, endpoint agentpolicy.EndpointPolicy) policymodel.Policy {
	policy := policymodel.DefaultPolicy(tenantID)
	policy.PolicyID = endpoint.PolicyID
	policy.Version = endpoint.Version
	policy.Detection = &endpoint.Detection
	policy.Telemetry = &endpoint.Telemetry
	policy.Response = endpoint.Response
	return policy
}

func errRejectedDetection(details []string) error {
	return &policyPreparationError{message: "detection policy rejected: " + strings.Join(details, "; ")}
}

type policyPreparationError struct{ message string }

func (e *policyPreparationError) Error() string { return e.message }
