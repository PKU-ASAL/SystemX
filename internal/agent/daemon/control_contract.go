package daemon

type EffectivePolicyRequest struct {
	TenantID      string
	AgentID       string
	ScopeType     string
	ScopeSelector string
}
