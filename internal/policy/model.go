package policy

import (
	"sort"
	"time"

	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

const (
	DefaultPolicyID      = "default-edr-policy"
	DefaultPolicyVersion = uint64(1)
)

type RuleContent struct {
	RuleID         string   `json:"rule_id"`
	Version        uint64   `json:"version"`
	Enabled        bool     `json:"enabled"`
	Where          string   `json:"where"`
	Severity       string   `json:"severity,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	MITRE          []string `json:"mitre,omitempty"`
	ResponseIntent string   `json:"response_intent,omitempty"`
}

type Policy struct {
	PolicyID      string                   `json:"policy_id"`
	Version       uint64                   `json:"version"`
	TenantID      string                   `json:"tenant_id"`
	Scope         ScopeSelector            `json:"scope,omitempty"`
	EndpointRules []string                 `json:"endpoint_rules,omitempty"`
	CloudRules    []string                 `json:"cloud_rules,omitempty"`
	Mode          string                   `json:"mode,omitempty"`
	Converge      *policyv1.ConvergeParams `json:"converge,omitempty"`
	Rarity        *policyv1.RarityParams   `json:"rarity,omitempty"`
	Response      responsemodel.Policy     `json:"response_policy,omitempty"`
	Published     bool                     `json:"published"`
	CreatedAt     time.Time                `json:"created_at,omitempty"`
	UpdatedAt     time.Time                `json:"updated_at,omitempty"`
}

type ScopeSelector struct {
	Type     string `json:"type,omitempty"`
	Selector string `json:"selector,omitempty"`
}

type Assignment struct {
	AssignmentID  string        `json:"assignment_id"`
	TenantID      string        `json:"tenant_id"`
	AgentID       string        `json:"agent_id,omitempty"`
	Scope         ScopeSelector `json:"scope,omitempty"`
	PolicyID      string        `json:"policy_id"`
	PolicyVersion uint64        `json:"policy_version"`
	CreatedAt     time.Time     `json:"created_at,omitempty"`
	UpdatedAt     time.Time     `json:"updated_at,omitempty"`
}

type AuditRecord struct {
	AuditID       string    `json:"audit_id"`
	TenantID      string    `json:"tenant_id"`
	Action        string    `json:"action"`
	PolicyID      string    `json:"policy_id,omitempty"`
	PolicyVersion uint64    `json:"policy_version,omitempty"`
	AssignmentID  string    `json:"assignment_id,omitempty"`
	Actor         string    `json:"actor,omitempty"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
}

func DefaultRules() []RuleContent {
	return []RuleContent{
		{RuleID: "web_runtime_spawns_shell", Version: 1, Enabled: true, Where: "endpoint", Severity: "high", Tags: []string{"process", "web"}, MITRE: []string{"T1059"}},
		{RuleID: "download_by_lolbin", Version: 1, Enabled: true, Where: "endpoint", Severity: "medium", Tags: []string{"download"}, MITRE: []string{"T1105"}},
		{RuleID: "payload_dropped", Version: 1, Enabled: true, Where: "endpoint", Severity: "high", Tags: []string{"file", "drop"}, MITRE: []string{"T1105"}},
		{RuleID: "reverse_shell_pattern", Version: 1, Enabled: true, Where: "endpoint", Severity: "critical", Tags: []string{"c2"}, MITRE: []string{"T1571"}, ResponseIntent: "collect"},
		{RuleID: "suspicious_exec_connect", Version: 1, Enabled: true, Where: "endpoint", Severity: "high", Tags: []string{"exec", "network"}, MITRE: []string{"T1574"}},
		{RuleID: "dropped_payload_executed_and_connects", Version: 1, Enabled: true, Where: "cloud", Severity: "critical", Tags: []string{"graph", "cross-lineage"}, MITRE: []string{"T1105", "T1574"}, ResponseIntent: "collect"},
		{RuleID: "web_shell_chain", Version: 1, Enabled: true, Where: "cloud", Severity: "critical", Tags: []string{"web", "c2"}, MITRE: []string{"T1190", "T1059", "T1571"}, ResponseIntent: "collect"},
	}
}

func DefaultPolicy(tenantID string) Policy {
	if tenantID == "" {
		tenantID = "default"
	}
	return Policy{
		PolicyID:      DefaultPolicyID,
		Version:       DefaultPolicyVersion,
		TenantID:      tenantID,
		EndpointRules: []string{"web_runtime_spawns_shell", "download_by_lolbin", "payload_dropped", "reverse_shell_pattern", "suspicious_exec_connect"},
		CloudRules:    []string{"dropped_payload_executed_and_connects", "web_shell_chain"},
		Mode:          "observe",
		Converge:      &policyv1.ConvergeParams{Mode: "rarity_structural", CrossLineage: true, TopK: 8, MaxPathHops: 6},
		Response:      responsemodel.DefaultPolicy(),
		Published:     true,
	}
}

func (p Policy) DetectionPolicy() *policyv1.DetectionPolicy {
	return &policyv1.DetectionPolicy{
		EndpointRules: append([]string(nil), p.EndpointRules...),
		CloudRules:    append([]string(nil), p.CloudRules...),
		Converge:      p.Converge,
		Rarity:        p.Rarity,
	}
}

func Normalize(policy Policy) Policy {
	if policy.TenantID == "" {
		policy.TenantID = "default"
	}
	if policy.Version == 0 {
		policy.Version = 1
	}
	if policy.Mode == "" {
		policy.Mode = "observe"
	}
	if policy.Converge == nil {
		policy.Converge = &policyv1.ConvergeParams{Mode: "rarity_structural", CrossLineage: true, TopK: 8, MaxPathHops: 6}
	}
	if len(policy.Response.AllowedActions) == 0 && len(policy.Response.AllowedModes) == 0 {
		policy.Response = responsemodel.DefaultPolicy()
	}
	now := time.Now().UTC()
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = now
	}
	policy.UpdatedAt = now
	return policy
}

func RuleIDs(rules []RuleContent, where string) []string {
	var out []string
	for _, rule := range rules {
		if rule.Enabled && (where == "" || rule.Where == where) {
			out = append(out, rule.RuleID)
		}
	}
	sort.Strings(out)
	return out
}
