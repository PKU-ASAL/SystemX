package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/rarity"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/store/migrations"
	"google.golang.org/protobuf/encoding/protojson"
	"strings"
)

const snapshotStateKey = "default"

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
	st.ConfigureBackend(info, func(state store.State) error {
		return saveTables(context.Background(), db, state)
	})
	st.ConfigureQueryHooks(
		func(scenario, kind string) ([]*eventv1.CanonicalEvent, error) {
			return queryEvents(context.Background(), db, scenario, kind)
		},
		func(scenario, layer string, terminalOnly bool) ([]*signalv1.Signal, error) {
			return querySignals(context.Background(), db, scenario, layer, terminalOnly)
		},
		func(scenario string) ([]*incidentv1.Incident, error) {
			return queryIncidents(context.Background(), db, scenario)
		},
		func(tenantID, agentID string) ([]responsemodel.AuditRecord, error) {
			return queryResponses(context.Background(), db, tenantID, agentID)
		},
		func(tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error) {
			return queryControlCommands(context.Background(), db, tenantID, agentID, commandType)
		},
		func(tenantID string) ([]policymodel.Policy, error) {
			return queryPolicies(context.Background(), db, tenantID)
		},
		func(tenantID, agentID string) ([]policymodel.Assignment, error) {
			return queryPolicyAssignments(context.Background(), db, tenantID, agentID)
		},
		func(tenantID, policyID string) ([]policymodel.AuditRecord, error) {
			return queryPolicyAudits(context.Background(), db, tenantID, policyID)
		},
		func(tenantID, policyID string, version uint64) (policymodel.Policy, bool, error) {
			return queryPolicy(context.Background(), db, tenantID, policyID, version)
		},
		func(tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool, error) {
			return queryEffectivePolicy(context.Background(), db, tenantID, agentID, scopeType, scopeSelector)
		},
	)
	st.ConfigureWriteHooks(
		func(cmd responsemodel.Command, ack *responsemodel.Ack) error {
			return upsertResponseAudit(context.Background(), db, cmd, ack)
		},
		func(policy policymodel.Policy) error {
			return upsertPolicy(context.Background(), db, policy)
		},
		func(assignment policymodel.Assignment) error {
			return upsertPolicyAssignment(context.Background(), db, assignment)
		},
		func(audit policymodel.AuditRecord) error {
			return upsertPolicyAudit(context.Background(), db, audit)
		},
	)
	return st, nil
}

func saveTables(ctx context.Context, db *sql.DB, state store.State) error {
	if err := projectAgents(ctx, db, state.Agents); err != nil {
		return err
	}
	if err := projectAgentHealth(ctx, db, state.Health); err != nil {
		return err
	}
	if err := projectEvents(ctx, db, state.Events); err != nil {
		return err
	}
	if err := projectSignals(ctx, db, state.Signals); err != nil {
		return err
	}
	if err := projectRules(ctx, db, state.Rules); err != nil {
		return err
	}
	if err := projectResponseAudit(ctx, db, state.Responses, state.ResponseAcks); err != nil {
		return err
	}
	if err := projectPolicies(ctx, db, state.Policies); err != nil {
		return err
	}
	if err := projectPolicyAssignments(ctx, db, state.Assignments); err != nil {
		return err
	}
	if err := projectPolicyAudits(ctx, db, state.PolicyAudits); err != nil {
		return err
	}
	if err := projectOperatorRoleBindings(ctx, db, state.OperatorRoles); err != nil {
		return err
	}
	if err := projectIncidents(ctx, db, state.Incidents); err != nil {
		return err
	}
	if err := projectEvidencePullbacks(ctx, db, state.Pullbacks); err != nil {
		return err
	}
	if err := projectControlCommands(ctx, db, state.ControlCommands); err != nil {
		return err
	}
	if err := projectAgentSessions(ctx, db, state.AgentSessions); err != nil {
		return err
	}
	if err := projectRarityBaseline(ctx, db, state.RarityBaseline); err != nil {
		return err
	}
	if err := projectMetrics(ctx, db, state.Metrics); err != nil {
		return err
	}
	return nil
}

func queryEvents(ctx context.Context, db *sql.DB, scenario, behavior string) ([]*eventv1.CanonicalEvent, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM events
WHERE ($1 = '' OR scenario = $1)
  AND ($2 = '' OR event_behavior = $2)
ORDER BY observed_at ASC, event_id ASC
`, scenario, behavior)
	if err != nil {
		return nil, fmt.Errorf("query postgres events: %w", err)
	}
	defer rows.Close()
	out := []*eventv1.CanonicalEvent{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres event: %w", err)
		}
		event := &eventv1.CanonicalEvent{}
		if err := protojson.Unmarshal(raw, event); err != nil {
			return nil, fmt.Errorf("decode postgres event: %w", err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres events: %w", err)
	}
	return out, nil
}

func querySignals(ctx context.Context, db *sql.DB, scenario, layer string, terminalOnly bool) ([]*signalv1.Signal, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM signals
WHERE ($1 = '' OR scenario = $1)
  AND ($2 = '' OR layer = $2)
  AND ($3 = false OR terminal = true)
ORDER BY observed_at ASC, signal_key ASC
`, scenario, layer, terminalOnly)
	if err != nil {
		return nil, fmt.Errorf("query postgres signals: %w", err)
	}
	defer rows.Close()
	out := []*signalv1.Signal{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres signal: %w", err)
		}
		signal := &signalv1.Signal{}
		if err := protojson.Unmarshal(raw, signal); err != nil {
			return nil, fmt.Errorf("decode postgres signal: %w", err)
		}
		out = append(out, signal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres signals: %w", err)
	}
	return out, nil
}

func queryIncidents(ctx context.Context, db *sql.DB, scenario string) ([]*incidentv1.Incident, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM incidents
WHERE ($1 = '' OR scenario = $1)
ORDER BY updated_at ASC, incident_id ASC
`, scenario)
	if err != nil {
		return nil, fmt.Errorf("query postgres incidents: %w", err)
	}
	defer rows.Close()
	out := []*incidentv1.Incident{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres incident: %w", err)
		}
		incident := &incidentv1.Incident{}
		if err := protojson.Unmarshal(raw, incident); err != nil {
			return nil, fmt.Errorf("decode postgres incident: %w", err)
		}
		out = append(out, incident)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres incidents: %w", err)
	}
	return out, nil
}

func queryResponses(ctx context.Context, db *sql.DB, tenantID, agentID string) ([]responsemodel.AuditRecord, error) {
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

func queryControlCommands(ctx context.Context, db *sql.DB, tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error) {
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

func queryPolicies(ctx context.Context, db *sql.DB, tenantID string) ([]policymodel.Policy, error) {
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

func queryPolicy(ctx context.Context, db *sql.DB, tenantID, policyID string, version uint64) (policymodel.Policy, bool, error) {
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

func queryPublishedPolicy(ctx context.Context, db *sql.DB, tenantID, policyID string, version uint64) (policymodel.Policy, bool, error) {
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

func queryEffectivePolicy(ctx context.Context, db *sql.DB, tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool, error) {
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

func queryPolicyAssignments(ctx context.Context, db *sql.DB, tenantID, agentID string) ([]policymodel.Assignment, error) {
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

func projectAgents(ctx context.Context, db *sql.DB, agentRows []json.RawMessage) error {
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

func projectAgentHealth(ctx context.Context, db *sql.DB, healthRows []json.RawMessage) error {
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

func projectEvents(ctx context.Context, db *sql.DB, eventRows []json.RawMessage) error {
	for _, raw := range eventRows {
		var event eventv1.CanonicalEvent
		if err := protojson.Unmarshal(raw, &event); err != nil {
			return fmt.Errorf("decode event projection: %w", err)
		}
		if event.GetId() == "" {
			continue
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO events (tenant_id, event_id, scenario, event_behavior, agent_id, host_id, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, event_id) DO UPDATE SET
  scenario = EXCLUDED.scenario,
  event_behavior = EXCLUDED.event_behavior,
  agent_id = EXCLUDED.agent_id,
  host_id = EXCLUDED.host_id,
  observed_at = now(),
  data = EXCLUDED.data
`, "default", event.GetId(), event.GetScenario(), event.GetBehavior(), event.GetAgentId(), event.GetHostId(), []byte(raw))
		if err != nil {
			return fmt.Errorf("project event: %w", err)
		}
	}
	return nil
}

func projectSignals(ctx context.Context, db *sql.DB, signalRows []json.RawMessage) error {
	for _, raw := range signalRows {
		var signal signalv1.Signal
		if err := protojson.Unmarshal(raw, &signal); err != nil {
			return fmt.Errorf("decode signal projection: %w", err)
		}
		signalKey := store.SignalProjectionKey(&signal)
		if signalKey == "" {
			continue
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO signals (tenant_id, signal_key, signal_id, scenario, layer, signal_name, lineage_id, terminal, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (tenant_id, signal_key) DO UPDATE SET
  signal_id = EXCLUDED.signal_id,
  scenario = EXCLUDED.scenario,
  layer = EXCLUDED.layer,
  signal_name = EXCLUDED.signal_name,
  lineage_id = EXCLUDED.lineage_id,
  terminal = EXCLUDED.terminal,
  observed_at = now(),
  data = EXCLUDED.data
`, "default", signalKey, signal.GetId(), signal.GetScenario(), store.SignalLayerName(signal.GetWhere()), signal.GetName(), signal.GetLineageId(), signal.GetTerminal(), []byte(raw))
		if err != nil {
			return fmt.Errorf("project signal: %w", err)
		}
	}
	return nil
}

func projectRules(ctx context.Context, db *sql.DB, rules []policymodel.RuleContent) error {
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

func projectResponseAudit(ctx context.Context, db *sql.DB, commands []responsemodel.Command, acks []responsemodel.Ack) error {
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

func queryPolicyAudits(ctx context.Context, db *sql.DB, tenantID, policyID string) ([]policymodel.AuditRecord, error) {
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

func upsertResponseAudit(ctx context.Context, db *sql.DB, cmd responsemodel.Command, ack *responsemodel.Ack) error {
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

func projectEvidencePullbacks(ctx context.Context, db *sql.DB, pullbacks []controlmodel.EvidencePullbackRequest) error {
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
INSERT INTO evidence_pullbacks (tenant_id, request_id, agent_id, incident_id, scenario, status, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, request_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  incident_id = EXCLUDED.incident_id,
  scenario = EXCLUDED.scenario,
  status = EXCLUDED.status,
  updated_at = now(),
  data = EXCLUDED.data
`, tenantID, req.RequestID, req.AgentID, req.IncidentID, req.Scenario, status, data)
		} else {
			_, err = db.ExecContext(ctx, `
INSERT INTO evidence_pullbacks (tenant_id, request_id, agent_id, incident_id, scenario, status, created_at, updated_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (tenant_id, request_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  incident_id = EXCLUDED.incident_id,
  scenario = EXCLUDED.scenario,
  status = EXCLUDED.status,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, tenantID, req.RequestID, req.AgentID, req.IncidentID, req.Scenario, status, createdAt, updatedAt, data)
		}
		if err != nil {
			return fmt.Errorf("project evidence pullback: %w", err)
		}
	}
	return nil
}

func projectControlCommands(ctx context.Context, db *sql.DB, commands []controlmodel.ControlCommand) error {
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

func projectPolicies(ctx context.Context, db *sql.DB, policies []policymodel.Policy) error {
	for _, policy := range policies {
		if err := upsertPolicy(ctx, db, policy); err != nil {
			return err
		}
	}
	return nil
}

func upsertPolicy(ctx context.Context, db *sql.DB, policy policymodel.Policy) error {
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

func projectPolicyAssignments(ctx context.Context, db *sql.DB, assignments []policymodel.Assignment) error {
	for _, assignment := range assignments {
		if err := upsertPolicyAssignment(ctx, db, assignment); err != nil {
			return err
		}
	}
	return nil
}

func upsertPolicyAssignment(ctx context.Context, db *sql.DB, assignment policymodel.Assignment) error {
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

func projectPolicyAudits(ctx context.Context, db *sql.DB, audits []policymodel.AuditRecord) error {
	for _, audit := range audits {
		if err := upsertPolicyAudit(ctx, db, audit); err != nil {
			return err
		}
	}
	return nil
}

func upsertPolicyAudit(ctx context.Context, db *sql.DB, audit policymodel.AuditRecord) error {
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

func projectOperatorRoleBindings(ctx context.Context, db *sql.DB, bindings []store.OperatorRoleBinding) error {
	for _, binding := range bindings {
		if binding.Actor == "" {
			continue
		}
		data, err := json.Marshal(binding)
		if err != nil {
			return fmt.Errorf("encode operator role binding projection: %w", err)
		}
		createdAt := binding.CreatedAt
		updatedAt := binding.UpdatedAt
		if createdAt.IsZero() || updatedAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO operator_role_bindings (tenant_id, actor, roles, data)
VALUES ($1, $2, string_to_array($3, E'\x1f'), $4)
ON CONFLICT (tenant_id, actor) DO UPDATE SET
  roles = EXCLUDED.roles,
  updated_at = now(),
  data = EXCLUDED.data
`, "default", binding.Actor, joinTextArray(binding.Roles), data)
		} else {
			_, err = db.ExecContext(ctx, `
INSERT INTO operator_role_bindings (tenant_id, actor, roles, created_at, updated_at, data)
VALUES ($1, $2, string_to_array($3, E'\x1f'), $4, $5, $6)
ON CONFLICT (tenant_id, actor) DO UPDATE SET
  roles = EXCLUDED.roles,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, "default", binding.Actor, joinTextArray(binding.Roles), createdAt, updatedAt, data)
		}
		if err != nil {
			return fmt.Errorf("project operator role binding: %w", err)
		}
	}
	return nil
}

func projectIncidents(ctx context.Context, db *sql.DB, incidentRows []json.RawMessage) error {
	for _, raw := range incidentRows {
		var inc incidentv1.Incident
		if err := protojson.Unmarshal(raw, &inc); err != nil {
			return fmt.Errorf("decode incident projection: %w", err)
		}
		if inc.GetId() == "" {
			continue
		}
		incidentKey := store.IncidentProjectionKey(&inc)
		if incidentKey == "" {
			incidentKey = inc.GetId()
		}
		status := inc.GetStatus()
		if status == "" {
			status = "open"
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO incidents (tenant_id, incident_key, incident_id, scenario, status, severity, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, incident_key) DO UPDATE SET
  incident_id = EXCLUDED.incident_id,
  scenario = EXCLUDED.scenario,
  status = EXCLUDED.status,
  severity = EXCLUDED.severity,
  updated_at = now(),
  data = EXCLUDED.data
`, "default", incidentKey, inc.GetId(), inc.GetScenario(), status, inc.GetSeverity(), []byte(raw))
		if err != nil {
			return fmt.Errorf("project incident: %w", err)
		}
		if err := projectIncidentEvidence(ctx, db, inc.GetId(), inc.GetEvidence()); err != nil {
			return err
		}
		if err := projectIncidentEvents(ctx, db, inc.GetId(), &inc); err != nil {
			return err
		}
	}
	return nil
}

func projectIncidentEvents(ctx context.Context, db *sql.DB, incidentID string, inc *incidentv1.Incident) error {
	if incidentID == "" || inc == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, sig := range inc.GetContributingSignals() {
		for _, eventID := range sig.GetEventRefs() {
			if err := upsertIncidentEvent(ctx, db, incidentID, eventID, seen); err != nil {
				return err
			}
		}
		for _, eventID := range sig.GetEvidence().GetEventRefs() {
			if err := upsertIncidentEvent(ctx, db, incidentID, eventID, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func upsertIncidentEvent(ctx context.Context, db *sql.DB, incidentID, eventID string, seen map[string]bool) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || seen[eventID] {
		return nil
	}
	seen[eventID] = true
	_, err := db.ExecContext(ctx, `
INSERT INTO incident_events (tenant_id, incident_id, event_id)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, incident_id, event_id) DO NOTHING
`, "default", incidentID, eventID)
	if err != nil {
		return fmt.Errorf("project incident event: %w", err)
	}
	return nil
}

func projectIncidentEvidence(ctx context.Context, db *sql.DB, incidentID string, evidence *incidentv1.EvidenceSubgraph) error {
	if incidentID == "" || evidence == nil {
		return nil
	}
	mo := protojson.MarshalOptions{UseProtoNames: true}
	for _, node := range evidence.GetNodes() {
		evidenceID := nodeEvidenceID(node)
		if evidenceID == "" {
			continue
		}
		kind := node.GetKind()
		if kind == "" {
			kind = "node"
		}
		data, err := mo.Marshal(node)
		if err != nil {
			return fmt.Errorf("encode evidence node projection: %w", err)
		}
		if err := upsertEvidence(ctx, db, incidentID, evidenceID, kind, data); err != nil {
			return err
		}
	}
	for _, edge := range evidence.GetEdges() {
		evidenceID := edgeEvidenceID(edge)
		if evidenceID == "" {
			continue
		}
		kind := edge.GetKind()
		if kind == "" {
			kind = "edge"
		}
		data, err := mo.Marshal(edge)
		if err != nil {
			return fmt.Errorf("encode evidence edge projection: %w", err)
		}
		if err := upsertEvidence(ctx, db, incidentID, evidenceID, kind, data); err != nil {
			return err
		}
	}
	return nil
}

func upsertEvidence(ctx context.Context, db *sql.DB, incidentID, evidenceID, kind string, data []byte) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO evidence (tenant_id, incident_id, evidence_id, evidence_kind, data)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, incident_id, evidence_id) DO UPDATE SET
  evidence_kind = EXCLUDED.evidence_kind,
  data = EXCLUDED.data
`, "default", incidentID, evidenceID, kind, data)
	if err != nil {
		return fmt.Errorf("project evidence: %w", err)
	}
	return nil
}

func nodeEvidenceID(node *incidentv1.GraphNode) string {
	if node == nil {
		return ""
	}
	if node.GetId() != "" {
		return "node:" + node.GetId()
	}
	if node.GetKind() == "" && node.GetLabel() == "" {
		return ""
	}
	return "node:" + node.GetKind() + ":" + node.GetLabel()
}

func edgeEvidenceID(edge *incidentv1.GraphEdge) string {
	if edge == nil {
		return ""
	}
	if edge.GetId() != "" {
		return "edge:" + edge.GetId()
	}
	if edge.GetFrom() == "" || edge.GetTo() == "" || edge.GetKind() == "" {
		return ""
	}
	return "edge:" + edge.GetFrom() + ":" + edge.GetKind() + ":" + edge.GetTo()
}

func projectMetrics(ctx context.Context, db *sql.DB, metrics store.Metrics) error {
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

func projectRarityBaseline(ctx context.Context, db *sql.DB, baseline rarity.Baseline) error {
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

func projectAgentSessions(ctx context.Context, db *sql.DB, sessions []store.AgentSession) error {
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
