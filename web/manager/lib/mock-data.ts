export interface OverviewMetric {
  label: string;
  value: string;
  trend: string;
}

export interface AgentRecord {
  id: string;
  host: string;
  version: string;
  status: "healthy" | "degraded" | "offline";
  policy: string;
  lastSeen: string;
}

export interface SecurityEvent {
  id: string;
  timestamp: string;
  index: "sysarmor-events" | "sysarmor-signals";
  severity: "critical" | "high" | "medium" | "info";
  host: string;
  summary: string;
  tactic: string;
}

export type SecurityEventIndex = SecurityEvent["index"];

export interface EventFilterOptions {
  query?: string;
  indexes?: SecurityEventIndex[];
  minutes?: number;
  now?: number;
  startTime?: number;
  endTime?: number;
}

export interface EventHistogramOptions extends EventFilterOptions {
  bucketCount: number;
}

export interface EventHistogramBucket {
  start: number;
  end: number;
  label: string;
  count: number;
}

export interface EventField {
  name: string;
  type: "date" | "keyword" | "text" | "number" | "object";
}

export interface EventSourceChip {
  key: string;
  value: string;
}

export interface EventRawDocument {
  _index: SecurityEventIndex;
  _id: string;
  "@timestamp": string;
  host: {
    name: string;
  };
  event: {
    kind: "event" | "signal";
    severity: SecurityEvent["severity"];
    summary: string;
    tactic: string;
  };
}

export interface EventDiscoverRow {
  id: string;
  time: string;
  severity: SecurityEvent["severity"];
  event: SecurityEvent;
  source: EventSourceChip[];
  raw: EventRawDocument;
}

export interface IncidentRecord {
  id: string;
  chainId: string;
  title: string;
  severity: "critical" | "high" | "medium";
  status: "active" | "triage" | "contained";
  rootCause: string;
  detectedStages: number;
  totalStages: number;
  hosts: string[];
  alertCount: number;
}

export interface AttackStage {
  id: string;
  label: string;
  tactic: string;
  evidence: string;
}

export const overviewMetrics: OverviewMetric[] = [
  { label: "Online agents", value: "128", trend: "+12 today" },
  { label: "Open events", value: "24.8K", trend: "last 24h" },
  { label: "Signals", value: "318", trend: "+8 high risk" },
  { label: "Incidents", value: "7", trend: "2 active" },
];

export const agents: AgentRecord[] = [
  {
    id: "agent-prod-001",
    host: "prod-api-01",
    version: "0.8.0",
    status: "healthy",
    policy: "default-edr-policy",
    lastSeen: "32s ago",
  },
  {
    id: "agent-prod-014",
    host: "prod-db-02",
    version: "0.8.0",
    status: "degraded",
    policy: "linux-server-hardening",
    lastSeen: "4m ago",
  },
  {
    id: "agent-lab-003",
    host: "lab-runner-03",
    version: "0.7.4",
    status: "healthy",
    policy: "detection-lab",
    lastSeen: "51s ago",
  },
];

export const events: SecurityEvent[] = [
  {
    id: "evt-24091",
    timestamp: "2026-07-08 21:04:18",
    index: "sysarmor-signals",
    severity: "critical",
    host: "prod-api-01",
    summary: "credential access via suspicious memory read",
    tactic: "CredentialAccess",
  },
  {
    id: "evt-24076",
    timestamp: "2026-07-08 20:58:02",
    index: "sysarmor-events",
    severity: "medium",
    host: "prod-web-03",
    summary: "container process spawned an unexpected shell",
    tactic: "Execution",
  },
  {
    id: "evt-24065",
    timestamp: "2026-07-08 20:52:40",
    index: "sysarmor-signals",
    severity: "high",
    host: "prod-db-02",
    summary: "credential material copied before outbound connection",
    tactic: "Exfiltration",
  },
  {
    id: "evt-24011",
    timestamp: "2026-07-08 20:41:35",
    index: "sysarmor-events",
    severity: "info",
    host: "lab-runner-03",
    summary: "policy update applied to sensor",
    tactic: "DefenseEvasion",
  },
];

export const incidents: IncidentRecord[] = [
  {
    id: "inc-1027",
    chainId: "threat-chain-001",
    title: "Web服务器入侵链",
    severity: "critical",
    status: "active",
    rootCause: "Suspicious process memory access followed by lateral movement.",
    detectedStages: 4,
    totalStages: 12,
    hosts: ["oa-web"],
    alertCount: 5,
  },
  {
    id: "inc-1019",
    chainId: "threat-chain-002",
    title: "凭据窃取与横向移动链",
    severity: "critical",
    status: "triage",
    rootCause: "Unexpected shell spawned by CI job container.",
    detectedStages: 2,
    totalStages: 12,
    hosts: ["jump-server", "ad-controller"],
    alertCount: 2,
  },
];

export const attackChain: AttackStage[] = [
  {
    id: "stage-1",
    label: "Initial Access",
    tactic: "Exploit public-facing service",
    evidence: "nginx worker launched anomalous child process",
  },
  {
    id: "stage-2",
    label: "Execution",
    tactic: "Command and scripting interpreter",
    evidence: "shell created under service account",
  },
  {
    id: "stage-3",
    label: "Credential Access",
    tactic: "Process memory",
    evidence: "credential access signal evt-24091",
  },
  {
    id: "stage-4",
    label: "Exfiltration",
    tactic: "Exfiltration over web service",
    evidence: "outbound connection after credential copy",
  },
];

export function filterEvents(sourceEvents: SecurityEvent[], options: EventFilterOptions = {}) {
  const normalized = options.query?.trim().toLowerCase() ?? "";
  const indexes = options.indexes?.length ? new Set(options.indexes) : undefined;
  const startTime =
    options.startTime ??
    (options.minutes && options.now ? options.now - options.minutes * 60 * 1000 : undefined);
  const endTime = options.endTime;

  return sourceEvents.filter((event) => {
    const eventTime = new Date(event.timestamp).getTime();

    if (indexes && !indexes.has(event.index)) return false;
    if (startTime && eventTime < startTime) return false;
    if (endTime && eventTime > endTime) return false;
    if (!normalized) return true;

    return [event.summary, event.host, event.index, event.tactic, event.severity]
      .join(" ")
      .toLowerCase()
      .includes(normalized);
  });
}

export function buildEventHistogram(
  sourceEvents: SecurityEvent[],
  options: EventHistogramOptions,
): EventHistogramBucket[] {
  const now = options.now ?? Date.now();
  const minutes = options.minutes ?? 30;
  const start = options.startTime ?? now - minutes * 60 * 1000;
  const end = options.endTime ?? now;
  const bucketWidth = (end - start) / options.bucketCount;
  const filtered = filterEvents(sourceEvents, options);

  return Array.from({ length: options.bucketCount }).map((_, index) => {
    const bucketStart = start + bucketWidth * index;
    const bucketEnd = index === options.bucketCount - 1 ? end + 1 : bucketStart + bucketWidth;
    const count = filtered.filter((event) => {
      const timestamp = new Date(event.timestamp).getTime();
      return timestamp >= bucketStart && timestamp < bucketEnd;
    }).length;

    return {
      start: bucketStart,
      end: bucketEnd,
      label: new Date(bucketStart).toLocaleTimeString("zh-CN", {
        hour: "2-digit",
        minute: "2-digit",
      }),
      count,
    };
  });
}

export function createEventFields(sourceEvents: SecurityEvent[]) {
  const hasSeverity = sourceEvents.some((event) => event.severity);
  const available: EventField[] = [
    { name: "_id", type: "keyword" },
    { name: "_index", type: "keyword" },
    { name: "@timestamp", type: "date" },
    { name: "host.name", type: "keyword" },
    { name: "event.kind", type: "keyword" },
    { name: "event.summary", type: "text" },
    { name: "event.tactic", type: "keyword" },
  ];

  if (hasSeverity) {
    available.push({ name: "event.severity", type: "keyword" });
  }

  return {
    selected: [{ name: "_source", type: "object" as const }],
    available,
  };
}

export function createEventSourceChips(event: SecurityEvent): EventSourceChip[] {
  return [
    { key: "event.kind", value: event.index === "sysarmor-signals" ? "signal" : "event" },
    { key: "host.name", value: event.host },
    { key: "event.summary", value: event.summary },
    { key: "event.tactic", value: event.tactic },
    { key: "event.severity", value: event.severity },
  ];
}

export function createEventRawDocument(event: SecurityEvent): EventRawDocument {
  return {
    _index: event.index,
    _id: event.id,
    "@timestamp": event.timestamp,
    host: {
      name: event.host,
    },
    event: {
      kind: event.index === "sysarmor-signals" ? "signal" : "event",
      severity: event.severity,
      summary: event.summary,
      tactic: event.tactic,
    },
  };
}

export function createDiscoverRows(sourceEvents: SecurityEvent[]): EventDiscoverRow[] {
  return sourceEvents.map((event) => ({
    id: event.id,
    time: event.timestamp,
    severity: event.severity,
    event,
    source: createEventSourceChips(event),
    raw: createEventRawDocument(event),
  }));
}
