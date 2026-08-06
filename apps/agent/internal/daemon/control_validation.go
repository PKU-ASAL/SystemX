package daemon

import (
	"fmt"
	"strings"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func (s *localControlServer) validateContext(ctx *controlplanev1.RequestContext) error {
	return s.runner.validateControlContext(ctx)
}

func (r *AgentRuntime) validateControlContext(ctx *controlplanev1.RequestContext) error {
	if ctx == nil {
		return nil
	}
	identity := r.currentIdentity()
	if tenantID := strings.TrimSpace(ctx.GetTenantId()); tenantID != "" && tenantID != identity.TenantID {
		return fmt.Errorf("tenant mismatch: request=%s agent=%s", tenantID, identity.TenantID)
	}
	if agentID := strings.TrimSpace(ctx.GetAgentId()); agentID != "" && agentID != identity.AgentID {
		return fmt.Errorf("agent mismatch: request=%s agent=%s", agentID, identity.AgentID)
	}
	return nil
}
