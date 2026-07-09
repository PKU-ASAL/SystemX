export type ManagerTimeRange = {
  field?: string;
  from?: string;
  to?: string;
};

export type ManagerSort = {
  field: string;
  direction: "asc" | "desc";
};

export type AgentListItem = {
  agent_id: string;
  host_id?: string;
  tenant_id?: string;
  version?: string;
  auth_type?: string;
  cert_identity?: string;
  health_status?: string;
  health_observed?: string;
  scope?: {
    type?: string;
    selector?: string;
  };
  capability?: Record<string, boolean | string | number | null>;
};

export type OverviewSummary = {
  generated_at: string;
  agents: {
    total: number;
    online: number;
    degraded: number;
    offline: number;
  };
  telemetry: {
    events_24h: number;
    signals_24h: number;
  };
  incidents: {
    open: number;
    critical: number;
    high: number;
    medium: number;
  };
  store: {
    backend: string;
    postgres_schema_version?: number;
  };
};

export type SearchFieldCapability = {
  name: string;
  type: string;
  searchable: boolean;
  aggregatable: boolean;
};

export type SearchFieldsResponse = {
  indexes: string[];
  fields: SearchFieldCapability[];
};

export type TelemetrySearchRequest = {
  indexes: string[];
  query?: string;
  time?: ManagerTimeRange;
  sort?: ManagerSort[];
  limit?: number;
  offset?: number;
};

export type TelemetrySearchRow = {
  index: string;
  id: string;
  timestamp?: string;
  severity?: string;
  host?: string;
  summary?: string;
  tactic?: string;
  source?: Record<string, string | number | boolean | null>;
  raw?: unknown;
};

export type TelemetrySearchResponse = {
  total: number;
  total_relation?: "eq" | "gte";
  rows: TelemetrySearchRow[];
};

export type IncidentSearchRequest = {
  query?: string;
  time?: ManagerTimeRange;
  severity?: string[];
  status?: string[];
  limit?: number;
  offset?: number;
};

export type IncidentListItem = {
  incident_id: string;
  chain_id?: string;
  title?: string;
  timestamp?: string;
  severity?: string;
  status?: string;
  root_cause?: string;
  detected_stages?: number;
  total_stages?: number;
  hosts?: string[];
  alert_count?: number;
};

export type IncidentHistogramBucket = {
  start: string;
  end: string;
  total: number;
  critical: number;
  high: number;
  medium: number;
};

export type IncidentSearchResponse = {
  total: number;
  rows: IncidentListItem[];
  histogram: IncidentHistogramBucket[];
};

export type AttackChainStepDTO = {
  id: string;
  order: number;
  tactic: string;
  technique_id: string;
  technique: string;
  source: string;
  target: string;
  detected: boolean;
  evidence: string;
};

export type ProvenanceNodeDTO = {
  id: string;
  node_name: string;
  node_type: string;
  node_desc: string;
  node_score: number;
  stage_level?: string;
  node_variant?: string;
  reveal_at_sec?: number;
};

export type ProvenanceEdgeDTO = {
  source: string;
  target: string;
  technique?: string;
  syscall?: string;
  tactic?: string;
};

export type EvidenceEventDTO = {
  time: string;
  relative_time_sec?: number;
  host?: string;
  process?: string;
  syscall?: string;
  args?: string;
  result?: string;
  category?: string;
  detail?: string;
};

export type IncidentDetailResponse = {
  incident: IncidentListItem & {
    summary?: string;
  };
  attack_chain: AttackChainStepDTO[];
  provenance: {
    nodes: ProvenanceNodeDTO[];
    edges: ProvenanceEdgeDTO[];
  };
  evidence: EvidenceEventDTO[];
};
