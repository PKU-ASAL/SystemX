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

type DetectionPolicy struct {
	PolicyID      string         `json:"policy_id,omitempty"`
	Version       uint64         `json:"version,omitempty"`
	Mode          string         `json:"mode,omitempty"`
	Scope         ScopeSelector  `json:"scope,omitempty"`
	RuleSets      []RuleSetRef   `json:"rulesets,omitempty"`
	RuleOverrides []RuleOverride `json:"rule_overrides,omitempty"`
	ContextRefs   []ContentRef   `json:"context_refs,omitempty"`
	IOCRefs       []ContentRef   `json:"ioc_refs,omitempty"`
}

type RuleSetRef struct {
	Ref     string   `json:"ref"`
	Version string   `json:"version,omitempty"`
	Enabled *bool    `json:"enabled,omitempty"`
	IOCRefs []string `json:"ioc_refs,omitempty"`
}

type RuleOverride struct {
	RuleID         string             `json:"rule_id"`
	Enabled        *bool              `json:"enabled,omitempty"`
	Mode           string             `json:"mode,omitempty"`
	Severity       string             `json:"severity,omitempty"`
	Scope          ScopeSelector      `json:"scope,omitempty"`
	ResponseIntent *ResponseIntentRef `json:"response_intent,omitempty"`
	Params         map[string]string  `json:"params,omitempty"`
	Reason         string             `json:"reason,omitempty"`
}

type ResponseIntentRef struct {
	Action     string `json:"action,omitempty"`
	Confidence uint32 `json:"confidence,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type ContentRef struct {
	Ref     string `json:"ref"`
	Version string `json:"version,omitempty"`
}

type Policy struct {
	PolicyID      string                   `json:"policy_id"`
	Version       uint64                   `json:"version"`
	TenantID      string                   `json:"tenant_id"`
	Scope         ScopeSelector            `json:"scope,omitempty"`
	Detection     *DetectionPolicy         `json:"detection,omitempty"`
	Upload        *UploadPolicy            `json:"upload,omitempty"`
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

type UploadPolicy struct {
	Transport      string `json:"transport,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	BatchSize      int    `json:"batch_size,omitempty"`
	FlushInterval  string `json:"flush_interval,omitempty"`
	RetryInitial   string `json:"retry_initial,omitempty"`
	RetryMax       string `json:"retry_max,omitempty"`
	RequestTimeout string `json:"request_timeout,omitempty"`
	MaxInflight    int    `json:"max_inflight,omitempty"`
	Compression    string `json:"compression,omitempty"`
	TLSProfile     string `json:"tls_profile,omitempty"`
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
		{RuleID: "payload_lifecycle", Version: 1, Enabled: true, Where: "endpoint", Severity: "high", Tags: []string{"chain", "evidence"}, MITRE: []string{"T1105", "T1574"}},
		{RuleID: "dropped_payload_executed_and_connects", Version: 1, Enabled: true, Where: "cloud", Severity: "critical", Tags: []string{"graph", "cross-lineage"}, MITRE: []string{"T1105", "T1574"}, ResponseIntent: "collect"},
		{RuleID: "web_shell_chain", Version: 1, Enabled: true, Where: "cloud", Severity: "critical", Tags: []string{"web", "c2"}, MITRE: []string{"T1190", "T1059", "T1571"}, ResponseIntent: "collect"},
	}
}

func DefaultPolicy(tenantID string) Policy {
	if tenantID == "" {
		tenantID = "default"
	}
	return Policy{
		PolicyID:   DefaultPolicyID,
		Version:    DefaultPolicyVersion,
		TenantID:   tenantID,
		Detection:  DefaultDetectionPolicy(),
		CloudRules: []string{"dropped_payload_executed_and_connects", "web_shell_chain"},
		Mode:       "observe",
		Converge:   &policyv1.ConvergeParams{Mode: "rarity_structural", CrossLineage: true, TopK: 8, MaxPathHops: 6},
		Response:   responsemodel.DefaultPolicy(),
		Published:  true,
	}
}

func DefaultDetectionPolicy() *DetectionPolicy {
	enabled := true
	disabled := false
	return &DetectionPolicy{
		PolicyID: "default-endpoint-detection",
		Version:  1,
		Mode:     "observe",
		RuleSets: []RuleSetRef{{
			Ref:     "ruleset:endpoint-linux-builtin",
			Version: "1",
			Enabled: &enabled,
		}},
		ContextRefs: []ContentRef{
			{Ref: "ctx:credential-path-prefixes", Version: "builtin"},
			{Ref: "ctx:payload-path-prefixes", Version: "builtin"},
			{Ref: "ctx:trusted-admin-binaries", Version: "builtin"},
		},
		RuleOverrides: []RuleOverride{
			{
				RuleID:  "credential_file_read",
				Enabled: &disabled,
				Reason:  "credential file reads are collected by incident-deep or triggered policies, not the long-running balanced baseline",
			},
		},
		IOCRefs: []ContentRef{
			{Ref: "ioc:c2-port-feed", Version: "builtin"},
		},
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
	if policy.Detection == nil {
		policy.Detection = DefaultDetectionPolicy()
	}
	normalizedDetection := NormalizeDetectionPolicy(*policy.Detection)
	policy.Detection = &normalizedDetection
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

func NormalizeDetectionPolicy(policy DetectionPolicy) DetectionPolicy {
	if policy.PolicyID == "" {
		policy.PolicyID = "default-endpoint-detection"
	}
	if policy.Version == 0 {
		policy.Version = 1
	}
	if policy.Mode == "" {
		policy.Mode = "observe"
	}
	for i := range policy.RuleSets {
		if policy.RuleSets[i].Version == "" {
			policy.RuleSets[i].Version = "latest"
		}
	}
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
