package daemon

import (
	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
)

type runtimeIdentity struct {
	AgentID  string
	HostID   string
	TenantID string
}

func (r *AgentRuntime) setRuntimeIdentity(identity runtimeIdentity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.identity = identity
	if r.standaloneIdentity.AgentID == "" {
		r.standaloneIdentity = identity
	}
	if r.normalizer != nil {
		r.normalizer.SetIdentity(identity.AgentID, identity.HostID, identity.TenantID)
	}
}

func (r *AgentRuntime) currentIdentity() runtimeIdentity {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.identity.AgentID != "" {
		return r.identity
	}
	return runtimeIdentity{AgentID: r.Config.Agent.ID, HostID: r.Config.Agent.HostID, TenantID: r.Config.Agent.TenantID}
}

func (r *AgentRuntime) applyEnrollmentIdentity(enrollment localstore.Enrollment) {
	identity := r.standaloneRuntimeIdentity()
	if enrollment.State == localstore.StateManaged {
		identity.AgentID = enrollment.AgentID
		identity.TenantID = enrollment.TenantID
	}
	r.setRuntimeIdentity(identity)
}

func (r *AgentRuntime) standaloneRuntimeIdentity() runtimeIdentity {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.standaloneIdentity.AgentID != "" {
		return r.standaloneIdentity
	}
	return runtimeIdentity{AgentID: r.Config.Agent.ID, HostID: r.Config.Agent.HostID, TenantID: r.Config.Agent.TenantID}
}

func (r *AgentRuntime) setNormalizer(normalizer *normalize.Normalizer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.normalizer = normalizer
	identity := r.identity
	if identity.AgentID == "" {
		identity = runtimeIdentity{AgentID: r.Config.Agent.ID, HostID: r.Config.Agent.HostID, TenantID: r.Config.Agent.TenantID}
	}
	normalizer.SetIdentity(identity.AgentID, identity.HostID, identity.TenantID)
}
