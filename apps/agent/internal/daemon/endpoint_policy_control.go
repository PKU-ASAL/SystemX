package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/config"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/endpoint/detection"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localstore"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/sensors/linux/tetragon"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/sensors/runtime"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/telemetry"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/packages/policy"
	"github.com/sysarmor/sysarmor-next-project/packages/sensor-sdk/contract"
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
	release, err := s.runner.beginLocalPolicyMutation(ctx, !req.GetDryRun())
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", err.Error())
	}
	defer release()
	return s.applyEndpointPolicyInternal(ctx, req, localstore.PolicySourceStandalone)
}

func (s *localControlServer) applyEndpointPolicyInternal(ctx context.Context, req *controlplanev1.ApplyPolicyRequest, source localstore.PolicySource) *controlplanev1.ControlAck {
	if source == localstore.PolicySourceManaged {
		return s.applyManagedEndpointPolicy(ctx, req)
	}
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
	if _, err := s.policyReconciler().Apply(ctx, prepared.intent); err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "collection", "apply collection policy: "+err.Error())
	}
	if err := s.runner.persistEndpointPolicy(ctx, source, prepared.policy); err != nil {
		_, _ = s.policyReconciler().Apply(ctx, previousIntent)
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", "persist endpoint policy: "+err.Error())
	}
	s.commitEndpointPolicy(prepared)
	return appliedAck(s.runner.Config, req.GetContext(), prepared.runtime, prepared.report.Status, "endpoint policy applied", true)
}

func (s *localControlServer) applyManagedEndpointPolicy(ctx context.Context, req *controlplanev1.ApplyPolicyRequest) *controlplanev1.ControlAck {
	s.runner.policyAuthorityMu.Lock()
	s.runner.detectionUpdateMu.Lock()
	prepared, err := s.prepareEndpointPolicy(req.GetPolicyJson())
	if err == nil && !req.GetDryRun() {
		err = agentpolicy.SaveDesiredManagedEndpointPolicy(ctx, s.runner.localStore, prepared.policy)
		if err == nil {
			s.runner.setPendingEndpointPolicy(prepared)
		}
	}
	s.runner.detectionUpdateMu.Unlock()
	s.runner.policyAuthorityMu.Unlock()
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", err.Error())
	}
	if req.GetDryRun() {
		return appliedAck(s.runner.Config, req.GetContext(), prepared.runtime, "validated", "endpoint policy accepted in dry-run", true)
	}
	if _, err := s.policyReconciler().Apply(ctx, prepared.intent); err != nil {
		return appliedAck(s.runner.Config, req.GetContext(), prepared.runtime, "pending", "endpoint policy persisted; waiting for sensor recovery", false)
	}
	if err := s.runner.completePendingEndpointPolicy(ctx, prepared.intent); err != nil {
		return appliedAck(s.runner.Config, req.GetContext(), prepared.runtime, "pending", "endpoint policy persisted; waiting for durable activation", false)
	}
	return appliedAck(s.runner.Config, req.GetContext(), prepared.runtime, prepared.report.Status, "endpoint policy applied", true)
}

func (r *AgentRuntime) setPendingEndpointPolicy(prepared preparedEndpointPolicy) {
	r.mu.Lock()
	r.pendingEndpoint = &prepared
	r.mu.Unlock()
}

func (r *AgentRuntime) completePendingEndpointPolicy(ctx context.Context, intent contract.CollectionIntent) error {
	r.policyAuthorityMu.Lock()
	defer r.policyAuthorityMu.Unlock()
	r.detectionUpdateMu.Lock()
	defer r.detectionUpdateMu.Unlock()
	return r.completePendingEndpointPolicyLocked(ctx, intent)
}

func (r *AgentRuntime) completePendingEndpointPolicyLocked(ctx context.Context, intent contract.CollectionIntent) error {
	r.mu.RLock()
	pending := r.pendingEndpoint
	r.mu.RUnlock()
	if pending == nil || !reflect.DeepEqual(pending.intent, intent) {
		return nil
	}
	if err := agentpolicy.ActivateManagedEndpointPolicy(ctx, r.localStore, pending.policy); err != nil {
		return err
	}
	server := &localControlServer{runner: r}
	r.setEndpointPolicy(pending.policy)
	server.commitEndpointPolicy(*pending)
	r.mu.Lock()
	r.pendingEndpoint = nil
	r.mu.Unlock()
	return server.promoteManagedAuthority(ctx)
}

func (s *localControlServer) promoteManagedAuthority(ctx context.Context) error {
	if s.runner.localStore == nil {
		return nil
	}
	enrollment, err := s.runner.localStore.Enrollment(ctx)
	if err != nil {
		return fmt.Errorf("read enrollment for managed policy activation: %w", err)
	}
	if enrollment.State != localstore.StateManaged {
		return nil
	}
	s.runner.applyEnrollmentIdentity(enrollment)
	if s.runner.network != nil {
		s.runner.network.PromoteEnrollment(enrollment)
	}
	return nil
}

func (s *localControlServer) restoreStandaloneEndpointPolicy(ctx context.Context) error {
	return s.restoreStandaloneEndpointPolicyWithActivation(ctx, func(ctx context.Context) error {
		return s.runner.localStore.ActivateStandalonePolicy(ctx, "endpoint")
	})
}

func (s *localControlServer) restoreStandaloneEndpointPolicyWithActivation(ctx context.Context, activate func(context.Context) error) error {
	return restoreStandaloneEndpointPolicyWithActivation(ctx, s.runner, s.runtime, activate)
}

func restoreStandaloneEndpointPolicyWithActivation(ctx context.Context, runner *AgentRuntime, runtime sensorruntime.Runtime, activate func(context.Context) error) error {
	s := &localControlServer{runner: runner, runtime: runtime}
	policy, ok, err := agentpolicy.LoadEndpointPolicy(ctx, s.runner.localStore, localstore.PolicySourceStandalone)
	if err != nil {
		return fmt.Errorf("load standalone endpoint policy: %w", err)
	}
	if !ok {
		return fmt.Errorf("standalone endpoint policy is not initialized")
	}
	document, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("encode standalone endpoint policy: %w", err)
	}
	s.runner.detectionUpdateMu.Lock()
	defer s.runner.detectionUpdateMu.Unlock()
	prepared, err := s.prepareEndpointPolicy(string(document))
	if err != nil {
		return fmt.Errorf("prepare standalone endpoint policy: %w", err)
	}
	previous := s.runner.currentCollectionIntent()
	if err := applyAndActivateIntent(ctx, previous, prepared.intent, func(ctx context.Context, intent contract.CollectionIntent) error {
		_, err := s.policyReconciler().Apply(ctx, intent)
		return err
	}, activate); err != nil {
		return fmt.Errorf("restore standalone endpoint policy: %w", err)
	}
	s.runner.setEndpointPolicy(prepared.policy)
	s.commitEndpointPolicy(prepared)
	return nil
}

func applyAndActivateIntent(ctx context.Context, previous, next contract.CollectionIntent, apply func(context.Context, contract.CollectionIntent) error, activate func(context.Context) error) error {
	if err := apply(ctx, next); err != nil {
		return fmt.Errorf("apply sensor intent: %w", err)
	}
	if err := activate(ctx); err != nil {
		if rollbackErr := apply(ctx, previous); rollbackErr != nil {
			return fmt.Errorf("activate policy: %w; rollback sensor intent: %v", err, rollbackErr)
		}
		return fmt.Errorf("activate policy: %w", err)
	}
	return nil
}

func (s *localControlServer) loadPendingManagedEndpointPolicy(ctx context.Context) (preparedEndpointPolicy, bool, error) {
	if s.runner.localStore == nil {
		return preparedEndpointPolicy{}, false, nil
	}
	record, status, ok, err := s.runner.localStore.DesiredPolicy(ctx, "endpoint", localstore.PolicySourceManaged)
	if err != nil || !ok || status != localstore.PolicyStatusPending {
		return preparedEndpointPolicy{}, false, err
	}
	prepared, err := s.prepareEndpointPolicy(string(record.Document))
	if err != nil {
		return preparedEndpointPolicy{}, false, fmt.Errorf("prepare pending managed endpoint policy: %w", err)
	}
	return prepared, true, nil
}

func (r *AgentRuntime) beginLocalPolicyMutation(ctx context.Context, mutation bool) (func(), error) {
	if !mutation {
		return func() {}, nil
	}
	r.policyAuthorityMu.RLock()
	if r.localStore == nil {
		return r.policyAuthorityMu.RUnlock, nil
	}
	enrollment, err := r.localStore.Enrollment(ctx)
	if err != nil {
		r.policyAuthorityMu.RUnlock()
		return nil, fmt.Errorf("read enrollment state: %w", err)
	}
	if enrollment.State != localstore.StateStandalone {
		r.policyAuthorityMu.RUnlock()
		return nil, fmt.Errorf("managed policy authority is active; local policy mutation is not allowed")
	}
	return r.policyAuthorityMu.RUnlock, nil
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
