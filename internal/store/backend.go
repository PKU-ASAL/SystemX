package store

import (
	"context"

	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

// Backend is the durable persistence boundary for platform state. A nil Backend
// means the Store is a pure in-memory/file store. Telemetry (events and signals)
// is intentionally excluded: high-volume telemetry belongs in the search/index
// tier (OpenSearch) and the in-process working set, not the relational state
// store. The Backend only persists low-volume, relational platform state.
type Backend interface {
	SaveState(ctx context.Context, state State) error

	ListIncidents(ctx context.Context, labels LabelSelector) ([]*incidentv1.Incident, error)
	ListResponses(ctx context.Context, tenantID, agentID string) ([]responsemodel.AuditRecord, error)
	ListControlCommands(ctx context.Context, tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error)
	ListPolicies(ctx context.Context, tenantID string) ([]policymodel.Policy, error)
	ListAssignments(ctx context.Context, tenantID, agentID string) ([]policymodel.Assignment, error)
	ListPolicyAudits(ctx context.Context, tenantID, policyID string) ([]policymodel.AuditRecord, error)
	GetPolicy(ctx context.Context, tenantID, policyID string, version uint64) (policymodel.Policy, bool, error)
	EffectivePolicy(ctx context.Context, tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool, error)

	WriteResponse(ctx context.Context, cmd responsemodel.Command, ack *responsemodel.Ack) error
	WritePolicy(ctx context.Context, policy policymodel.Policy) error
	WriteAssignment(ctx context.Context, assignment policymodel.Assignment) error
	WritePolicyAudit(ctx context.Context, audit policymodel.AuditRecord) error
}
