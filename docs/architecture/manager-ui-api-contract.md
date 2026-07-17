# Manager UI API Contract

This document defines the current boundary between the Manager Console BFF and
Manager. Handler code and tests under `internal/manager/api/` remain the source
of truth for exact request and response fields.

## Runtime Boundary

The browser sends same-origin requests to the Next.js BFF. The BFF validates
the Auth.js session, adds a short-lived Manager JWT, and forwards to Manager.
The browser does not receive the JWT and does not call Manager directly.

Manager derives tenant and role authorization from the verified JWT principal.
It rejects conflicting tenant input and never trusts caller identity headers.
Enrollment certificate and install-script endpoints use their one-time token
contract instead of operator JWT authentication.

## UI Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/ui/overview` | Summary counts and service health for the overview page |
| `GET` | `/api/v1/ui/deploy/options` | Available artifacts, channels, profiles, and deployment choices |
| `POST` | `/api/v1/ui/deploy/agent-command` | Create enrollment and render the Agent installation command |
| `GET` | `/api/v1/agents` | Filtered Agent inventory |
| `GET` | `/api/v1/search/fields` | Searchable field metadata for query assistance |
| `POST` | `/api/v1/search` | Tenant-bound event or signal search |
| `POST` | `/api/v1/search/histogram` | Time-bucket aggregation for the active search |
| `GET` | `/api/v1/incidents` | Tenant-bound incident list or one report selected by `incident_id` |

The Manager also exposes lower-level policy, artifact, enrollment, response,
control, event, signal, incident, health, and metrics APIs. They are product
APIs, not a separate browser trust boundary.

## Current Integration Status

Overview, Deploy, Agents, and event/signal search have Manager handlers matching
the table above. The Incidents page currently renders local mock data. Its
unused typed client still targets `/incidents/search` and `/incidents/{id}`;
those routes are not Manager contracts and must not be used for integration.
The implemented Manager query is:

```text
GET /api/v1/incidents?tenant_id=<tenant>&limit=<n>&offset=<n>
GET /api/v1/incidents?tenant_id=<tenant>&incident_id=<id>
```

Until the page is connected to these responses, the UI must be described as a
design fixture rather than a live incident workflow.

## Query Rules

- Tenant scope comes from the principal; a permitted explicit tenant must
  match it.
- Search accepts only supported aliases and validated fields.
- Exact identifiers and labels use exact-match semantics.
- Pagination and time ranges are bounded by the handler.
- Empty results are successful responses, not transport errors.

## Errors

API errors use an HTTP status appropriate to authentication, authorization,
validation, conflict, or dependency failure. The BFF preserves the Manager
status and structured response; it must not turn a Manager error into an empty
success payload.

## Change Rules

Additive response fields are preferred. Renaming or changing the meaning of a
field requires coordinated Manager and UI changes. UI code must consume typed
clients under `web/manager/lib/api/`; page components do not construct Manager
URLs or authorization headers.
