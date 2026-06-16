package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
)

type PolicyClient struct {
	Manager string
	Token   string
	Client  *http.Client
}

type EffectivePolicyRequest struct {
	TenantID      string
	AgentID       string
	ScopeType     string
	ScopeSelector string
}

func NewPolicyClient(manager, token string, timeout time.Duration) *PolicyClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &PolicyClient{
		Manager: normalizeManagerURL(manager),
		Token:   token,
		Client:  &http.Client{Timeout: timeout},
	}
}

func (c *PolicyClient) EffectivePolicy(ctx context.Context, req EffectivePolicyRequest) (policymodel.Policy, error) {
	if c == nil {
		return policymodel.Policy{}, fmt.Errorf("policy client is nil")
	}
	q := url.Values{}
	q.Set("tenant_id", req.TenantID)
	q.Set("agent_id", req.AgentID)
	q.Set("scope_type", req.ScopeType)
	q.Set("scope_selector", req.ScopeSelector)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Manager+"/api/v1/effective-policy?"+q.Encode(), nil)
	if err != nil {
		return policymodel.Policy{}, err
	}
	if c.Token != "" {
		httpReq.Header.Set("X-SysArmor-Agent-Token", c.Token)
	}
	resp, err := c.client().Do(httpReq)
	if err != nil {
		return policymodel.Policy{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return policymodel.Policy{}, fmt.Errorf("effective policy fetch failed: %s: %s", resp.Status, string(body))
	}
	var policy policymodel.Policy
	if err := json.Unmarshal(body, &policy); err != nil {
		return policymodel.Policy{}, err
	}
	if policy.PolicyID == "" {
		return policymodel.Policy{}, fmt.Errorf("effective policy missing policy_id")
	}
	return policy, nil
}

func (c *PolicyClient) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return http.DefaultClient
}

func normalizeManagerURL(manager string) string {
	if strings.HasPrefix(manager, "http://") || strings.HasPrefix(manager, "https://") {
		return strings.TrimRight(manager, "/")
	}
	return "http://" + strings.TrimRight(manager, "/")
}
