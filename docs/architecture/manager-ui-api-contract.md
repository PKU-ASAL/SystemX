# Manager UI API Contract

This document defines the HTTP contract between `web/manager` and
`sysarmor-manager`.

The manager UI must use `sysarmor-manager` as its only backend API. The browser
must not connect directly to Postgres, Kafka, or OpenSearch.

## Goals

- Give the manager UI stable view models for overview, agents, events, signals,
  incidents, attack chains, provenance graphs, and evidence rows.
- Keep OpenSearch credentials and query DSL ownership in the backend.
- Allow the UI to keep local mock fixtures for design work while default
  integration uses real manager APIs.
- Make each page independently testable with HTTP handler tests and frontend
  API mapper tests.

## Runtime Boundary

```text
web/manager
  -> /api/v1/*
  -> sysarmor-manager
  -> Postgres store
  -> OpenSearch searcher
```

Development should proxy `/api/v1/*` from Next.js to the local manager process.
With the default compose deployment this is normally
`http://127.0.0.1:19443/api/v1/*`, because compose exposes host port `19443`
to the manager container's `9443`. This keeps browser requests same origin and
avoids CORS in local development.

## Shared Rules

- All responses are JSON.
- Time values are RFC3339 strings in UTC unless a field explicitly documents a
  different format.
- Pagination uses `limit` and `offset`.
- List endpoints should return `total` when the backend can calculate it
  cheaply. Search-backed endpoints should return `total_relation` when exact
  totals are not available.
- Write endpoints use the existing operator authorization headers:
  - `Authorization: Bearer <token>`
  - or `X-SysArmor-Operator-Token: <token>`
- Read endpoints are unauthenticated when the manager is started without an
  operator token. If an operator token is configured, read authorization policy
  should be introduced deliberately rather than inferred from write paths.
- Errors use a stable envelope:

```json
{
  "error": {
    "code": "invalid_query",
    "message": "query field is not supported",
    "details": {
      "field": "process.parent.pid"
    }
  }
}
```

Recommended error codes:

```text
bad_request
unauthorized
forbidden
not_found
invalid_query
backend_unavailable
internal_error
```

## Existing API Surface

These endpoints already exist and should be reused where possible:

```text
GET  /healthz
GET  /api/v1/store-status
GET  /api/v1/metrics
GET  /api/v1/agents
GET  /api/v1/agent-health
GET  /api/v1/agent-sessions
GET  /api/v1/events
GET  /api/v1/signals
GET  /api/v1/incidents
GET  /api/v1/incident-evidence
POST /api/v1/incident-evidence
POST /api/v1/incident-lifecycle
POST /api/v1/incident-merge
```

Current telemetry list endpoints can use the store or the configured
OpenSearch searcher. Today they support labels, limited exact filters and
pagination. The manager UI search experience requires the extensions described
below.

## Data Source Configuration

The UI should support two data modes:

```text
api   default for integration
mock  local UI development fixtures
```

Recommended frontend environment variables:

```text
NEXT_PUBLIC_MANAGER_API_BASE=/api/v1
NEXT_PUBLIC_MANAGER_DATA_SOURCE=api
```

`mock` mode may keep using files under `web/manager/lib/mock-data.ts`. API mode
must go through typed client modules under `web/manager/lib/api/`.

## Overview

### GET /api/v1/ui/overview

Returns summary metrics for the overview tab.

Response:

```json
{
  "generated_at": "2026-07-09T15:00:00Z",
  "agents": {
    "total": 128,
    "online": 122,
    "degraded": 4,
    "offline": 2
  },
  "telemetry": {
    "events_24h": 24800,
    "signals_24h": 318
  },
  "incidents": {
    "open": 7,
    "critical": 2,
    "high": 3,
    "medium": 2
  },
  "store": {
    "backend": "postgres",
    "postgres_schema_version": 4
  }
}
```

Implementation note: this endpoint can initially compose existing
`/api/v1/metrics`, `/api/v1/agents`, `/api/v1/agent-health` and
`/api/v1/incidents` data. Add it as a backend view-model endpoint only if the
UI would otherwise need multiple requests for one overview render.

## Deploy

### GET /api/v1/ui/deploy/options

Returns the agent deployment defaults, supported platforms, active artifacts
and recent enrollments for the Deploy tab.

Query:

```text
tenant_id=default
```

Response:

```json
{
  "tenant_id": "default",
  "gateway_addr": "127.0.0.1:19444",
  "gateway_sni": "",
  "supported_platforms": [
    { "os": "linux", "arch": "amd64" },
    { "os": "linux", "arch": "arm64" }
  ],
  "artifacts": [
    {
      "artifact_id": "art-linux-amd64",
      "version": "0.8.0",
      "os": "linux",
      "arch": "amd64",
      "sha256": "abc123",
      "status": "active",
      "download_url": "http://127.0.0.1:19443/api/v1/artifacts/art-linux-amd64/download",
      "created_at": "2026-07-10T06:00:00Z"
    }
  ],
  "enrollments": [
    {
      "enrollment_id": "enr-existing",
      "tenant_id": "default",
      "agent_id": "agent-existing",
      "token_preview": "enr_...abcd",
      "labels": { "env": "prod" },
      "status": "active",
      "expires_at": "2026-07-10T07:00:00Z"
    }
  ]
}
```

The response must not include `token_hash`.

### POST /api/v1/ui/deploy/agent-command

Creates an enrollment and returns a copyable install command. This endpoint is
a UI view-model wrapper around the existing enrollment, artifact and install
script endpoints.

Request:

```json
{
  "tenant_id": "default",
  "agent_id": "agent-prod-001",
  "host_id": "prod-api-01",
  "gateway_addr": "127.0.0.1:19444",
  "gateway_sni": "localhost",
  "artifact_id": "art-linux-amd64",
  "ttl": "24h",
  "labels": {
    "env": "prod",
    "role": "api"
  }
}
```

Response:

```json
{
  "enrollment_id": "enr-new",
  "token_expires_at": "2026-07-10T07:00:00Z",
  "install_command": "curl -fsSL 'http://127.0.0.1:19443/api/v1/agent-install.sh?token=enr_x' | sudo bash",
  "script_url": "http://127.0.0.1:19443/api/v1/agent-install.sh?token=enr_x",
  "artifact": {
    "artifact_id": "art-linux-amd64",
    "download_url": "http://127.0.0.1:19443/api/v1/artifacts/art-linux-amd64/download",
    "sha256": "abc123"
  }
}
```

## Agents

### GET /api/v1/agents

Existing endpoint. The UI needs these query parameters:

```text
tenant_id
scope_type
scope_selector
health_status
limit
offset
```

Response should remain compatible with the current `AgentListItem`, with these
fields treated as stable for the manager UI:

```json
[
  {
    "agent_id": "agent-prod-001",
    "host_id": "prod-api-01",
    "tenant_id": "default",
    "version": "0.8.0",
    "auth_type": "mtls",
    "cert_identity": "spiffe://sysarmor/default/agent-prod-001",
    "health_status": "ok",
    "health_observed": "2026-07-09T15:00:00Z",
    "scope": {
      "type": "host",
      "selector": "prod-api-01"
    },
    "capability": {
      "event_stream": true,
      "response": true
    }
  }
]
```

Frontend mapping:

```text
agent_id          -> Agent
host_id           -> Host
health_status     -> Status
version           -> Version
health_observed   -> Last seen
```

Policy binding can be added later from policy assignment data. Until then the
UI should display `-` instead of inventing a policy name.

## Search Query Model

The UI query bar should not send raw OpenSearch DSL. It sends a restricted
manager query string that the backend parses.

Supported phase 1 grammar:

```text
expression := term { ("and" | "or") term }
term       := field ":" value
value      := bare-word | quoted-string
```

Examples:

```text
agent.id: agent-prod-001
host.name: prod-api-01 and event.kind: signal
severity: critical or severity: high
```

Unsupported syntax should return `400 invalid_query` with the unsupported token
or field in `details`.

The backend should allow only fields returned by `/api/v1/search/fields`.
Unknown fields must not be forwarded blindly to OpenSearch.

## Search Fields

### GET /api/v1/search/fields

Returns searchable fields for one or more index patterns.

Query parameters:

```text
index=events-*,signals-*
```

Response:

```json
{
  "indexes": ["sysarmor-events", "sysarmor-signals"],
  "fields": [
    {
      "name": "@timestamp",
      "type": "date",
      "searchable": true,
      "aggregatable": true
    },
    {
      "name": "agent.id",
      "type": "keyword",
      "searchable": true,
      "aggregatable": true
    },
    {
      "name": "event.kind",
      "type": "keyword",
      "searchable": true,
      "aggregatable": true
    },
    {
      "name": "message",
      "type": "text",
      "searchable": true,
      "aggregatable": false
    }
  ]
}
```

Phase 1 may return a static allowlist maintained in the backend. Phase 2 may
populate this from OpenSearch field capabilities.

## Events And Signals Discover

### POST /api/v1/search

Searches events and signals with a Kibana-like request model.

Request:

```json
{
  "indexes": ["sysarmor-events", "sysarmor-signals"],
  "query": "agent.id: agent-prod-001 and event.kind: signal",
  "time": {
    "field": "@timestamp",
    "from": "2026-07-09T14:30:00Z",
    "to": "2026-07-09T15:00:00Z"
  },
  "sort": [
    {
      "field": "@timestamp",
      "direction": "desc"
    }
  ],
  "limit": 100,
  "offset": 0
}
```

Response:

```json
{
  "total": 42,
  "total_relation": "eq",
  "rows": [
    {
      "index": "sysarmor-signals",
      "id": "sig-24091",
      "timestamp": "2026-07-09T14:58:12Z",
      "severity": "critical",
      "host": "prod-api-01",
      "summary": "credential access via suspicious memory read",
      "tactic": "CredentialAccess",
      "source": {
        "agent.id": "agent-prod-001",
        "host.name": "prod-api-01",
        "event.kind": "signal",
        "signal.id": "sig-24091"
      },
      "raw": {
        "signal_id": "sig-24091"
      }
    }
  ]
}
```

`source` is a flattened display map for the Discover table. `raw` is the
original document source for row expansion.

### POST /api/v1/search/histogram

Returns time buckets for the same query model.

Request:

```json
{
  "indexes": ["sysarmor-events", "sysarmor-signals"],
  "query": "severity: critical",
  "time": {
    "field": "@timestamp",
    "from": "2026-07-09T14:30:00Z",
    "to": "2026-07-09T15:00:00Z"
  },
  "interval": "auto",
  "series": [
    {
      "name": "events",
      "filter": "event.kind: event"
    },
    {
      "name": "signals",
      "filter": "event.kind: signal"
    }
  ]
}
```

Response:

```json
{
  "buckets": [
    {
      "start": "2026-07-09T14:30:00Z",
      "end": "2026-07-09T14:35:00Z",
      "total": 12,
      "series": {
        "events": 9,
        "signals": 3
      }
    }
  ]
}
```

Phase 1 may use fixed bucket counts from the request window. Phase 2 may align
with OpenSearch `date_histogram` auto intervals.

## Incidents

### POST /api/v1/incidents/search

Searches incidents for the incidents tab.

Request:

```json
{
  "query": "severity: critical",
  "time": {
    "field": "@timestamp",
    "from": "2026-07-09T00:00:00Z",
    "to": "2026-07-09T15:00:00Z"
  },
  "severity": ["critical", "high"],
  "status": ["active", "triage"],
  "limit": 100,
  "offset": 0
}
```

Response:

```json
{
  "total": 2,
  "rows": [
    {
      "incident_id": "inc-1027",
      "chain_id": "threat-chain-001",
      "title": "Web server intrusion chain",
      "timestamp": "2026-07-09T14:52:00Z",
      "severity": "critical",
      "status": "active",
      "root_cause": "Suspicious process lineage followed by C2",
      "detected_stages": 4,
      "total_stages": 12,
      "hosts": ["oa-web"],
      "alert_count": 5
    }
  ],
  "histogram": [
    {
      "start": "2026-07-09T14:30:00Z",
      "end": "2026-07-09T14:35:00Z",
      "total": 1,
      "critical": 1,
      "high": 0,
      "medium": 0
    }
  ]
}
```

The existing `GET /api/v1/incidents` should remain for compatibility and simple
store-backed listing. The UI can move to `POST /api/v1/incidents/search` when
it needs status, severity, time and histogram in one request.

### GET /api/v1/incidents/{incident_id}

Returns one incident detail view model.

Response:

```json
{
  "incident": {
    "incident_id": "inc-1027",
    "chain_id": "threat-chain-001",
    "title": "Web server intrusion chain",
    "severity": "critical",
    "status": "active",
    "summary": "External attacker exploited OA web service and established C2"
  },
  "attack_chain": [
    {
      "id": "step-initial-access",
      "order": 1,
      "tactic": "Initial Access",
      "technique_id": "T1190",
      "technique": "Exploit Public-Facing Application",
      "source": "203.0.113.42",
      "target": "oa-web",
      "detected": true,
      "evidence": "nginx accepted exploit traffic before web shell write"
    }
  ],
  "provenance": {
    "nodes": [
      {
        "id": "n-nginx",
        "node_name": "nginx",
        "node_type": "1",
        "node_desc": "Web service process",
        "node_score": 72,
        "stage_level": "L1",
        "node_variant": "tp",
        "reveal_at_sec": 65
      }
    ],
    "edges": [
      {
        "source": "n-nginx",
        "target": "n-bash",
        "technique": "T1059.004",
        "syscall": "execve",
        "tactic": "Execution"
      }
    ]
  },
  "evidence": [
    {
      "time": "2026-07-09T14:53:24Z",
      "relative_time_sec": 107,
      "host": "oa-web",
      "process": "bash",
      "syscall": "execve",
      "args": ".sysupd",
      "result": "success",
      "category": "process",
      "detail": "Spawned hidden implant .sysupd"
    }
  ]
}
```

Implementation note: the first backend version may derive this DTO from the
existing incident proto and `/api/v1/incident-evidence`. The UI should not need
to understand protobuf-specific field names or graph internals.

## Incident Evidence

### GET /api/v1/incident-evidence

Existing endpoint. Required query parameters:

```text
incident_id
label
path_from
path_to
seed
hops
```

The manager UI uses it for provenance graph expansion and node-focused
evidence. `seed` plus `hops` should remain the preferred lightweight query for
clicking a provenance node.

## Frontend File Ownership

Recommended API client files:

```text
web/manager/lib/api/client.ts       shared request/error handling
web/manager/lib/api/types.ts        UI-facing DTOs
web/manager/lib/api/overview.ts     overview service
web/manager/lib/api/deploy.ts       deploy/enrollment command service
web/manager/lib/api/agents.ts       agents service
web/manager/lib/api/search.ts       events/signals search service
web/manager/lib/api/incidents.ts    incident list/detail service
```

Page components should not call `fetch` directly. They should consume these API
modules and keep UI-specific table/chart mapping near the page or in small
feature-local mapper files.

## Backend File Ownership

Recommended backend files:

```text
internal/manager/api/http_ui_overview.go
internal/manager/api/http_ui_deploy.go
internal/manager/api/http_search.go
internal/manager/api/http_incident_detail.go
internal/manager/api/search_query.go
internal/manager/api/search_fields.go
internal/manager/api/ui_dto.go
```

OpenSearch low-level request construction should stay in
`internal/platform/opensearch`. Manager API files should own validation and
view-model shaping.

## Phased Delivery

### Phase 1: Contract And Client Skeleton

- Add frontend API client and DTO types.
- Add data source switch: `api` or `mock`.
- Keep current page behavior on `mock`.
- Add mapper tests for agents, events and incidents.

### Phase 2: Overview, Deploy And Agents

- Connect overview to existing metrics/agents endpoints or
  `/api/v1/ui/overview`.
- Add `/api/v1/ui/deploy/options`.
- Add `/api/v1/ui/deploy/agent-command`.
- Add Deploy tab for install command generation, artifacts and enrollments.
- Connect agents table to `/api/v1/agents`.
- Add local View details and Uninstall command actions to the agents table.
- Show policy as `-` until policy binding is exposed.

### Phase 3: Events And Signals Search

- Add `/api/v1/search/fields`.
- Add `/api/v1/search`.
- Add `/api/v1/search/histogram`.
- Support phase 1 query grammar and allowlisted fields only.
- Connect the events Discover page.

### Phase 4: Incidents

- Add `/api/v1/incidents/search`.
- Add `/api/v1/incidents/{incident_id}`.
- Connect incident list, severity histogram, attack chain, provenance graph and
  evidence sidebar.

### Phase 5: Integration Smoke

- Add one smoke path that starts manager with Postgres and optional OpenSearch.
- Verify:
  - `/healthz` is healthy.
  - Agents tab renders API data.
  - Events tab returns rows and histogram.
  - Incidents tab opens detail.
  - Provenance node selection returns focused evidence.

## Acceptance Criteria

- The UI can run with `NEXT_PUBLIC_MANAGER_DATA_SOURCE=api` against local
  `sysarmor-manager`.
- The browser never receives OpenSearch credentials.
- Events and incidents search reject unsupported fields with `invalid_query`.
- Existing manager API tests continue to pass.
- Frontend tests cover API mapping and mock fallback.
- The local development path is documented in `Makefile` help or a nearby
  manager UI README when implementation starts.
