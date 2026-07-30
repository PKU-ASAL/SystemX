package daemon

import (
	"context"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/sensors/contract"
)

func (r *AgentRuntime) loadStartupPolicy(ctx context.Context) (contract.CollectionIntent, policymodel.Policy, config.EffectiveTelemetry, error) {
	if r.localStore == nil {
		return r.loadLegacyRuntimePolicy()
	}
	endpoint, err := agentpolicy.LoadEffectiveEndpointPolicy(ctx, r.localStore, r.Config.Policy.Path)
	if err != nil {
		return contract.CollectionIntent{}, policymodel.Policy{}, config.EffectiveTelemetry{}, err
	}
	intent, err := agentpolicy.CollectionPolicyIntent(endpoint.Collection)
	if err != nil {
		return contract.CollectionIntent{}, policymodel.Policy{}, config.EffectiveTelemetry{}, err
	}
	effectiveTelemetry, err := config.ResolveTelemetry(r.Config.Telemetry, &endpoint.Telemetry)
	if err != nil {
		return contract.CollectionIntent{}, policymodel.Policy{}, config.EffectiveTelemetry{}, err
	}
	policy := policymodel.DefaultPolicy(r.Config.Agent.TenantID)
	policy.PolicyID = endpoint.PolicyID
	policy.Version = endpoint.Version
	policy.Detection = &endpoint.Detection
	policy.Telemetry = &endpoint.Telemetry
	policy.Response = endpoint.Response
	r.setEndpointPolicy(endpoint)
	r.setEffectiveTelemetry(effectiveTelemetry)
	return intent, policy, effectiveTelemetry, nil
}

func (r *AgentRuntime) setEffectiveTelemetry(value config.EffectiveTelemetry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.effectiveTelemetry = value
}

func (r *AgentRuntime) currentEffectiveTelemetry() config.EffectiveTelemetry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.effectiveTelemetry
}

func (r *AgentRuntime) setEndpointPolicy(policy agentpolicy.EndpointPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpointPolicy = policy
}

func (r *AgentRuntime) currentEndpointPolicy() agentpolicy.EndpointPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.endpointPolicy
}

func (r *AgentRuntime) persistEndpointPolicy(ctx context.Context, policy agentpolicy.EndpointPolicy) error {
	if r.localStore == nil {
		r.setEndpointPolicy(policy)
		return nil
	}
	if err := agentpolicy.SaveEffectiveEndpointPolicy(ctx, r.localStore, policy); err != nil {
		return err
	}
	r.setEndpointPolicy(policy)
	return nil
}

func (r *AgentRuntime) loadLegacyRuntimePolicy() (contract.CollectionIntent, policymodel.Policy, config.EffectiveTelemetry, error) {
	intent, err := agentpolicy.LoadCollectionIntent(r.Config.Sensor.PolicyPath, r.Config.Sensor.ObserveOnly)
	if err != nil {
		return contract.CollectionIntent{}, policymodel.Policy{}, config.EffectiveTelemetry{}, err
	}
	policy := r.activePolicy()
	effectiveTelemetry, err := config.ResolveTelemetry(r.Config.Telemetry, policy.Telemetry)
	return intent, policy, effectiveTelemetry, err
}
