# Platform Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement restart-safe bounded analysis, convergent Bulk projection, RS256 JWT tenant binding, ordered PostgreSQL migrations, and a formal Incident Report contract.

**Architecture:** OpenSearch supplies bounded historical analysis data and receives deterministic Bulk projections. Manager derives tenant scope from verified JWT Principals. PostgreSQL is restricted to ordered, transactional control-plane migrations.

**Tech Stack:** Go 1.26, protobuf, OpenSearch HTTP Bulk/Search APIs, Kafka, PostgreSQL, `github.com/golang-jwt/jwt/v5`, standard Go tests.

## Global Constraints

- Follow TDD for every behavior change.
- Use a fixed 15-minute analysis window and `incident.v1` analysis version.
- Accept JWT algorithm `RS256` only.
- Do not automatically drop legacy PostgreSQL tables.
- Do not introduce IncidentCase or generic framework abstractions.
- Preserve unrelated untracked Manager UI documents.

---

### Task 1: Formal Incident Report Contract

**Files:**
- Modify: `api/proto/incident/v1/incident.proto`
- Regenerate: `api/proto/incident/v1/incident.pb.go`
- Modify: `internal/analytics/incident/incident.go`
- Modify: `internal/analytics/incident/incident_test.go`
- Modify: `internal/workers/ingest/processor.go`
- Modify: `internal/workers/ingest/worker_test.go`

**Interfaces:**
- Produces: formal tenant, correlation, analysis version, and observation range fields on every new Incident Report.

- [ ] Add failing tests asserting formal fields, `incident.v1`, stable document identity, RFC3339Nano ranges, and empty deprecated workflow fields.
- [ ] Run `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/analytics/incident ./internal/workers/ingest -count=1` and verify expected failures.
- [ ] Add protobuf fields 15-19, mark fields 11-13 deprecated, run `make api`, and minimally populate the fields in the builder/orchestrator.
- [ ] Re-run focused tests and protobuf compatibility tests.
- [ ] Commit with `feat: formalize incident report identity`.

### Task 2: Bulk Projection And Classified Errors

**Files:**
- Modify: `internal/platform/opensearch/opensearch.go`
- Modify: `internal/platform/opensearch/opensearch_test.go`
- Modify: `internal/workers/ingest/processor.go`
- Modify: `internal/workers/ingest/worker.go`
- Modify: `internal/workers/ingest/worker_test.go`

**Interfaces:**
- Produces: `Projector.BulkIndex`, classified projection errors, and one projection plan per DataBatch.

- [ ] Add failing HTTP tests for valid NDJSON, all-success responses, item-level `429/5xx`, item-level mapping `400`, malformed response, and deterministic replay.
- [ ] Verify RED with `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/platform/opensearch ./internal/workers/ingest -run 'Test.*(Bulk|Projection|Retry)' -count=1`.
- [ ] Implement the smallest Bulk adapter and error type; remove per-document Worker writes.
- [ ] Add Worker tests proving transient failure does not commit, permanent projection failure commits only after DLQ, and partial success converges on retry.
- [ ] Run both package suites and commit with `feat: project telemetry through opensearch bulk`.

### Task 3: Stateless Bounded Analysis

**Files:**
- Create: `internal/workers/ingest/history.go`
- Create: `internal/workers/ingest/history_test.go`
- Modify: `internal/workers/ingest/processor.go`
- Modify: `internal/workers/ingest/worker_test.go`
- Modify: `internal/platform/opensearch/opensearch.go`
- Modify: `internal/gateway/runtime.go`
- Modify: `internal/gateway/grpc_test.go`

**Interfaces:**
- Consumes: tenant/scope/time-bounded OpenSearch search.
- Produces: pure merge-and-deduplicate analysis input without cross-message Store telemetry.

- [ ] Add failing tests for server-side tenant/scope/time filters, current-batch merge, duplicate IDs, restart equivalence, and partition-independent shared history.
- [ ] Verify RED using focused ingest and gateway tests.
- [ ] Implement `HistoryReader`, fixed-window selection, pure merge helpers, and Kafka correlation keys.
- [ ] Remove Worker reliance on Store Event/Signal history and keep policy lookup only.
- [ ] Run ingest, analytics, OpenSearch, and gateway tests; commit with `refactor: make ingest analysis stateless`.

### Task 4: RS256 JWT Principal And Tenant Binding

**Files:**
- Create: `internal/manager/auth/jwt.go`
- Create: `internal/manager/auth/jwt_test.go`
- Modify: `internal/manager/api/http.go`
- Modify: `internal/manager/api/http_identity.go`
- Modify: Manager API test helpers and affected tests under `internal/manager/api/`
- Modify: `cmd/sysarmor-manager/main.go`
- Modify: `deployments/manager/manager.env.example`
- Modify: `deployments/compose.platform.yaml`
- Modify: Manager CLI callers under `cmd/sysarmorctl/main.go`

**Interfaces:**
- Produces: verified `Principal` context and role/tenant authorization middleware.

- [ ] Add failing tests for RS256 success and rejection of wrong algorithm, signature, issuer, audience, expiry, subject, tenant, and roles.
- [ ] Add failing API tests for anonymous rejection, health exemption, tenant mismatch `403`, server-injected tenant, and viewer/operator/admin roles.
- [ ] Add `github.com/golang-jwt/jwt/v5`, implement verifier/middleware, remove static operator-token and trusted identity headers, and make production startup require JWT configuration.
- [ ] Update test constructors to use explicit authenticated Principals; update deployment examples without hardcoded private keys or tokens.
- [ ] Run auth, Manager API, CLI, and command tests; commit with `feat: bind manager access to jwt principals`.

### Task 5: Ordered PostgreSQL Migrations

**Files:**
- Replace: `internal/store/migrations/postgres.go`
- Modify: `internal/store/migrations/postgres_test.go`
- Modify: `internal/store/postgres/migrate.go`
- Modify: `internal/store/postgres/migrate_test.go`
- Modify: `internal/store/postgres/fake_driver_test.go`
- Create: `internal/store/migrations/legacy_incident_cleanup.sql`
- Create: `cmd/sysarmorctl/storage_inspect.go` or a focused equivalent following current CLI structure
- Modify: `cmd/sysarmorctl/main.go`
- Modify: `test/suites/product/platform/STORAGE.md`

**Interfaces:**
- Produces: ordered `Migration` list, transactional runner, fresh bootstrap, legacy upgrade, and read-only legacy inspection.

- [ ] Add failing tests for ordered application, per-version transaction, rollback, idempotent reopen, fresh schema without Incident tables, legacy upgrade without DROP, and lock release.
- [ ] Verify RED with migration/store focused tests.
- [ ] Implement V1/V2 migrations and fresh/legacy detection without modifying applied migration SQL.
- [ ] Add read-only legacy table inspection and separate manual cleanup SQL; never execute cleanup at runtime.
- [ ] Run migration, backend, CLI, and PostgreSQL product tests; commit with `refactor: add ordered postgres migrations`.

### Task 6: Documentation And End-To-End Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/telemetry-semantics.md`
- Modify: `test/suites/product/platform/STORAGE.md`
- Modify: relevant deployment and test documentation.

**Interfaces:**
- Documents: JWT provisioning, Bulk/DLQ semantics, bounded history, migration inspection, rollback, and protobuf compatibility.

- [ ] Add or update product tests for restart equivalence, partial Bulk failure, tenant isolation, JWT failure, and legacy migration.
- [ ] Run `make api`, `gofmt`, `go vet ./...`, `go test ./...`, relevant product platform tests, and `git diff --check`.
- [ ] Review every diff line against the approved specification and remove stale Store telemetry/identity code introduced by this change.
- [ ] Request code review and resolve every Critical/Important finding.
- [ ] Commit documentation with `docs: document platform reliability boundaries`.
