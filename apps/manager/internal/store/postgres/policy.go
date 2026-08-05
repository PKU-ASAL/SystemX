package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/store"
	"github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/packages/policy"
)

func (b *tableBackend) ListPolicies(ctx context.Context, tenantID string) ([]policymodel.Policy, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryPolicies(ctx, b.db, tenantID)
}

func (b *tableBackend) ListAssignments(ctx context.Context, tenantID, agentID string) ([]policymodel.Assignment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryPolicyAssignments(ctx, b.db, tenantID, agentID)
}

func queryPolicies(ctx context.Context, db sqlExecutor, tenantID string) ([]policymodel.Policy, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM policies
WHERE ($1 = '' OR tenant_id = $1)
ORDER BY tenant_id ASC, policy_id ASC, version ASC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query postgres policies: %w", err)
	}
	defer rows.Close()
	out := []policymodel.Policy{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres policy: %w", err)
		}
		var policy policymodel.Policy
		if err := json.Unmarshal(raw, &policy); err != nil {
			return nil, fmt.Errorf("decode postgres policy: %w", err)
		}
		out = append(out, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres policies: %w", err)
	}
	return out, nil
}

func (b *tableBackend) ListPolicyAudits(ctx context.Context, tenantID, policyID string) ([]policymodel.AuditRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryPolicyAudits(ctx, b.db, tenantID, policyID)
}

func (b *tableBackend) GetPolicy(ctx context.Context, tenantID, policyID string, version uint64) (policymodel.Policy, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryPolicy(ctx, b.db, tenantID, policyID, version)
}

func (b *tableBackend) EffectivePolicy(ctx context.Context, tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryEffectivePolicy(ctx, b.db, tenantID, agentID, scopeType, scopeSelector)
}

func (b *tableBackend) WritePolicy(ctx context.Context, policy policymodel.Policy) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertPolicy(ctx, b.db, policy)
}

func (b *tableBackend) WriteAssignment(ctx context.Context, assignment policymodel.Assignment) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertPolicyAssignment(ctx, b.db, assignment)
}

func (b *tableBackend) WritePolicyAudit(ctx context.Context, audit policymodel.AuditRecord) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertPolicyAudit(ctx, b.db, audit)
}

func (b *tableBackend) CommitPolicyPublication(ctx context.Context, policy policymodel.Policy, audit policymodel.AuditRecord) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return b.withTransaction(ctx, func(tx *sql.Tx) error {
		if err := upsertPolicy(ctx, tx, policy); err != nil {
			return err
		}
		return upsertPolicyAudit(ctx, tx, audit)
	})
}

func (b *tableBackend) CommitPolicyAssignment(ctx context.Context, assignment policymodel.Assignment, audit policymodel.AuditRecord, command *controlmodel.ControlCommand) (*controlmodel.ControlCommand, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	persistedCommand := command
	err := b.withTransaction(ctx, func(tx *sql.Tx) error {
		if err := upsertPolicyAssignment(ctx, tx, assignment); err != nil {
			return err
		}
		if err := upsertPolicyAudit(ctx, tx, audit); err != nil {
			return err
		}
		if command != nil {
			created, err := insertControlCommand(ctx, tx, *command)
			if err != nil {
				return err
			}
			if !created {
				existing, err := findControlCommand(ctx, tx, command.TenantID, command.CommandID)
				if err != nil {
					return err
				}
				if existing == nil || !sameControlCommandRequest(*existing, *command) {
					return fmt.Errorf("%w: control command %s has different payload", store.ErrConflict, command.CommandID)
				}
				persistedCommand = existing
			}
		}
		return nil
	})
	return persistedCommand, err
}

func queryPolicy(ctx context.Context, db sqlExecutor, tenantID, policyID string, version uint64) (policymodel.Policy, bool, error) {
	if policyID == "" {
		return policymodel.Policy{}, false, nil
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM policies
WHERE ($1 = '' OR tenant_id = $1)
  AND policy_id = $2
  AND ($3 = 0 OR version = $3)
ORDER BY version DESC
LIMIT 1
`, tenantID, policyID, version)
	if err != nil {
		return policymodel.Policy{}, false, fmt.Errorf("query postgres policy: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return policymodel.Policy{}, false, fmt.Errorf("iterate postgres policy: %w", err)
		}
		return policymodel.Policy{}, false, nil
	}
	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		return policymodel.Policy{}, false, fmt.Errorf("scan postgres policy: %w", err)
	}
	var policy policymodel.Policy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return policymodel.Policy{}, false, fmt.Errorf("decode postgres policy: %w", err)
	}
	return policy, true, nil
}

func queryPublishedPolicy(ctx context.Context, db sqlExecutor, tenantID, policyID string, version uint64) (policymodel.Policy, bool, error) {
	if policyID == "" {
		return policymodel.Policy{}, false, nil
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM policies
WHERE ($1 = '' OR tenant_id = $1)
  AND policy_id = $2
  AND ($3 = 0 OR version = $3)
ORDER BY version DESC
`, tenantID, policyID, version)
	if err != nil {
		return policymodel.Policy{}, false, fmt.Errorf("query postgres published policy: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return policymodel.Policy{}, false, fmt.Errorf("scan postgres published policy: %w", err)
		}
		var policy policymodel.Policy
		if err := json.Unmarshal(raw, &policy); err != nil {
			return policymodel.Policy{}, false, fmt.Errorf("decode postgres published policy: %w", err)
		}
		if policy.Published {
			return policy, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return policymodel.Policy{}, false, fmt.Errorf("iterate postgres published policy: %w", err)
	}
	return policymodel.Policy{}, false, nil
}

func queryEffectivePolicy(ctx context.Context, db sqlExecutor, tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool, error) {
	assignments, err := queryPolicyAssignments(ctx, db, tenantID, "")
	if err != nil {
		return policymodel.Policy{}, false, err
	}
	var best policymodel.Assignment
	bestRank := -1
	for _, assignment := range assignments {
		if tenantID != "" && assignment.TenantID != tenantID {
			continue
		}
		rank := store.AssignmentRank(assignment, agentID, scopeType, scopeSelector)
		if rank > bestRank {
			best = assignment
			bestRank = rank
		}
	}
	if bestRank >= 0 {
		return queryPublishedPolicy(ctx, db, best.TenantID, best.PolicyID, best.PolicyVersion)
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if policy, ok, err := queryPublishedPolicy(ctx, db, tenantID, policymodel.DefaultPolicyID, 0); err != nil || ok {
		return policy, ok, err
	}
	return policymodel.ManagerDefaultPolicy(tenantID), true, nil
}

func queryPolicyAssignments(ctx context.Context, db sqlExecutor, tenantID, agentID string) ([]policymodel.Assignment, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM policy_assignments
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR agent_id = $2)
ORDER BY tenant_id ASC, assignment_id ASC
`, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("query postgres policy assignments: %w", err)
	}
	defer rows.Close()
	out := []policymodel.Assignment{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres policy assignment: %w", err)
		}
		var assignment policymodel.Assignment
		if err := json.Unmarshal(raw, &assignment); err != nil {
			return nil, fmt.Errorf("decode postgres policy assignment: %w", err)
		}
		out = append(out, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres policy assignments: %w", err)
	}
	return out, nil
}

func projectRules(ctx context.Context, db sqlExecutor, rules []policymodel.RuleContent) error {
	for _, rule := range rules {
		if rule.RuleID == "" || rule.Version == 0 {
			continue
		}
		data, err := json.Marshal(rule)
		if err != nil {
			return fmt.Errorf("encode rule projection: %w", err)
		}
		_, err = db.ExecContext(ctx, `
INSERT INTO rules (tenant_id, rule_id, version, rule_where, enabled, severity, tags, mitre, data)
VALUES ($1, $2, $3, $4, $5, $6, string_to_array($7, E'\x1f'), string_to_array($8, E'\x1f'), $9)
ON CONFLICT (tenant_id, rule_id, version) DO UPDATE SET
  rule_where = EXCLUDED.rule_where,
  enabled = EXCLUDED.enabled,
  severity = EXCLUDED.severity,
  tags = EXCLUDED.tags,
  mitre = EXCLUDED.mitre,
  data = EXCLUDED.data
`, "default", rule.RuleID, rule.Version, rule.Where, rule.Enabled, severityRank(rule.Severity), joinTextArray(rule.Tags), joinTextArray(rule.MITRE), data)
		if err != nil {
			return fmt.Errorf("project rule: %w", err)
		}
	}
	return nil
}

func queryPolicyAudits(ctx context.Context, db sqlExecutor, tenantID, policyID string) ([]policymodel.AuditRecord, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM policy_audit
WHERE tenant_id = $1 AND ($2 = '' OR policy_id = $2)
ORDER BY created_at ASC, audit_id ASC
`, tenantID, policyID)
	if err != nil {
		return nil, fmt.Errorf("query postgres policy audits: %w", err)
	}
	defer rows.Close()
	out := []policymodel.AuditRecord{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres policy audit: %w", err)
		}
		var audit policymodel.AuditRecord
		if err := json.Unmarshal(raw, &audit); err != nil {
			return nil, fmt.Errorf("decode postgres policy audit: %w", err)
		}
		out = append(out, audit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres policy audits: %w", err)
	}
	return out, nil
}

func upsertPolicy(ctx context.Context, db sqlExecutor, policy policymodel.Policy) error {
	if policy.PolicyID == "" || policy.Version == 0 {
		return nil
	}
	tenantID := policy.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("encode policy projection: %w", err)
	}
	createdAt := policy.CreatedAt
	if createdAt.IsZero() {
		_, err = db.ExecContext(ctx, `
INSERT INTO policies (tenant_id, policy_id, version, scope_type, scope_selector, mode, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, policy_id, version) DO UPDATE SET
  scope_type = EXCLUDED.scope_type,
  scope_selector = EXCLUDED.scope_selector,
  mode = EXCLUDED.mode,
  data = EXCLUDED.data
`, tenantID, policy.PolicyID, policy.Version, policy.Scope.Type, policy.Scope.Selector, policy.Mode, data)
	} else {
		_, err = db.ExecContext(ctx, `
INSERT INTO policies (tenant_id, policy_id, version, scope_type, scope_selector, mode, created_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, policy_id, version) DO UPDATE SET
  scope_type = EXCLUDED.scope_type,
  scope_selector = EXCLUDED.scope_selector,
  mode = EXCLUDED.mode,
  data = EXCLUDED.data
`, tenantID, policy.PolicyID, policy.Version, policy.Scope.Type, policy.Scope.Selector, policy.Mode, createdAt, data)
	}
	if err != nil {
		return fmt.Errorf("project policy: %w", err)
	}
	return nil
}

func upsertPolicyAssignment(ctx context.Context, db sqlExecutor, assignment policymodel.Assignment) error {
	if assignment.AssignmentID == "" || assignment.PolicyID == "" {
		return nil
	}
	tenantID := assignment.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	data, err := json.Marshal(assignment)
	if err != nil {
		return fmt.Errorf("encode policy assignment projection: %w", err)
	}
	createdAt := assignment.CreatedAt
	updatedAt := assignment.UpdatedAt
	if createdAt.IsZero() || updatedAt.IsZero() {
		_, err = db.ExecContext(ctx, `
INSERT INTO policy_assignments (tenant_id, assignment_id, agent_id, scope_type, scope_selector, policy_id, policy_version, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, assignment_id) DO NOTHING
`, tenantID, assignment.AssignmentID, assignment.AgentID, assignment.Scope.Type, assignment.Scope.Selector, assignment.PolicyID, assignment.PolicyVersion, data)
	} else {
		_, err = db.ExecContext(ctx, `
INSERT INTO policy_assignments (tenant_id, assignment_id, agent_id, scope_type, scope_selector, policy_id, policy_version, created_at, updated_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (tenant_id, assignment_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  scope_type = EXCLUDED.scope_type,
  scope_selector = EXCLUDED.scope_selector,
  policy_id = EXCLUDED.policy_id,
  policy_version = EXCLUDED.policy_version,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, tenantID, assignment.AssignmentID, assignment.AgentID, assignment.Scope.Type, assignment.Scope.Selector, assignment.PolicyID, assignment.PolicyVersion, createdAt, updatedAt, data)
	}
	if err != nil {
		return fmt.Errorf("project policy assignment: %w", err)
	}
	return nil
}

func upsertPolicyAudit(ctx context.Context, db sqlExecutor, audit policymodel.AuditRecord) error {
	if audit.AuditID == "" {
		return nil
	}
	tenantID := audit.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	data, err := json.Marshal(audit)
	if err != nil {
		return fmt.Errorf("encode policy audit projection: %w", err)
	}
	createdAt := audit.CreatedAt
	if createdAt.IsZero() {
		_, err = db.ExecContext(ctx, `
INSERT INTO policy_audit (tenant_id, audit_id, action, policy_id, policy_version, assignment_id, actor, status, reason, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (tenant_id, audit_id) DO UPDATE SET
  action = EXCLUDED.action,
  policy_id = EXCLUDED.policy_id,
  policy_version = EXCLUDED.policy_version,
  assignment_id = EXCLUDED.assignment_id,
  actor = EXCLUDED.actor,
  status = EXCLUDED.status,
  reason = EXCLUDED.reason,
  data = EXCLUDED.data
`, tenantID, audit.AuditID, audit.Action, audit.PolicyID, audit.PolicyVersion, audit.AssignmentID, audit.Actor, audit.Status, audit.Reason, data)
	} else {
		_, err = db.ExecContext(ctx, `
INSERT INTO policy_audit (tenant_id, audit_id, action, policy_id, policy_version, assignment_id, actor, status, reason, created_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (tenant_id, audit_id) DO UPDATE SET
  action = EXCLUDED.action,
  policy_id = EXCLUDED.policy_id,
  policy_version = EXCLUDED.policy_version,
  assignment_id = EXCLUDED.assignment_id,
  actor = EXCLUDED.actor,
  status = EXCLUDED.status,
  reason = EXCLUDED.reason,
  data = EXCLUDED.data
`, tenantID, audit.AuditID, audit.Action, audit.PolicyID, audit.PolicyVersion, audit.AssignmentID, audit.Actor, audit.Status, audit.Reason, createdAt, data)
	}
	if err != nil {
		return fmt.Errorf("project policy audit: %w", err)
	}
	return nil
}
