package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/rarity"
	link1model "github.com/sysarmor/sysarmor-next-project/internal/link1"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/store/migrations"
	"google.golang.org/protobuf/encoding/protojson"
)

const snapshotStateKey = "default"

func OpenSnapshotStore(ctx context.Context, db *sql.DB, migration MigrationResult) (*store.Store, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres snapshot db is nil")
	}
	st, err := store.Open("")
	if err != nil {
		return nil, err
	}
	state, ok, err := loadSnapshot(ctx, db)
	if err != nil {
		return nil, err
	}
	if ok {
		if err := st.ImportState(state); err != nil {
			return nil, fmt.Errorf("import postgres snapshot state: %w", err)
		}
	}
	info := store.Info{
		Backend:          "postgres",
		StateVersion:     store.FileStoreStateVersion,
		MigrationVersion: migration.Version,
		PostgresSchema:   migrations.PostgresVersion,
	}
	st.ConfigureBackend(info, func(state store.State) error {
		return saveSnapshot(context.Background(), db, state)
	})
	st.ConfigureQueryHooks(
		func(scenario, kind string) ([]*eventv1.CanonicalEvent, error) {
			return queryEvents(context.Background(), db, scenario, kind)
		},
		func(scenario, layer string, terminalOnly bool) ([]*signalv1.Signal, error) {
			return querySignals(context.Background(), db, scenario, layer, terminalOnly)
		},
	)
	return st, nil
}

func loadSnapshot(ctx context.Context, db *sql.DB) (store.State, bool, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, "SELECT data FROM sysarmor_state WHERE state_key = $1", snapshotStateKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return store.State{}, false, nil
	}
	if err != nil {
		return store.State{}, false, fmt.Errorf("load postgres snapshot: %w", err)
	}
	var state store.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return store.State{}, false, fmt.Errorf("decode postgres snapshot: %w", err)
	}
	return state, true, nil
}

func saveSnapshot(ctx context.Context, db *sql.DB, state store.State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO sysarmor_state (state_key, state_version, data, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (state_key) DO UPDATE SET
  state_version = EXCLUDED.state_version,
  data = EXCLUDED.data,
  updated_at = now()
`, snapshotStateKey, store.FileStoreStateVersion, data)
	if err != nil {
		return fmt.Errorf("save postgres snapshot: %w", err)
	}
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
	if err := projectLink1Sessions(ctx, db, state.Link1Sessions); err != nil {
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

func queryEvents(ctx context.Context, db *sql.DB, scenario, kind string) ([]*eventv1.CanonicalEvent, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM events
WHERE ($1 = '' OR scenario = $1)
  AND ($2 = '' OR event_kind = $2)
ORDER BY observed_at ASC, event_id ASC
`, scenario, kind)
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

func projectAgents(ctx context.Context, db *sql.DB, agentRows []json.RawMessage) error {
	for _, raw := range agentRows {
		var agent analyticsv1.AgentHello
		if err := protojson.Unmarshal(raw, &agent); err != nil {
			return fmt.Errorf("decode agent projection: %w", err)
		}
		if agent.GetAgentId() == "" {
			continue
		}
		tenantID := agent.GetTenantId()
		if tenantID == "" {
			tenantID = "default"
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO agents (tenant_id, agent_id, host_id, version, data)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
  host_id = EXCLUDED.host_id,
  version = EXCLUDED.version,
  observed_at = now(),
  data = EXCLUDED.data
`, tenantID, agent.GetAgentId(), agent.GetHostId(), agent.GetVersion(), []byte(raw))
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
INSERT INTO events (tenant_id, event_id, scenario, event_kind, agent_id, host_id, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, event_id) DO UPDATE SET
  scenario = EXCLUDED.scenario,
  event_kind = EXCLUDED.event_kind,
  agent_id = EXCLUDED.agent_id,
  host_id = EXCLUDED.host_id,
  observed_at = now(),
  data = EXCLUDED.data
`, "default", event.GetId(), event.GetScenario(), store.EventKindName(event.GetKind()), event.GetAgentId(), event.GetHostId(), []byte(raw))
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
		if cmd.ResponseID == "" {
			continue
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
		if ack, ok := ackByResponseID[cmd.ResponseID]; ok {
			data, err := json.Marshal(ack)
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
	}
	return nil
}

func projectEvidencePullbacks(ctx context.Context, db *sql.DB, pullbacks []link1model.EvidencePullbackRequest) error {
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
			status = link1model.EvidencePullbackStatusPending
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
		if policy.PolicyID == "" || policy.Version == 0 {
			continue
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
	}
	return nil
}

func projectPolicyAssignments(ctx context.Context, db *sql.DB, assignments []policymodel.Assignment) error {
	for _, assignment := range assignments {
		if assignment.AssignmentID == "" || assignment.PolicyID == "" {
			continue
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
	}
	return nil
}

func projectPolicyAudits(ctx context.Context, db *sql.DB, audits []policymodel.AuditRecord) error {
	for _, audit := range audits {
		if audit.AuditID == "" {
			continue
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

func projectLink1Sessions(ctx context.Context, db *sql.DB, sessions []store.Link1Session) error {
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
			return fmt.Errorf("encode link1 session projection: %w", err)
		}
		startedAt := session.StartedAt
		lastSeenAt := session.LastSeenAt
		if startedAt.IsZero() || lastSeenAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO link1_sessions (tenant_id, session_id, agent_id, status, transport, last_ack_cursor, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, session_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  transport = EXCLUDED.transport,
  last_ack_cursor = EXCLUDED.last_ack_cursor,
  last_seen_at = now(),
  data = EXCLUDED.data
`, tenantID, session.SessionID, session.AgentID, session.Status, session.Transport, session.LastAckCursor, data)
		} else if session.ClosedAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO link1_sessions (tenant_id, session_id, agent_id, status, transport, last_ack_cursor, started_at, last_seen_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (tenant_id, session_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  transport = EXCLUDED.transport,
  last_ack_cursor = EXCLUDED.last_ack_cursor,
  last_seen_at = EXCLUDED.last_seen_at,
  closed_at = NULL,
  data = EXCLUDED.data
`, tenantID, session.SessionID, session.AgentID, session.Status, session.Transport, session.LastAckCursor, startedAt, lastSeenAt, data)
		} else {
			_, err = db.ExecContext(ctx, `
INSERT INTO link1_sessions (tenant_id, session_id, agent_id, status, transport, last_ack_cursor, started_at, last_seen_at, closed_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (tenant_id, session_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  transport = EXCLUDED.transport,
  last_ack_cursor = EXCLUDED.last_ack_cursor,
  last_seen_at = EXCLUDED.last_seen_at,
  closed_at = EXCLUDED.closed_at,
  data = EXCLUDED.data
`, tenantID, session.SessionID, session.AgentID, session.Status, session.Transport, session.LastAckCursor, startedAt, lastSeenAt, session.ClosedAt, data)
		}
		if err != nil {
			return fmt.Errorf("project link1 session: %w", err)
		}
	}
	return nil
}
