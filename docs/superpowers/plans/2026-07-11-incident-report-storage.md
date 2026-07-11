# Incident Report Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist Incident Reports only in OpenSearch, make Kafka acknowledgement safe under indexing failures, and remove PostgreSQL Incident persistence.

**Architecture:** Worker writes deterministic Event, Signal, and Incident Report projections to OpenSearch before committing Kafka offsets. Permanent invalid messages go to a Kafka DLQ; transient processing errors remain uncommitted. Manager reads Incident Reports only from OpenSearch, while PostgreSQL remains the control-plane store.

**Tech Stack:** Go 1.26, Kafka via `segmentio/kafka-go`, OpenSearch HTTP API, PostgreSQL, protobuf, standard Go tests.

## Global Constraints

- Follow TDD: every behavior change starts with a focused failing test.
- Do not add a future `IncidentCase` implementation.
- Do not destructively drop legacy PostgreSQL tables during automatic migration.
- Preserve unrelated working-tree changes and generated deployment snapshots.

---

### Task 1: Reliable OpenSearch Projection

**Files:**
- Modify: `internal/workers/ingest/processor.go`
- Modify: `internal/workers/ingest/worker_test.go`
- Modify: `internal/analytics/incident/incident.go`
- Modify: `internal/analytics/incident/incident_test.go`

**Interfaces:**
- Consumes: `platformopensearch.Indexer.Index(context.Context, Document) error`
- Produces: deterministic `IncidentDocumentID`, and `Processor.Process` errors for every failed required projection

- [ ] **Step 1: Write failing indexing-error tests**

Add a failing indexer to `worker_test.go`; assert `Processor.Process` returns its error for Event, Signal, cloud Signal, and Incident Report writes.

- [ ] **Step 2: Verify RED**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/workers/ingest -run 'TestProcessorReturnsIndexError|TestIncidentDocumentID' -count=1`

Expected: FAIL because indexing errors are ignored and report IDs depend on generated Incident IDs.

- [ ] **Step 3: Implement minimal propagation and deterministic identity**

Change projection helpers to return errors, stop processing on the first required projection failure, index reports directly from the current analysis result, and derive report identity from tenant/correlation inputs rather than the process-local builder counter.

- [ ] **Step 4: Verify GREEN**

Run the Task 1 test command and `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/analytics/incident ./internal/workers/ingest`.

- [ ] **Step 5: Commit**

```bash
git add internal/workers/ingest internal/analytics/incident
git commit -m "fix: make incident projections reliable"
```

### Task 2: Kafka Retry And Dead-Letter Handling

**Files:**
- Modify: `internal/platform/kafka/kafka.go`
- Modify: `internal/platform/kafka/kafka_test.go`
- Modify: `internal/workers/ingest/worker.go`
- Modify: `internal/workers/ingest/worker_test.go`
- Modify: `cmd/sysarmor-worker/main.go`
- Modify: `deployments/worker/worker.env.example`
- Modify: `deployments/compose.platform.yaml`

**Interfaces:**
- Produces: Kafka `Message` metadata (`Partition`, `Offset`), a DLQ publisher, and Worker poison-message classification

- [ ] **Step 1: Write failing commit and DLQ tests**

Test that transient processing failures do not commit, malformed payloads commit only after DLQ append succeeds, and DLQ append failures do not commit.

- [ ] **Step 2: Verify RED**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/workers/ingest ./internal/platform/kafka -run 'TestWorker.*(Failure|DLQ|Malformed)' -count=1`

Expected: FAIL because Worker currently exits on malformed messages and has no DLQ dependency.

- [ ] **Step 3: Implement minimal Worker policy**

Add source metadata to the Kafka adapter, encode a bounded dead-letter envelope, retry transient processor errors three times with capped backoff, append permanent invalid payloads to `<topic>.dlq`, then commit only after successful processing or DLQ publication.

- [ ] **Step 4: Verify GREEN**

Run Task 2 tests and `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/platform/kafka ./internal/workers/ingest ./cmd/sysarmor-worker`.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/kafka internal/workers/ingest cmd/sysarmor-worker deployments/worker deployments/compose.platform.yaml
git commit -m "feat: add reliable ingest retry and dead letter handling"
```

### Task 3: OpenSearch-Only Manager Incident Reads

**Files:**
- Modify: `internal/manager/api/http.go`
- Modify: `internal/manager/api/http_incidents.go`
- Modify: `internal/manager/api/http_incidents_test.go`
- Modify: `internal/manager/api/http_telemetry_test.go`
- Modify: `internal/platform/opensearch/opensearch.go`
- Modify: `internal/platform/opensearch/opensearch_test.go`

**Interfaces:**
- Produces: tenant-scoped Incident Report list/detail reads through the existing OpenSearch search abstraction

- [ ] **Step 1: Write failing query-boundary tests**

Assert Incident list/detail requires `tenant_id`, sends a server-side tenant filter, never falls back to Store, and mutation routes return `404`.

- [ ] **Step 2: Verify RED**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/manager/api ./internal/platform/opensearch -run 'Test.*Incident' -count=1`

Expected: FAIL because Manager currently permits Store fallback and registers mutation routes.

- [ ] **Step 3: Implement minimal read-only API**

Use OpenSearch for Incident list and detail, require tenant scope, remove Incident mutation handlers and their `ManagerStore` methods, and remove the three mutation route registrations.

- [ ] **Step 4: Verify GREEN**

Run Task 3 tests and `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/manager/api ./internal/platform/opensearch ./cmd/sysarmor-manager`.

- [ ] **Step 5: Commit**

```bash
git add internal/manager/api internal/platform/opensearch
git commit -m "refactor: make incident reports read only"
```

### Task 4: Remove PostgreSQL Incident Projection

**Files:**
- Modify: `internal/store/backend.go`
- Modify: `internal/store/store.go`
- Modify: `internal/store/postgres/snapshot.go`
- Modify: `internal/store/postgres/migrate_test.go`
- Modify: `internal/store/backend/backend_test.go`
- Modify: `internal/store/migrations/postgres.go`
- Modify: `internal/store/migrations/postgres_test.go`

**Interfaces:**
- Removes: `Backend.ListIncidents` and PostgreSQL Incident snapshot projection
- Preserves: in-memory Incident analysis only where required by isolated analytics code

- [ ] **Step 1: Change persistence tests first**

Assert fresh schema no longer creates Incident tables or indexes, `SaveState` performs no Incident INSERT, and opening the PostgreSQL backend does not hydrate Incident data.

- [ ] **Step 2: Verify RED**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/store/migrations ./internal/store/postgres ./internal/store/backend -run 'Test.*Incident|TestApplyMigrations' -count=1`

Expected: FAIL because schema and snapshot projection still persist Incidents.

- [ ] **Step 3: Remove persistence code**

Remove Incident tables and indexes from the fresh schema, remove the backend query/projection methods, and stop exporting Incidents to a durable backend. Do not issue `DROP TABLE` for existing databases.

- [ ] **Step 4: Verify GREEN and regression suite**

Run:

```bash
GOCACHE=/tmp/sysarmor-go-cache go test ./internal/store/... ./internal/workers/ingest ./internal/manager/api
GOCACHE=/tmp/sysarmor-go-cache go test ./...
```

Expected: all tests pass; environment-restricted socket tests may require an unsandboxed rerun.

- [ ] **Step 5: Commit**

```bash
git add internal/store
git commit -m "refactor: remove postgres incident persistence"
```

### Task 5: End-To-End Verification And Documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/telemetry-semantics.md`
- Modify: `test/suites/product/platform/STORAGE.md`

**Interfaces:**
- Documents: OpenSearch ownership, at-least-once projection, DLQ behavior, and future Case boundary

- [ ] **Step 1: Update architecture documentation**

Document the verified implementation only, including operational replay and legacy-table cleanup behavior.

- [ ] **Step 2: Run formatting and static verification**

Run:

```bash
gofmt -w <changed-go-files>
GOCACHE=/tmp/sysarmor-go-cache go vet ./...
GOCACHE=/tmp/sysarmor-go-cache go test ./...
git diff --check
```

- [ ] **Step 3: Review diff against the approved specification**

Confirm every changed line maps to reliable projection, OpenSearch-only Incident reads, PostgreSQL projection removal, or required documentation/test updates.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/architecture/telemetry-semantics.md test/suites/product/platform/STORAGE.md
git commit -m "docs: document incident report data ownership"
```
