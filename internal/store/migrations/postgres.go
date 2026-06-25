package migrations

const PostgresVersion = 1

const PostgresSchema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS agents (
  tenant_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  host_id TEXT NOT NULL,
  version TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, agent_id)
);

CREATE TABLE IF NOT EXISTS agent_health (
  tenant_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  host_id TEXT NOT NULL DEFAULT '',
  scope_type TEXT NOT NULL DEFAULT '',
  scope_selector TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, agent_id)
);

CREATE TABLE IF NOT EXISTS rules (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  rule_id TEXT NOT NULL,
  version BIGINT NOT NULL,
  rule_where TEXT NOT NULL,
  enabled BOOLEAN NOT NULL,
  severity INTEGER NOT NULL DEFAULT 0,
  tags TEXT[] NOT NULL DEFAULT '{}',
  mitre TEXT[] NOT NULL DEFAULT '{}',
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, rule_id, version)
);

CREATE TABLE IF NOT EXISTS policies (
  tenant_id TEXT NOT NULL,
  policy_id TEXT NOT NULL,
  version BIGINT NOT NULL,
  scope_type TEXT NOT NULL DEFAULT '',
  scope_selector TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT 'observe',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, policy_id, version)
);

CREATE TABLE IF NOT EXISTS policy_assignments (
  tenant_id TEXT NOT NULL,
  assignment_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  scope_type TEXT NOT NULL DEFAULT '',
  scope_selector TEXT NOT NULL DEFAULT '',
  policy_id TEXT NOT NULL,
  policy_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, assignment_id)
);

CREATE TABLE IF NOT EXISTS policy_audit (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  audit_id TEXT NOT NULL,
  action TEXT NOT NULL DEFAULT '',
  policy_id TEXT NOT NULL DEFAULT '',
  policy_version BIGINT NOT NULL DEFAULT 0,
  assignment_id TEXT NOT NULL DEFAULT '',
  actor TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, audit_id)
);

CREATE TABLE IF NOT EXISTS operator_role_bindings (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  actor TEXT NOT NULL,
  roles TEXT[] NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, actor)
);

CREATE TABLE IF NOT EXISTS events (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  event_id TEXT NOT NULL,
  event_behavior TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  host_id TEXT NOT NULL DEFAULT '',
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, event_id)
);

CREATE TABLE IF NOT EXISTS signals (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  signal_key TEXT NOT NULL,
  signal_id TEXT NOT NULL DEFAULT '',
  layer TEXT NOT NULL DEFAULT '',
  signal_name TEXT NOT NULL DEFAULT '',
  lineage_id TEXT NOT NULL DEFAULT '',
  terminal BOOLEAN NOT NULL DEFAULT false,
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, signal_key)
);

CREATE TABLE IF NOT EXISTS incidents (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  incident_key TEXT NOT NULL,
  incident_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  severity INTEGER NOT NULL DEFAULT 0,
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, incident_key)
);

CREATE TABLE IF NOT EXISTS incident_events (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  incident_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  PRIMARY KEY (tenant_id, incident_id, event_id)
);

CREATE TABLE IF NOT EXISTS evidence (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  incident_id TEXT NOT NULL,
  evidence_id TEXT NOT NULL,
  evidence_kind TEXT NOT NULL,
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, incident_id, evidence_id)
);

CREATE TABLE IF NOT EXISTS response_audit (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  response_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  action TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  command JSONB NOT NULL,
  ack JSONB,
  PRIMARY KEY (tenant_id, response_id)
);

CREATE TABLE IF NOT EXISTS evidence_pullbacks (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  request_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  incident_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, request_id)
);

CREATE TABLE IF NOT EXISTS control_commands (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  command_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  command_type TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  policy_id TEXT NOT NULL DEFAULT '',
  policy_version BIGINT NOT NULL DEFAULT 0,
  content_ref TEXT NOT NULL DEFAULT '',
  content_kind TEXT NOT NULL DEFAULT '',
  content_version TEXT NOT NULL DEFAULT '',
  actor TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at TIMESTAMPTZ,
  last_sent_at TIMESTAMPTZ,
  acked_at TIMESTAMPTZ,
  canceled_at TIMESTAMPTZ,
  expired_at TIMESTAMPTZ,
  attempt_count BIGINT NOT NULL DEFAULT 0,
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, command_id)
);

CREATE TABLE IF NOT EXISTS agent_sessions (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  session_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  data_transport TEXT NOT NULL DEFAULT '',
  control_transport TEXT NOT NULL DEFAULT '',
  last_ack_cursor TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_data_seen_at TIMESTAMPTZ,
  last_control_seen_at TIMESTAMPTZ,
  closed_at TIMESTAMPTZ,
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, session_id)
);

CREATE TABLE IF NOT EXISTS rarity_baseline (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  workload_key TEXT NOT NULL,
  signal_name TEXT NOT NULL,
  signal_count BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, workload_key, signal_name)
);

CREATE TABLE IF NOT EXISTS metrics (
  tenant_id TEXT NOT NULL DEFAULT 'default',
  metric_key TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  data JSONB NOT NULL,
  PRIMARY KEY (tenant_id, metric_key)
);

CREATE INDEX IF NOT EXISTS idx_agents_host_id ON agents (host_id);
CREATE INDEX IF NOT EXISTS idx_agent_health_scope ON agent_health (scope_type, scope_selector);
CREATE INDEX IF NOT EXISTS idx_policy_assignments_agent ON policy_assignments (tenant_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_policy_assignments_scope ON policy_assignments (tenant_id, scope_type, scope_selector);
CREATE INDEX IF NOT EXISTS idx_policy_audit_policy ON policy_audit (tenant_id, policy_id);
CREATE INDEX IF NOT EXISTS idx_policy_audit_actor ON policy_audit (tenant_id, actor);
CREATE INDEX IF NOT EXISTS idx_operator_role_bindings_actor ON operator_role_bindings (tenant_id, actor);
CREATE INDEX IF NOT EXISTS idx_events_labels ON events USING GIN ((data->'labels'));
CREATE INDEX IF NOT EXISTS idx_events_observed_at ON events (observed_at);
CREATE INDEX IF NOT EXISTS idx_signals_labels_layer ON signals USING GIN ((data->'labels'));
CREATE INDEX IF NOT EXISTS idx_signals_layer ON signals (tenant_id, layer);
CREATE INDEX IF NOT EXISTS idx_signals_lineage ON signals (tenant_id, lineage_id);
CREATE INDEX IF NOT EXISTS idx_incidents_labels ON incidents USING GIN ((data->'labels'));
CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_evidence_incident ON evidence (tenant_id, incident_id);
CREATE INDEX IF NOT EXISTS idx_response_audit_agent ON response_audit (tenant_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_response_audit_status ON response_audit (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_evidence_pullbacks_agent ON evidence_pullbacks (tenant_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_evidence_pullbacks_status ON evidence_pullbacks (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_control_commands_agent ON control_commands (tenant_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_control_commands_status ON control_commands (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_control_commands_type ON control_commands (tenant_id, command_type);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent ON agent_sessions (tenant_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_status ON agent_sessions (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_rarity_baseline_workload ON rarity_baseline (tenant_id, workload_key);
CREATE INDEX IF NOT EXISTS idx_rarity_baseline_signal ON rarity_baseline (tenant_id, signal_name);

INSERT INTO schema_migrations (version) VALUES (1)
ON CONFLICT (version) DO NOTHING;
`
