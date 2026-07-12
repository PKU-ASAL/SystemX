package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/rarity"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/store/migrations"
)

const snapshotStateKey = "default"

// opTimeout bounds each backend database operation so a single query or
// projection cannot block indefinitely.
const opTimeout = 30 * time.Second

// sqlExecutor is satisfied by both *sql.DB and *sql.Tx, so projection and query
// helpers can run either directly or inside the state-projection transaction.
type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// tableBackend is the postgres implementation of store.Backend. It persists
// low-volume relational platform state only; high-volume telemetry (events and
// signals) is intentionally not projected here and lives in the index tier.
type tableBackend struct {
	db *sql.DB
}

func OpenTableStore(ctx context.Context, db *sql.DB, migration MigrationResult) (*store.Store, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres db is nil")
	}
	st, err := store.Open("")
	if err != nil {
		return nil, err
	}
	info := store.Info{
		Backend:          "postgres",
		StateVersion:     store.FileStoreStateVersion,
		MigrationVersion: migration.Version,
		PostgresSchema:   migrations.PostgresVersion,
	}
	st.AttachBackend(ctx, &tableBackend{db: db}, info)
	return st, nil
}

// withTimeout derives a bounded operation context from the store-supplied base
// context, so backend work is cancelled both on base-context cancellation
// (e.g. server shutdown) and after opTimeout.
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, opTimeout)
}

func (b *tableBackend) SaveState(ctx context.Context, state store.State) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return saveTables(ctx, b.db, state)
}

func (b *tableBackend) ListAgents(ctx context.Context) ([]store.AgentIdentity, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryAgents(ctx, b.db)
}

func (b *tableBackend) ListAgentHealth(ctx context.Context) ([]agenthealth.AgentHealth, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryAgentHealth(ctx, b.db, "", "")
}

func (b *tableBackend) GetAgentHealth(ctx context.Context, tenantID, agentID string) (agenthealth.AgentHealth, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	rows, err := queryAgentHealth(ctx, b.db, tenantID, agentID)
	if err != nil {
		return agenthealth.AgentHealth{}, false, err
	}
	if len(rows) == 0 {
		return agenthealth.AgentHealth{}, false, nil
	}
	if len(rows) > 1 {
		return agenthealth.AgentHealth{}, false, nil
	}
	return rows[0], true, nil
}

func (b *tableBackend) ListAgentSessions(ctx context.Context, tenantID, agentID string) ([]store.AgentSession, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryAgentSessions(ctx, b.db, tenantID, agentID)
}

func (b *tableBackend) ListResponses(ctx context.Context, tenantID, agentID string) ([]responsemodel.AuditRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryResponses(ctx, b.db, tenantID, agentID)
}

func (b *tableBackend) ListControlCommands(ctx context.Context, tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryControlCommands(ctx, b.db, tenantID, agentID, commandType)
}

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

func (b *tableBackend) ListEnrollments(ctx context.Context, tenantID, status string) ([]store.Enrollment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryEnrollments(ctx, b.db, tenantID, status)
}

func (b *tableBackend) GetEnrollmentByTokenHash(ctx context.Context, tokenHash string) (store.Enrollment, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryEnrollmentByTokenHash(ctx, b.db, tokenHash)
}

func (b *tableBackend) ListArtifacts(ctx context.Context, tenantID, kind, status string) ([]store.Artifact, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryArtifacts(ctx, b.db, tenantID, kind, status)
}

func (b *tableBackend) GetArtifact(ctx context.Context, tenantID, artifactID string) (store.Artifact, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryArtifact(ctx, b.db, tenantID, artifactID)
}

func (b *tableBackend) ListChannels(ctx context.Context, tenantID string) ([]store.ArtifactChannel, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryChannels(ctx, b.db, tenantID)
}

func (b *tableBackend) GetChannel(ctx context.Context, tenantID, channel string) (store.ArtifactChannel, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryChannel(ctx, b.db, tenantID, channel)
}

func (b *tableBackend) WriteResponse(ctx context.Context, cmd responsemodel.Command, ack *responsemodel.Ack) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertResponseAudit(ctx, b.db, cmd, ack)
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

func (b *tableBackend) WriteEnrollment(ctx context.Context, enrollment store.Enrollment) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertEnrollment(ctx, b.db, enrollment)
}

func (b *tableBackend) WriteArtifact(ctx context.Context, artifact store.Artifact) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertArtifact(ctx, b.db, artifact)
}

func (b *tableBackend) WriteChannel(ctx context.Context, channel store.ArtifactChannel) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertChannel(ctx, b.db, channel)
}

func (b *tableBackend) WriteAgentCertificate(ctx context.Context, cert store.AgentCertificate) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertAgentCertificate(ctx, b.db, cert)
}

// saveTables projects the full platform-state snapshot in a single transaction
// so a partially applied projection cannot leave the store inconsistent.
// Telemetry (events, signals) is deliberately excluded: it belongs in the index
// tier, not the relational state store.
func saveTables(ctx context.Context, db *sql.DB, state store.State) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin state projection tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := projectAgents(ctx, tx, state.Agents); err != nil {
		return err
	}
	if err := projectAgentHealth(ctx, tx, state.Health); err != nil {
		return err
	}
	if err := projectRules(ctx, tx, state.Rules); err != nil {
		return err
	}
	if err := projectResponseAudit(ctx, tx, state.Responses, state.ResponseAcks); err != nil {
		return err
	}
	if err := projectPolicies(ctx, tx, state.Policies); err != nil {
		return err
	}
	if err := projectPolicyAssignments(ctx, tx, state.Assignments); err != nil {
		return err
	}
	if err := projectPolicyAudits(ctx, tx, state.PolicyAudits); err != nil {
		return err
	}
	if err := projectEnrollments(ctx, tx, state.Enrollments); err != nil {
		return err
	}
	if err := projectArtifacts(ctx, tx, state.Artifacts); err != nil {
		return err
	}
	if err := projectChannels(ctx, tx, state.Channels); err != nil {
		return err
	}
	if err := projectAgentCertificates(ctx, tx, state.Certificates); err != nil {
		return err
	}
	if err := projectEvidencePullbacks(ctx, tx, state.Pullbacks); err != nil {
		return err
	}
	if err := projectControlCommands(ctx, tx, state.ControlCommands); err != nil {
		return err
	}
	if err := projectAgentSessions(ctx, tx, state.AgentSessions); err != nil {
		return err
	}
	if err := projectRarityBaseline(ctx, tx, state.RarityBaseline); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit state projection tx: %w", err)
	}
	committed = true
	return nil
}

func queryAgents(ctx context.Context, db sqlExecutor) ([]store.AgentIdentity, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM agents
ORDER BY tenant_id ASC, agent_id ASC
`)
	if err != nil {
		return nil, fmt.Errorf("query postgres agents: %w", err)
	}
	defer rows.Close()
	var out []store.AgentIdentity
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres agent: %w", err)
		}
		var agent store.AgentIdentity
		if err := json.Unmarshal(raw, &agent); err != nil {
			return nil, fmt.Errorf("decode postgres agent: %w", err)
		}
		out = append(out, agent.Normalized())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres agents: %w", err)
	}
	return out, nil
}

func queryAgentHealth(ctx context.Context, db sqlExecutor, tenantID, agentID string) ([]agenthealth.AgentHealth, error) {
	query := `
SELECT data FROM agent_health
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR agent_id = $2)
ORDER BY tenant_id ASC, agent_id ASC
`
	rows, err := db.QueryContext(ctx, query, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("query postgres agent health: %w", err)
	}
	defer rows.Close()
	var out []agenthealth.AgentHealth
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres agent health: %w", err)
		}
		var health agenthealth.AgentHealth
		if err := json.Unmarshal(raw, &health); err != nil {
			return nil, fmt.Errorf("decode postgres agent health: %w", err)
		}
		out = append(out, health)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres agent health: %w", err)
	}
	return out, nil
}

func queryAgentSessions(ctx context.Context, db sqlExecutor, tenantID, agentID string) ([]store.AgentSession, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM agent_sessions
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR agent_id = $2)
ORDER BY last_seen_at DESC, tenant_id ASC, agent_id ASC
`, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("query postgres agent sessions: %w", err)
	}
	defer rows.Close()
	out := []store.AgentSession{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres agent session: %w", err)
		}
		var session store.AgentSession
		if err := json.Unmarshal(raw, &session); err != nil {
			return nil, fmt.Errorf("decode postgres agent session: %w", err)
		}
		out = append(out, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres agent sessions: %w", err)
	}
	return out, nil
}

func queryResponses(ctx context.Context, db sqlExecutor, tenantID, agentID string) ([]responsemodel.AuditRecord, error) {
	rows, err := db.QueryContext(ctx, `
SELECT command, ack FROM response_audit
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR agent_id = $2)
ORDER BY updated_at ASC, response_id ASC
`, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("query postgres response audit: %w", err)
	}
	defer rows.Close()
	out := []responsemodel.AuditRecord{}
	for rows.Next() {
		var commandRaw []byte
		var ackRaw []byte
		if err := rows.Scan(&commandRaw, &ackRaw); err != nil {
			return nil, fmt.Errorf("scan postgres response audit: %w", err)
		}
		var command responsemodel.Command
		if err := json.Unmarshal(commandRaw, &command); err != nil {
			return nil, fmt.Errorf("decode postgres response command: %w", err)
		}
		record := responsemodel.AuditRecord{Command: command}
		if len(ackRaw) > 0 {
			var ack responsemodel.Ack
			if err := json.Unmarshal(ackRaw, &ack); err != nil {
				return nil, fmt.Errorf("decode postgres response ack: %w", err)
			}
			record.Ack = &ack
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres response audit: %w", err)
	}
	return out, nil
}

func queryControlCommands(ctx context.Context, db sqlExecutor, tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM control_commands
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR agent_id = $2)
  AND ($3 = '' OR command_type = $3)
ORDER BY created_at ASC, command_id ASC
`, tenantID, agentID, commandType)
	if err != nil {
		return nil, fmt.Errorf("query postgres control commands: %w", err)
	}
	defer rows.Close()
	out := []controlmodel.ControlCommand{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres control command: %w", err)
		}
		var cmd controlmodel.ControlCommand
		if err := json.Unmarshal(raw, &cmd); err != nil {
			return nil, fmt.Errorf("decode postgres control command: %w", err)
		}
		out = append(out, cmd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres control commands: %w", err)
	}
	return out, nil
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
	return policymodel.DefaultPolicy(tenantID), true, nil
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

func projectAgents(ctx context.Context, db sqlExecutor, agentRows []json.RawMessage) error {
	for _, raw := range agentRows {
		var agent store.AgentIdentity
		if err := json.Unmarshal(raw, &agent); err != nil {
			return fmt.Errorf("decode agent projection: %w", err)
		}
		agent = agent.Normalized()
		if !agent.Valid() {
			continue
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO agents (tenant_id, agent_id, host_id, version, data)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
  host_id = EXCLUDED.host_id,
  version = EXCLUDED.version,
  observed_at = now(),
  data = EXCLUDED.data
`, agent.TenantID, agent.AgentID, agent.HostID, agent.Version, []byte(raw))
		if err != nil {
			return fmt.Errorf("project agent: %w", err)
		}
	}
	return nil
}

func projectAgentHealth(ctx context.Context, db sqlExecutor, healthRows []json.RawMessage) error {
	for _, raw := range healthRows {
		var health agenthealth.AgentHealth
		if err := json.Unmarshal(raw, &health); err != nil {
			return fmt.Errorf("decode agent health projection: %w", err)
		}
		if health.AgentID == "" {
			continue
		}
		tenantID := health.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		observedAt := health.ObservedAt
		if observedAt.IsZero() {
			_, err := db.ExecContext(ctx, `
INSERT INTO agent_health (tenant_id, agent_id, host_id, scope_type, scope_selector, data)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
  host_id = EXCLUDED.host_id,
  scope_type = EXCLUDED.scope_type,
  scope_selector = EXCLUDED.scope_selector,
  observed_at = now(),
  data = EXCLUDED.data
`, tenantID, health.AgentID, health.HostID, health.Scope.Type, health.Scope.Selector, []byte(raw))
			if err != nil {
				return fmt.Errorf("project agent health: %w", err)
			}
			continue
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO agent_health (tenant_id, agent_id, host_id, scope_type, scope_selector, observed_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
  host_id = EXCLUDED.host_id,
  scope_type = EXCLUDED.scope_type,
  scope_selector = EXCLUDED.scope_selector,
  observed_at = EXCLUDED.observed_at,
  data = EXCLUDED.data
`, tenantID, health.AgentID, health.HostID, health.Scope.Type, health.Scope.Selector, observedAt, []byte(raw))
		if err != nil {
			return fmt.Errorf("project agent health: %w", err)
		}
	}
	return nil
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

func projectResponseAudit(ctx context.Context, db sqlExecutor, commands []responsemodel.Command, acks []responsemodel.Ack) error {
	ackByResponseID := map[string]responsemodel.Ack{}
	for _, ack := range acks {
		if ack.ResponseID != "" {
			ackByResponseID[ack.ResponseID] = ack
		}
	}
	for _, cmd := range commands {
		var ackPtr *responsemodel.Ack
		if ack, ok := ackByResponseID[cmd.ResponseID]; ok {
			ackPtr = &ack
		}
		if err := upsertResponseAudit(ctx, db, cmd, ackPtr); err != nil {
			return err
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

func queryEnrollments(ctx context.Context, db sqlExecutor, tenantID, status string) ([]store.Enrollment, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM enrollments
WHERE tenant_id = $1 AND ($2 = '' OR status = $2)
ORDER BY created_at ASC, enrollment_id ASC
`, tenantID, status)
	if err != nil {
		return nil, fmt.Errorf("query postgres enrollments: %w", err)
	}
	defer rows.Close()
	var out []store.Enrollment
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres enrollment: %w", err)
		}
		var enrollment store.Enrollment
		if err := json.Unmarshal(raw, &enrollment); err != nil {
			return nil, fmt.Errorf("decode postgres enrollment: %w", err)
		}
		out = append(out, enrollment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres enrollments: %w", err)
	}
	return out, nil
}

func queryEnrollmentByTokenHash(ctx context.Context, db sqlExecutor, tokenHash string) (store.Enrollment, bool, error) {
	if tokenHash == "" {
		return store.Enrollment{}, false, nil
	}
	row := db.QueryRowContext(ctx, `
SELECT data FROM enrollments
WHERE token_hash = $1
ORDER BY created_at DESC
LIMIT 1
`, tokenHash)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.Enrollment{}, false, nil
		}
		return store.Enrollment{}, false, fmt.Errorf("query postgres enrollment by token: %w", err)
	}
	var enrollment store.Enrollment
	if err := json.Unmarshal(raw, &enrollment); err != nil {
		return store.Enrollment{}, false, fmt.Errorf("decode postgres enrollment by token: %w", err)
	}
	return enrollment, true, nil
}

func queryArtifacts(ctx context.Context, db sqlExecutor, tenantID, kind, status string) ([]store.Artifact, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM artifacts
WHERE tenant_id = $1
  AND ($2 = '' OR artifact_kind = $2)
  AND ($3 = '' OR status = $3)
ORDER BY created_at ASC, artifact_id ASC
`, tenantID, kind, status)
	if err != nil {
		return nil, fmt.Errorf("query postgres artifacts: %w", err)
	}
	defer rows.Close()
	var out []store.Artifact
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres artifact: %w", err)
		}
		var artifact store.Artifact
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return nil, fmt.Errorf("decode postgres artifact: %w", err)
		}
		out = append(out, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres artifacts: %w", err)
	}
	return out, nil
}

func queryArtifact(ctx context.Context, db sqlExecutor, tenantID, artifactID string) (store.Artifact, bool, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if artifactID == "" {
		return store.Artifact{}, false, nil
	}
	row := db.QueryRowContext(ctx, `
SELECT data FROM artifacts
WHERE tenant_id = $1 AND artifact_id = $2
`, tenantID, artifactID)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.Artifact{}, false, nil
		}
		return store.Artifact{}, false, fmt.Errorf("query postgres artifact: %w", err)
	}
	var artifact store.Artifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return store.Artifact{}, false, fmt.Errorf("decode postgres artifact: %w", err)
	}
	return artifact, true, nil
}

func queryChannels(ctx context.Context, db sqlExecutor, tenantID string) ([]store.ArtifactChannel, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM artifact_channels
WHERE tenant_id = $1
ORDER BY channel_name ASC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query postgres artifact channels: %w", err)
	}
	defer rows.Close()
	var out []store.ArtifactChannel
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres artifact channel: %w", err)
		}
		var channel store.ArtifactChannel
		if err := json.Unmarshal(raw, &channel); err != nil {
			return nil, fmt.Errorf("decode postgres artifact channel: %w", err)
		}
		out = append(out, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres artifact channels: %w", err)
	}
	return out, nil
}

func queryChannel(ctx context.Context, db sqlExecutor, tenantID, channelName string) (store.ArtifactChannel, bool, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if channelName == "" {
		return store.ArtifactChannel{}, false, nil
	}
	row := db.QueryRowContext(ctx, `
SELECT data FROM artifact_channels
WHERE tenant_id = $1 AND channel_name = $2
`, tenantID, channelName)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.ArtifactChannel{}, false, nil
		}
		return store.ArtifactChannel{}, false, fmt.Errorf("query postgres artifact channel: %w", err)
	}
	var channel store.ArtifactChannel
	if err := json.Unmarshal(raw, &channel); err != nil {
		return store.ArtifactChannel{}, false, fmt.Errorf("decode postgres artifact channel: %w", err)
	}
	return channel, true, nil
}

func upsertResponseAudit(ctx context.Context, db sqlExecutor, cmd responsemodel.Command, ack *responsemodel.Ack) error {
	if cmd.ResponseID == "" {
		return nil
	}
	tenantID := cmd.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	commandData, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("encode response command projection: %w", err)
	}
	var ackData any
	if ack != nil {
		data, err := json.Marshal(*ack)
		if err != nil {
			return fmt.Errorf("encode response ack projection: %w", err)
		}
		ackData = data
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO response_audit (tenant_id, response_id, agent_id, status, action, created_at, updated_at, command, ack)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (tenant_id, response_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  action = EXCLUDED.action,
  updated_at = EXCLUDED.updated_at,
  command = EXCLUDED.command,
  ack = EXCLUDED.ack
`, tenantID, cmd.ResponseID, cmd.AgentID, cmd.Status, cmd.Action, cmd.CreatedAt, cmd.UpdatedAt, commandData, ackData)
	if err != nil {
		return fmt.Errorf("project response audit: %w", err)
	}
	return nil
}

func projectEvidencePullbacks(ctx context.Context, db sqlExecutor, pullbacks []controlmodel.EvidencePullbackRequest) error {
	for _, req := range pullbacks {
		if req.RequestID == "" {
			continue
		}
		tenantID := req.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		status := req.Status
		if status == "" {
			status = controlmodel.EvidencePullbackStatusPending
		}
		data, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("encode evidence pullback projection: %w", err)
		}
		createdAt := req.CreatedAt
		updatedAt := req.UpdatedAt
		if createdAt.IsZero() || updatedAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO evidence_pullbacks (tenant_id, request_id, agent_id, incident_id, status, data)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, request_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  incident_id = EXCLUDED.incident_id,
  status = EXCLUDED.status,
  updated_at = now(),
  data = EXCLUDED.data
`, tenantID, req.RequestID, req.AgentID, req.IncidentID, status, data)
		} else {
			_, err = db.ExecContext(ctx, `
INSERT INTO evidence_pullbacks (tenant_id, request_id, agent_id, incident_id, status, created_at, updated_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, request_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  incident_id = EXCLUDED.incident_id,
  status = EXCLUDED.status,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, tenantID, req.RequestID, req.AgentID, req.IncidentID, status, createdAt, updatedAt, data)
		}
		if err != nil {
			return fmt.Errorf("project evidence pullback: %w", err)
		}
	}
	return nil
}

func projectControlCommands(ctx context.Context, db sqlExecutor, commands []controlmodel.ControlCommand) error {
	for _, cmd := range commands {
		if cmd.CommandID == "" {
			continue
		}
		tenantID := cmd.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		status := cmd.Status
		if status == "" {
			status = controlmodel.ControlCommandStatusPending
		}
		data, err := json.Marshal(cmd)
		if err != nil {
			return fmt.Errorf("encode control command projection: %w", err)
		}
		var sentAt any
		if !cmd.SentAt.IsZero() {
			sentAt = cmd.SentAt
		}
		var lastSentAt any
		if !cmd.LastSentAt.IsZero() {
			lastSentAt = cmd.LastSentAt
		}
		var ackedAt any
		if !cmd.AckedAt.IsZero() {
			ackedAt = cmd.AckedAt
		}
		var canceledAt any
		if !cmd.CanceledAt.IsZero() {
			canceledAt = cmd.CanceledAt
		}
		var expiredAt any
		if !cmd.ExpiredAt.IsZero() {
			expiredAt = cmd.ExpiredAt
		}
		createdAt := cmd.CreatedAt
		updatedAt := cmd.UpdatedAt
		if createdAt.IsZero() || updatedAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO control_commands (tenant_id, command_id, agent_id, command_type, status, policy_id, policy_version, content_ref, content_kind, content_version, actor, reason, sent_at, last_sent_at, acked_at, canceled_at, expired_at, attempt_count, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
ON CONFLICT (tenant_id, command_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  command_type = EXCLUDED.command_type,
  status = EXCLUDED.status,
  policy_id = EXCLUDED.policy_id,
  policy_version = EXCLUDED.policy_version,
  content_ref = EXCLUDED.content_ref,
  content_kind = EXCLUDED.content_kind,
  content_version = EXCLUDED.content_version,
  actor = EXCLUDED.actor,
  reason = EXCLUDED.reason,
  updated_at = now(),
  sent_at = EXCLUDED.sent_at,
  last_sent_at = EXCLUDED.last_sent_at,
  acked_at = EXCLUDED.acked_at,
  canceled_at = EXCLUDED.canceled_at,
  expired_at = EXCLUDED.expired_at,
  attempt_count = EXCLUDED.attempt_count,
  data = EXCLUDED.data
`, tenantID, cmd.CommandID, cmd.AgentID, cmd.Type, status, cmd.PolicyID, cmd.PolicyVersion, cmd.ContentRef, cmd.ContentKind, cmd.ContentVersion, cmd.Actor, cmd.Reason, sentAt, lastSentAt, ackedAt, canceledAt, expiredAt, cmd.AttemptCount, data)
		} else {
			_, err = db.ExecContext(ctx, `
INSERT INTO control_commands (tenant_id, command_id, agent_id, command_type, status, policy_id, policy_version, content_ref, content_kind, content_version, actor, reason, created_at, updated_at, sent_at, last_sent_at, acked_at, canceled_at, expired_at, attempt_count, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
ON CONFLICT (tenant_id, command_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  command_type = EXCLUDED.command_type,
  status = EXCLUDED.status,
  policy_id = EXCLUDED.policy_id,
  policy_version = EXCLUDED.policy_version,
  content_ref = EXCLUDED.content_ref,
  content_kind = EXCLUDED.content_kind,
  content_version = EXCLUDED.content_version,
  actor = EXCLUDED.actor,
  reason = EXCLUDED.reason,
  updated_at = EXCLUDED.updated_at,
  sent_at = EXCLUDED.sent_at,
  last_sent_at = EXCLUDED.last_sent_at,
  acked_at = EXCLUDED.acked_at,
  canceled_at = EXCLUDED.canceled_at,
  expired_at = EXCLUDED.expired_at,
  attempt_count = EXCLUDED.attempt_count,
  data = EXCLUDED.data
`, tenantID, cmd.CommandID, cmd.AgentID, cmd.Type, status, cmd.PolicyID, cmd.PolicyVersion, cmd.ContentRef, cmd.ContentKind, cmd.ContentVersion, cmd.Actor, cmd.Reason, createdAt, updatedAt, sentAt, lastSentAt, ackedAt, canceledAt, expiredAt, cmd.AttemptCount, data)
		}
		if err != nil {
			return fmt.Errorf("project control command: %w", err)
		}
	}
	return nil
}

func severityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func joinTextArray(values []string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, strings.ReplaceAll(value, "\x1f", ""))
	}
	return strings.Join(out, "\x1f")
}

func projectPolicies(ctx context.Context, db sqlExecutor, policies []policymodel.Policy) error {
	for _, policy := range policies {
		if err := upsertPolicy(ctx, db, policy); err != nil {
			return err
		}
	}
	return nil
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

func projectPolicyAssignments(ctx context.Context, db sqlExecutor, assignments []policymodel.Assignment) error {
	for _, assignment := range assignments {
		if err := upsertPolicyAssignment(ctx, db, assignment); err != nil {
			return err
		}
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
ON CONFLICT (tenant_id, assignment_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  scope_type = EXCLUDED.scope_type,
  scope_selector = EXCLUDED.scope_selector,
  policy_id = EXCLUDED.policy_id,
  policy_version = EXCLUDED.policy_version,
  updated_at = now(),
  data = EXCLUDED.data
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

func projectPolicyAudits(ctx context.Context, db sqlExecutor, audits []policymodel.AuditRecord) error {
	for _, audit := range audits {
		if err := upsertPolicyAudit(ctx, db, audit); err != nil {
			return err
		}
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

func projectEnrollments(ctx context.Context, db sqlExecutor, enrollments []store.Enrollment) error {
	for _, enrollment := range enrollments {
		if err := upsertEnrollment(ctx, db, enrollment); err != nil {
			return err
		}
	}
	return nil
}

func upsertEnrollment(ctx context.Context, db sqlExecutor, enrollment store.Enrollment) error {
	if enrollment.EnrollmentID == "" || enrollment.TokenHash == "" {
		return nil
	}
	tenantID := enrollment.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	status := enrollment.Status
	if status == "" {
		status = "active"
	}
	data, err := json.Marshal(enrollment)
	if err != nil {
		return fmt.Errorf("encode enrollment projection: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO enrollments (tenant_id, enrollment_id, agent_id, host_id, token_hash, status, created_at, expires_at, used_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, nullif($8, '0001-01-01T00:00:00Z')::timestamptz, nullif($9, '0001-01-01T00:00:00Z')::timestamptz, $10)
ON CONFLICT (tenant_id, enrollment_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  host_id = EXCLUDED.host_id,
  token_hash = EXCLUDED.token_hash,
  status = EXCLUDED.status,
  expires_at = EXCLUDED.expires_at,
  used_at = EXCLUDED.used_at,
  data = EXCLUDED.data
`, tenantID, enrollment.EnrollmentID, enrollment.AgentID, enrollment.HostID, enrollment.TokenHash, status, enrollment.CreatedAt, formatOptionalTime(enrollment.ExpiresAt), formatOptionalTime(enrollment.UsedAt), data)
	if err != nil {
		return fmt.Errorf("project enrollment: %w", err)
	}
	return nil
}

func projectArtifacts(ctx context.Context, db sqlExecutor, artifacts []store.Artifact) error {
	for _, artifact := range artifacts {
		if err := upsertArtifact(ctx, db, artifact); err != nil {
			return err
		}
	}
	return nil
}

func upsertArtifact(ctx context.Context, db sqlExecutor, artifact store.Artifact) error {
	if artifact.ArtifactID == "" || artifact.Name == "" || artifact.Kind == "" || artifact.Version == "" || artifact.SHA256 == "" {
		return nil
	}
	tenantID := artifact.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	status := artifact.Status
	if status == "" {
		status = "draft"
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return fmt.Errorf("encode artifact projection: %w", err)
	}
	createdAt := artifact.CreatedAt
	updatedAt := artifact.UpdatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO artifacts (tenant_id, artifact_id, artifact_name, artifact_kind, artifact_version, artifact_os, artifact_arch, sha256, size_bytes, status, storage_path, created_at, updated_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (tenant_id, artifact_id) DO UPDATE SET
  artifact_name = EXCLUDED.artifact_name,
  artifact_kind = EXCLUDED.artifact_kind,
  artifact_version = EXCLUDED.artifact_version,
  artifact_os = EXCLUDED.artifact_os,
  artifact_arch = EXCLUDED.artifact_arch,
  sha256 = EXCLUDED.sha256,
  size_bytes = EXCLUDED.size_bytes,
  status = EXCLUDED.status,
  storage_path = EXCLUDED.storage_path,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, tenantID, artifact.ArtifactID, artifact.Name, artifact.Kind, artifact.Version, artifact.OS, artifact.Arch, artifact.SHA256, artifact.SizeBytes, status, artifact.StoragePath, createdAt, updatedAt, data)
	if err != nil {
		return fmt.Errorf("project artifact: %w", err)
	}
	return nil
}

func projectChannels(ctx context.Context, db sqlExecutor, channels []store.ArtifactChannel) error {
	for _, channel := range channels {
		if err := upsertChannel(ctx, db, channel); err != nil {
			return err
		}
	}
	return nil
}

func upsertChannel(ctx context.Context, db sqlExecutor, channel store.ArtifactChannel) error {
	if channel.Channel == "" || channel.ArtifactID == "" {
		return nil
	}
	tenantID := channel.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	now := time.Now().UTC()
	if channel.CreatedAt.IsZero() {
		channel.CreatedAt = now
	}
	if channel.UpdatedAt.IsZero() {
		channel.UpdatedAt = channel.CreatedAt
	}
	data, err := json.Marshal(channel)
	if err != nil {
		return fmt.Errorf("encode artifact channel projection: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO artifact_channels (tenant_id, channel_name, artifact_id, created_at, updated_at, data)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, channel_name) DO UPDATE SET
  artifact_id = EXCLUDED.artifact_id,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, tenantID, channel.Channel, channel.ArtifactID, channel.CreatedAt, channel.UpdatedAt, data)
	if err != nil {
		return fmt.Errorf("project artifact channel: %w", err)
	}
	return nil
}

func projectAgentCertificates(ctx context.Context, db sqlExecutor, certs []store.AgentCertificate) error {
	for _, cert := range certs {
		if err := upsertAgentCertificate(ctx, db, cert); err != nil {
			return err
		}
	}
	return nil
}

func upsertAgentCertificate(ctx context.Context, db sqlExecutor, cert store.AgentCertificate) error {
	if cert.TenantID == "" {
		cert.TenantID = "default"
	}
	if cert.AgentID == "" || cert.SerialNumber == "" {
		return nil
	}
	if cert.CreatedAt.IsZero() {
		cert.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(cert)
	if err != nil {
		return fmt.Errorf("encode agent certificate projection: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO agent_certificates (tenant_id, agent_id, serial_number, enrollment_id, not_before, not_after, created_at, revoked_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, nullif($8, '0001-01-01T00:00:00Z')::timestamptz, $9)
ON CONFLICT (tenant_id, serial_number) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  enrollment_id = EXCLUDED.enrollment_id,
  not_before = EXCLUDED.not_before,
  not_after = EXCLUDED.not_after,
  revoked_at = EXCLUDED.revoked_at,
  data = EXCLUDED.data
`, cert.TenantID, cert.AgentID, cert.SerialNumber, cert.EnrollmentID, cert.NotBefore, cert.NotAfter, cert.CreatedAt, formatOptionalTime(cert.RevokedAt), data)
	if err != nil {
		return fmt.Errorf("project agent certificate: %w", err)
	}
	return nil
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return "0001-01-01T00:00:00Z"
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func projectMetrics(ctx context.Context, db sqlExecutor, metrics store.Metrics) error {
	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("encode metrics projection: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO metrics (tenant_id, metric_key, data)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, metric_key) DO UPDATE SET
  updated_at = now(),
  data = EXCLUDED.data
`, "default", "manager", data)
	if err != nil {
		return fmt.Errorf("project metrics: %w", err)
	}
	return nil
}

func (b *tableBackend) LoadMetrics(ctx context.Context) (store.Metrics, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryMetrics(ctx, b.db)
}

func (b *tableBackend) SaveMetrics(ctx context.Context, metrics store.Metrics) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return projectMetrics(ctx, b.db, metrics)
}

func (b *tableBackend) ResetMetrics(ctx context.Context) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return projectMetrics(ctx, b.db, store.Metrics{})
}

func queryMetrics(ctx context.Context, db sqlExecutor) (store.Metrics, error) {
	row := db.QueryRowContext(ctx, `
SELECT data FROM metrics
WHERE tenant_id = $1 AND metric_key = $2
`, "default", "manager")
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.Metrics{}, nil
		}
		return store.Metrics{}, fmt.Errorf("query metrics: %w", err)
	}
	var metrics store.Metrics
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return store.Metrics{}, fmt.Errorf("decode metrics: %w", err)
	}
	return metrics, nil
}

func projectRarityBaseline(ctx context.Context, db sqlExecutor, baseline rarity.Baseline) error {
	for workload, signals := range baseline.WorkloadCounts {
		workload = strings.TrimSpace(workload)
		if workload == "" {
			workload = "global"
		}
		for signalName, count := range signals {
			signalName = strings.TrimSpace(signalName)
			if signalName == "" || count == 0 {
				continue
			}
			row := map[string]any{
				"workload_key": workload,
				"signal_name":  signalName,
				"signal_count": count,
			}
			data, err := json.Marshal(row)
			if err != nil {
				return fmt.Errorf("encode rarity baseline projection: %w", err)
			}
			_, err = db.ExecContext(ctx, `
INSERT INTO rarity_baseline (tenant_id, workload_key, signal_name, signal_count, data)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, workload_key, signal_name) DO UPDATE SET
  signal_count = EXCLUDED.signal_count,
  updated_at = now(),
  data = EXCLUDED.data
`, "default", workload, signalName, count, data)
			if err != nil {
				return fmt.Errorf("project rarity baseline: %w", err)
			}
		}
	}
	return nil
}

func projectAgentSessions(ctx context.Context, db sqlExecutor, sessions []store.AgentSession) error {
	for _, session := range sessions {
		if session.SessionID == "" || session.AgentID == "" {
			continue
		}
		tenantID := session.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("encode agent session projection: %w", err)
		}
		startedAt := session.StartedAt
		lastSeenAt := session.LastSeenAt
		var lastDataSeenAt any
		if !session.LastDataSeenAt.IsZero() {
			lastDataSeenAt = session.LastDataSeenAt
		}
		var lastControlSeenAt any
		if !session.LastControlSeenAt.IsZero() {
			lastControlSeenAt = session.LastControlSeenAt
		}
		if startedAt.IsZero() || lastSeenAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO agent_sessions (tenant_id, session_id, agent_id, status, data_transport, control_transport, last_ack_cursor, last_data_seen_at, last_control_seen_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (tenant_id, session_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  data_transport = EXCLUDED.data_transport,
  control_transport = EXCLUDED.control_transport,
  last_ack_cursor = EXCLUDED.last_ack_cursor,
  last_data_seen_at = EXCLUDED.last_data_seen_at,
  last_control_seen_at = EXCLUDED.last_control_seen_at,
  last_seen_at = now(),
  data = EXCLUDED.data
`, tenantID, session.SessionID, session.AgentID, session.Status, session.DataTransport, session.ControlTransport, session.LastAckCursor, lastDataSeenAt, lastControlSeenAt, data)
		} else if session.ClosedAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO agent_sessions (tenant_id, session_id, agent_id, status, data_transport, control_transport, last_ack_cursor, started_at, last_seen_at, last_data_seen_at, last_control_seen_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (tenant_id, session_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  data_transport = EXCLUDED.data_transport,
  control_transport = EXCLUDED.control_transport,
  last_ack_cursor = EXCLUDED.last_ack_cursor,
  last_seen_at = EXCLUDED.last_seen_at,
  last_data_seen_at = EXCLUDED.last_data_seen_at,
  last_control_seen_at = EXCLUDED.last_control_seen_at,
  closed_at = NULL,
  data = EXCLUDED.data
`, tenantID, session.SessionID, session.AgentID, session.Status, session.DataTransport, session.ControlTransport, session.LastAckCursor, startedAt, lastSeenAt, lastDataSeenAt, lastControlSeenAt, data)
		} else {
			_, err = db.ExecContext(ctx, `
INSERT INTO agent_sessions (tenant_id, session_id, agent_id, status, data_transport, control_transport, last_ack_cursor, started_at, last_seen_at, last_data_seen_at, last_control_seen_at, closed_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (tenant_id, session_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  data_transport = EXCLUDED.data_transport,
  control_transport = EXCLUDED.control_transport,
  last_ack_cursor = EXCLUDED.last_ack_cursor,
  last_seen_at = EXCLUDED.last_seen_at,
  last_data_seen_at = EXCLUDED.last_data_seen_at,
  last_control_seen_at = EXCLUDED.last_control_seen_at,
  closed_at = EXCLUDED.closed_at,
  data = EXCLUDED.data
`, tenantID, session.SessionID, session.AgentID, session.Status, session.DataTransport, session.ControlTransport, session.LastAckCursor, startedAt, lastSeenAt, lastDataSeenAt, lastControlSeenAt, session.ClosedAt, data)
		}
		if err != nil {
			return fmt.Errorf("project agent session: %w", err)
		}
	}
	return nil
}
