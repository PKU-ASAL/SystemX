# Platform Reliability Design

**Status:** Approved

**Goal:** Make Worker analysis restart-safe and horizontally scalable, make OpenSearch projection convergent and observable, bind every Manager operation to a verified tenant identity, establish ordered PostgreSQL migrations, and formalize the Incident Report contract.

## Decisions

- OpenSearch is the durable source for Event, Signal, Evidence, and Incident Report analysis data.
- Worker analysis reads a bounded 15-minute history window from OpenSearch and holds no correctness-critical history across Kafka messages.
- Worker projects one logical batch through the OpenSearch Bulk API and commits Kafka only after every required item succeeds or a permanent payload is durably dead-lettered.
- Manager authenticates asymmetric `RS256` JWTs and derives actor, tenant, and roles only from verified claims.
- PostgreSQL migrations are ordered, transactional, and append-only. Existing Incident tables are not dropped automatically.
- Incident protobuf gains explicit identity and time-range fields. Legacy workflow fields are deprecated but their field numbers remain reserved by continued declaration.

## 1. Bounded Analysis Window

### Ownership

Worker does not use `store.Store.Events`, `store.Store.Signals`, cloud Signals, metrics, or rarity state as cross-message analysis memory. The Store remains a control-plane dependency only where policy lookup is required.

For every valid DataBatch, Worker:

1. Derives one or more analysis scopes from `case_type`, `scenario`, or `workload` labels.
2. Queries OpenSearch for Event and endpoint Signal documents belonging to the verified `tenant_id`, matching scope, and observed in `[batch_time-15m, batch_time]`.
3. Merges current batch documents with history and deduplicates by deterministic document ID.
4. Runs analytics against the bounded collection.
5. Builds all output projections without mutating shared historical state.

If no analysis scope exists, Worker indexes the batch but does not run cross-document convergence.

### Time

The batch upper bound is the newest valid observation time in the current batch. If the payload contains no valid observation time, Worker uses its processing timestamp consistently for that attempt. The value is captured once before retries so a retry does not shift the analysis window.

### Kafka Ordering

Gateway uses `tenant_id + correlation_key` as the Kafka message key when a scope can be derived. Otherwise it uses `tenant_id + agent_id`. This improves ordering but is not relied on for correctness; OpenSearch history is authoritative.

## 2. Bulk Projection And Error Classes

### Interface

```go
type Projector interface {
    BulkIndex(context.Context, []Document) error
}
```

`BulkIndex` sends one `_bulk` request and parses every response item. Documents are ordered as Event, endpoint Signal, cloud Signal, Evidence, then Incident Report. All IDs are deterministic.

OpenSearch Bulk is not transactional. A partial response may leave successful documents visible. Retrying the same deterministic IDs converges to the complete projection. Kafka offset is committed only after all required items report success.

### Error Classification

```go
type ErrorClass string

const (
    ErrorTransient ErrorClass = "transient"
    ErrorPermanent ErrorClass = "permanent"
)
```

- Transient: network failure, timeout, HTTP `429`, HTTP `5xx`, or matching Bulk item status.
- Permanent: serialization failure, invalid document identity, HTTP `400`-class mapping/request errors except `408` and `429`, or matching Bulk item status.
- Unknown infrastructure failures default to transient so source data is not discarded.

Transient errors are retried three times with exponential backoff and jitter capped at two seconds. After exhaustion the Worker exits without committing the source message.

Permanent projection errors produce a structured DLQ envelope. The source offset is committed only after the DLQ append succeeds. The envelope includes source coordinates, stable failure class/code, affected document metadata, and original payload; credentials are never included.

### Observability

Metrics cover Bulk requests, documents by kind, transient/permanent failures, partial responses, retries, DLQ success/failure, and Kafka commits. Logs contain tenant, batch, topic, partition, offset, document kind, and error code without raw payloads.

## 3. JWT Principal And Tenant Binding

### Configuration

Manager requires:

```text
SYSARMOR_JWT_PUBLIC_KEY_FILE
SYSARMOR_JWT_ISSUER
SYSARMOR_JWT_AUDIENCE
```

Only `RS256` is accepted. Missing or invalid JWT configuration prevents Manager startup. The public key is loaded once and is not fetched over the network.

### Claims

```json
{
  "sub": "operator-id",
  "tenant_id": "tenant-a",
  "roles": ["viewer", "operator"],
  "iss": "sysarmor-identity",
  "aud": "sysarmor-manager",
  "exp": 1780000000
}
```

Validation requires signature, algorithm, issuer, audience, expiry, non-empty subject, non-empty tenant, and at least one recognized role.

### Authorization

JWT middleware creates:

```go
type Principal struct {
    Subject  string
    TenantID string
    Roles    []string
}
```

- `/healthz` is anonymous. All `/api/` routes require a Principal.
- Tenant scope is injected from Principal into PostgreSQL and OpenSearch operations.
- A request-supplied `tenant_id`, when present, must match Principal or receives `403`.
- `X-SysArmor-Actor`, `X-SysArmor-Role`, and static operator token no longer establish identity or privileges.
- `viewer` permits reads, `operator` permits normal control writes, and `admin` permits administrative writes within the same tenant.
- No role receives cross-tenant access in this design.

Development and tests use explicitly constructed middleware with generated test RSA keys. There is no implicit authentication bypass in the production constructor.

## 4. Ordered PostgreSQL Migrations

### Model

```go
type Migration struct {
    Version int
    Name    string
    SQL     string
}
```

The migration runner acquires the existing advisory lock, loads applied versions, and executes each missing migration in its own transaction. It inserts the migration record in the same transaction, stops on first failure, and always releases the lock.

Migration files are immutable after release:

- V1 is the archived historical baseline needed to recognize legacy databases.
- V2 records `incident_reports_to_opensearch` and stops active Incident persistence. It performs no automatic `DROP` on an existing database.
- Fresh database bootstrap uses the current control-plane schema and records V1 and V2 as applied without creating legacy Incident tables.

The runner distinguishes fresh bootstrap from upgrade by whether `schema_migrations` exists before initialization. Tests cover both paths.

### Legacy Cleanup

A read-only inspection command reports whether `incidents`, `incident_events`, and `evidence` legacy tables exist and their row counts. A separate operator-run SQL file drops them. Runtime startup never executes destructive cleanup.

## 5. Incident Report Protobuf

The compatible schema adds:

```proto
string tenant_id = 15;
string correlation_key = 16;
string analysis_version = 17;
string first_observed_at = 18;
string last_observed_at = 19;
```

Legacy fields are retained and marked deprecated:

```proto
string status = 11 [deprecated = true];
string status_reason = 12 [deprecated = true];
string status_actor = 13 [deprecated = true];
```

New Worker output must populate all five new fields and must not write identity values into labels. `analysis_version` is `incident.v1`. Timestamps use RFC3339Nano UTC strings to match existing contracts.

The Incident document ID is a hash of normalized `tenant_id`, `correlation_key`, and `analysis_version`. Report content and evidence changes update the same document. Manager accepts legacy indexed documents during one compatibility window by reading identity from labels only when formal fields are absent; all new writes use formal fields.

## Component Boundaries

```text
internal/platform/opensearch  HTTP search/bulk adapter and error classification
internal/workers/ingest       stateless orchestration and projection planning
internal/analytics            pure bounded-input analysis
internal/manager/auth         JWT verification, Principal, role checks
internal/manager/api          tenant-bound HTTP handlers
internal/store/postgres       ordered migration runner and control-plane repositories
api/proto/incident/v1         Incident Report wire contract
```

No generic event bus, repository framework, cache framework, or future IncidentCase implementation is introduced.

## Rollout

1. Add protobuf fields and dual-read compatibility.
2. Add Bulk projection and classified failures while retaining current analysis input.
3. Switch Worker to bounded OpenSearch history and remove correctness-critical telemetry memory.
4. Deploy JWT verification and update clients/deployment configuration together.
5. Deploy ordered migrations and the legacy inspection tool.
6. Rebuild OpenSearch projections from retained Kafka data if existing reports lack formal identity fields.

Each stage is independently tested and committed. Rollback restores the prior binary; no stage automatically destroys PostgreSQL data.

## Acceptance Criteria

1. Worker restart produces the same analysis result for the same OpenSearch history and batch.
2. Two Workers processing different partitions use shared OpenSearch history rather than local history.
3. Analysis queries are tenant- and time-bounded server-side.
4. Bulk partial failure leaves Kafka uncommitted and deterministic retry converges without duplicate logical documents.
5. Permanent projection errors commit only after DLQ success.
6. No Manager API operation can select a tenant different from its verified Principal.
7. Invalid algorithm, signature, issuer, audience, expiry, subject, tenant, or role is rejected.
8. Existing databases upgrade without dropping legacy Incident tables.
9. Fresh databases contain only current control-plane tables and ordered migration records.
10. New Incident Reports use formal identity/time fields; legacy workflow fields are deprecated and unwritten.
11. Unit, API, migration, product platform, and relevant performance tests pass.
