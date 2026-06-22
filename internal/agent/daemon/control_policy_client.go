package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
)

type ControlPolicyClient struct {
	Manager string
	Token   string
	Timeout time.Duration
	TLS     tlsconfig.ClientConfig
}

func NewControlPolicyClient(manager, token string, timeout time.Duration) *ControlPolicyClient {
	return NewControlPolicyClientWithTLS(manager, token, timeout, tlsconfig.ClientConfig{})
}

func NewControlPolicyClientWithTLS(manager, token string, timeout time.Duration, tlsCfg tlsconfig.ClientConfig) *ControlPolicyClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ControlPolicyClient{Manager: normalizeGRPCAddress(manager), Token: token, Timeout: timeout, TLS: tlsCfg}
}

func (c *ControlPolicyClient) EffectivePolicy(ctx context.Context, req EffectivePolicyRequest) (policymodel.Policy, error) {
	if c == nil {
		return policymodel.Policy{}, fmt.Errorf("control policy client is nil")
	}
	frames, err := controlStreamHello(ctx, c.Manager, c.Token, c.Timeout, c.TLS, req.TenantID, req.AgentID, req.ScopeType, req.ScopeSelector)
	if err != nil {
		return policymodel.Policy{}, err
	}
	for _, frame := range frames {
		if frame.GetType() != "policy_update" {
			continue
		}
		return policyFromControlFrame(frame.GetPolicyUpdate())
	}
	return policymodel.Policy{}, fmt.Errorf("control stream downlink missing policy_update")
}

func policyFromControlFrame(in *controlv1.CurrentPolicyResponse) (policymodel.Policy, error) {
	if in == nil {
		return policymodel.Policy{}, fmt.Errorf("control stream missing policy_update")
	}
	if strings.TrimSpace(in.GetRawJson()) != "" {
		var policy policymodel.Policy
		if err := json.Unmarshal([]byte(in.GetRawJson()), &policy); err != nil {
			return policymodel.Policy{}, fmt.Errorf("decode control stream policy raw_json: %w", err)
		}
		if policy.PolicyID == "" {
			return policymodel.Policy{}, fmt.Errorf("control stream policy missing policy_id")
		}
		return policymodel.Normalize(policy), nil
	}
	policy := policymodel.Policy{
		PolicyID:      in.GetPolicyId(),
		Version:       in.GetVersion(),
		TenantID:      in.GetTenantId(),
		Scope:         policymodel.ScopeSelector{Type: in.GetScope().GetType(), Selector: in.GetScope().GetSelector()},
		EndpointRules: append([]string(nil), in.GetEndpointRules()...),
		CloudRules:    append([]string(nil), in.GetCloudRules()...),
		Mode:          in.GetMode(),
		Published:     in.GetPublished(),
	}
	if policy.PolicyID == "" {
		return policymodel.Policy{}, fmt.Errorf("control stream policy missing policy_id")
	}
	return policymodel.Normalize(policy), nil
}

func normalizeGRPCAddress(manager string) string {
	manager = strings.TrimPrefix(manager, "http://")
	manager = strings.TrimPrefix(manager, "https://")
	return strings.TrimRight(manager, "/")
}
