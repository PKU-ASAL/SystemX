package migrations

import (
	"strings"
	"testing"
)

func TestPostgresSchemaCoversV3StoreTables(t *testing.T) {
	for _, table := range []string{
		"schema_migrations",
		"agents",
		"agent_health",
		"rules",
		"policies",
		"policy_assignments",
		"policy_audit",
		"operator_role_bindings",
		"events",
		"signals",
		"incidents",
		"incident_events",
		"evidence",
		"response_audit",
		"evidence_pullbacks",
		"control_commands",
		"agent_sessions",
		"rarity_baseline",
		"metrics",
	} {
		if !strings.Contains(PostgresSchema, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("postgres schema missing table %s", table)
		}
	}
	for _, index := range []string{
		"idx_agents_host_id",
		"idx_agent_health_scope",
		"idx_policy_assignments_agent",
		"idx_policy_audit_policy",
		"idx_operator_role_bindings_actor",
		"idx_events_labels",
		"idx_signals_labels_layer",
		"idx_signals_lineage",
		"idx_incidents_labels",
		"idx_evidence_incident",
		"idx_response_audit_agent",
		"idx_evidence_pullbacks_agent",
		"idx_control_commands_agent",
		"idx_agent_sessions_agent",
		"idx_rarity_baseline_workload",
	} {
		if !strings.Contains(PostgresSchema, "CREATE INDEX IF NOT EXISTS "+index) {
			t.Fatalf("postgres schema missing index %s", index)
		}
	}
	if !strings.Contains(PostgresSchema, "INSERT INTO schema_migrations (version) VALUES (1)") {
		t.Fatal("postgres schema does not record migration version 1")
	}
}
