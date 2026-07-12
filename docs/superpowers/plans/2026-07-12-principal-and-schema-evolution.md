# Principal Boundary And Schema Evolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove every static Manager token path and establish explicit data-plane, protobuf, Worker, and OpenSearch schema evolution boundaries.

**Architecture:** Manager authorization consumes only a verified request-scoped Principal; test-only helpers inject Principals without adding a runtime bypass. `DataBatch` carries its transport schema version, Worker validates a bounded compatibility window, and all OpenSearch callers use centralized read/write aliases.

**Tech Stack:** Go, Protobuf, RS256 JWT, Kafka, OpenSearch, repository Make targets.

## Global Constraints

- `/healthz` is anonymous; every `/api/` request requires Principal.
- No static token, identity header, debug switch, or implicit local privilege remains.
- `schema_version` and `analysis_version` remain independent.
- Current data-plane schema is `sysarmor.dataplane/v1`; empty legacy schema is accepted for one release window only.
- Unsupported non-empty schema is a permanent failure committed only after DLQ success.
- Protobuf field numbers and names are never reused.
- OpenSearch callers use stable read/write aliases, never physical versioned index names.
- Do not modify the user-owned untracked Manager UI documents or the VM deployment snapshot.

---

### Task 1: Principal-Only Manager Authorization

**Files:**
- Create: `internal/manager/api/http_test_auth_test.go`
- Modify: `internal/manager/api/http.go`
- Modify: `internal/manager/api/http_identity.go`
- Modify: `internal/manager/api/http_auth_test.go`
- Modify: Manager API `*_test.go` files using `NewServerWithOperatorToken` or identity headers
- Modify: `docs/architecture/manager-ui-api-contract.md`

**Interfaces:**
- Produces: `withTestPrincipal(*http.Request, string, string, ...string) *http.Request` for tests only.
- Produces: `NewServerWithSearch(ManagerStore, platformopensearch.Searcher) *Server` with no token parameter.
- Consumes: `managerauth.WithPrincipal` and `Principal.HasRole`.

- [ ] **Step 1: Write failing boundary tests**

Add tests proving a bare protected Handler returns `401`, forged actor/role/static-token headers cannot authorize it, operator cannot perform admin work, and an injected admin Principal can. Define the test-only helper:

```go
func withTestPrincipal(req *http.Request, subject, tenantID string, roles ...string) *http.Request {
	return req.WithContext(managerauth.WithPrincipal(req.Context(), managerauth.Principal{
		Subject: subject, TenantID: tenantID, Roles: roles,
	}))
}
```

- [ ] **Step 2: Verify RED**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/manager/api -run 'Test.*(Principal|Forged|Unauthenticated)' -count=1`

Expected: FAIL because local Handler and token/header fallbacks still authorize requests.

- [ ] **Step 3: Remove the compatibility path**

Delete `Server.operatorToken`, `NewServerWithOperatorToken`, token-bearing constructor parameters, `operatorAuthorized`, header role lookup, and implicit authorization. Make `requireOperator` return `401` without Principal and `403` for insufficient roles. Derive actor and role only from Principal. Keep the JWT middleware as the only production Principal creator.

- [ ] **Step 4: Migrate Handler tests**

Replace token constructors and identity headers with `NewServer`/`NewServerWithSearch` plus `withTestPrincipal`. Keep tests that intentionally omit Principal unchanged so they prove fail-closed behavior. Retain real signed-JWT coverage in `internal/manager/auth` and a protected API integration test in `http_auth_test.go`.

- [ ] **Step 5: Verify GREEN and remove stale contracts**

Run:

```bash
GOCACHE=/tmp/sysarmor-go-cache go test ./internal/manager/api ./internal/manager/auth -count=1
rg -n 'operatorToken|NewServerWithOperatorToken|X-SysArmor-Operator-Token|X-SysArmor-Actor|X-SysArmor-Role' internal/manager cmd/sysarmor-manager cmd/sysarmorctl docs/architecture
```

Expected: tests PASS; search returns no static identity compatibility path. `cmd/sysarmorctl` keeps only `Authorization: Bearer <JWT>`.

- [ ] **Step 6: Commit**

```bash
git add internal/manager/api internal/manager/auth cmd/sysarmorctl docs/architecture/manager-ui-api-contract.md
git commit -m "refactor: remove static manager identity paths"
```

### Task 2: DataBatch Schema Contract And Producers

**Files:**
- Modify: `api/proto/dataplane/v1/dataplane.proto`
- Regenerate: `api/proto/dataplane/v1/dataplane.pb.go`
- Create: `internal/contracts/schema/schema.go`
- Create: `internal/contracts/schema/schema_test.go`
- Modify: `internal/agent/telemetry/batcher.go`
- Modify: `internal/endpoint/dataappend/stream.go`
- Modify: producer tests under `internal/agent` and `internal/endpoint`

**Interfaces:**
- Produces: `schema.DataPlaneCurrent = "sysarmor.dataplane/v1"`.
- Produces: `schema.ValidateDataPlane(string) (legacy bool, err error)`.
- Produces: `DataBatch.schema_version` at protobuf field number `4`.

- [ ] **Step 1: Write failing contract and producer tests**

Test that the current value is accepted, empty is reported as legacy, unsupported versions return a typed permanent compatibility error, and finalized Agent/dataappend batches always contain the current version.

- [ ] **Step 2: Verify RED**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/contracts/schema ./internal/agent/telemetry ./internal/endpoint/dataappend -run 'Test.*SchemaVersion' -count=1`

Expected: FAIL because the contract package and protobuf field do not exist.

- [ ] **Step 3: Add and generate the protobuf field**

Add this field without changing existing numbers:

```proto
message DataBatch {
  BatchHeader header = 1;
  repeated EventFrame events = 2;
  repeated SignalFrame signals = 3;
  string schema_version = 4;
}
```

Run: `make api`.

- [ ] **Step 4: Implement the minimal compatibility policy**

Create one schema package containing the constant, a typed unsupported-version error, and validation accepting only the current value or empty legacy value. Set the current version when batches are created or finalized so custom builders cannot emit new empty-version batches.

- [ ] **Step 5: Verify GREEN and commit**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/contracts/schema ./internal/agent/telemetry ./internal/endpoint/dataappend -count=1`

Expected: PASS.

```bash
git add api/proto/dataplane/v1 internal/contracts/schema internal/agent/telemetry internal/endpoint/dataappend
git commit -m "feat: version data plane batch schema"
```

### Task 3: Worker Compatibility Window And DLQ

**Files:**
- Modify: `internal/workers/ingest/worker.go`
- Modify: `internal/workers/ingest/worker_test.go`
- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `schema.ValidateDataPlane`.
- Produces: stable DLQ failure code `unsupported_schema_version`.
- Produces: `Metrics.LegacyDataBatches` and `Store.RecordLegacyDataBatch()`.

- [ ] **Step 1: Write failing Worker tests**

Test that current schema processes normally, empty legacy schema processes and increments the legacy counter, unsupported schema writes one DLQ envelope with `failure_class` and `failure_code` equal to `unsupported_schema_version`, DLQ success commits, and DLQ failure does not commit.

- [ ] **Step 2: Verify RED**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/workers/ingest ./internal/store -run 'Test.*(Schema|Legacy)' -count=1`

Expected: FAIL because Worker does not validate schema and metrics do not expose legacy use.

- [ ] **Step 3: Validate before processing**

After protobuf decoding and before identity validation, call `schema.ValidateDataPlane`. Record empty legacy acceptance once per consumed source message. Reject unsupported versions through the existing DLQ path without retrying the Processor.

- [ ] **Step 4: Verify GREEN and commit**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/workers/ingest ./internal/store -count=1`

Expected: PASS.

```bash
git add internal/workers/ingest internal/store
git commit -m "feat: enforce worker schema compatibility window"
```

### Task 4: Stable OpenSearch Read And Write Aliases

**Files:**
- Create: `internal/platform/opensearch/indexes.go`
- Create: `internal/platform/opensearch/indexes_test.go`
- Modify: `internal/workers/ingest/processor.go`
- Modify: `internal/workers/ingest/history.go`
- Modify: `internal/manager/api/http_incidents.go`
- Modify: `internal/manager/api/http_telemetry.go`
- Modify: `internal/manager/api/http_ui_overview.go`
- Modify: `internal/manager/api/http_search.go`
- Modify: affected tests and `test/shared/harness/start-container.sh`
- Create: `docs/operations/opensearch-schema-evolution.md`

**Interfaces:**
- Produces: typed constants `EventsReadAlias`, `EventsWriteAlias`, `SignalsReadAlias`, `SignalsWriteAlias`, `IncidentsReadAlias`, `IncidentsWriteAlias`, `EvidenceReadAlias`, and `EvidenceWriteAlias`.

- [ ] **Step 1: Write failing alias tests**

Test exact stable alias names and assert Worker documents use write aliases while history and Manager queries use read aliases.

- [ ] **Step 2: Verify RED**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/platform/opensearch ./internal/workers/ingest ./internal/manager/api -run 'Test.*Alias' -count=1`

Expected: FAIL because callers still use unversioned physical-looking names.

- [ ] **Step 3: Centralize and adopt aliases**

Add constants in the OpenSearch package and replace application index literals. Preserve accepted search API selectors such as `events-*`, but translate them to read aliases internally. Do not add runtime fallback to old indices.

- [ ] **Step 4: Document deterministic migration**

Document creation of `*-v1` physical indices, read/write alias creation, incompatible mapping migration through a new version, reindex validation, atomic `_aliases` switch, and rollback. Update the test harness cleanup to cover aliases and versioned test indices.

- [ ] **Step 5: Verify GREEN and commit**

Run: `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/platform/opensearch ./internal/workers/ingest ./internal/manager/api -count=1`

Expected: PASS.

```bash
git add internal/platform/opensearch internal/workers/ingest internal/manager/api test/shared/harness/start-container.sh docs/operations/opensearch-schema-evolution.md
git commit -m "refactor: route opensearch access through aliases"
```

### Task 5: Repository-Wide Contract Verification

**Files:**
- Modify: architecture documents containing active static-token or unversioned-index contracts
- Modify: tests or fixtures exposed by the full verification run

**Interfaces:**
- Consumes: all preceding task contracts.
- Produces: one coherent documented evolution policy with no active compatibility bypass.

- [ ] **Step 1: Run focused contract scans**

```bash
rg -n 'operatorToken|NewServerWithOperatorToken|X-SysArmor-Operator-Token|X-SysArmor-Actor|X-SysArmor-Role' internal cmd docs/architecture
rg -n 'Index: "sysarmor-(events|signals|incidents|evidence)"' internal
```

Expected: no production compatibility path and no direct application index literal. Historical design documents may describe removed behavior explicitly and are not rewritten.

- [ ] **Step 2: Run generated-code and full Go verification**

```bash
make api
GOCACHE=/tmp/sysarmor-go-cache go test ./...
GOCACHE=/tmp/sysarmor-go-cache go vet ./...
git diff --check
```

Expected: all commands PASS.

- [ ] **Step 3: Review scope and commit final documentation fixes**

Confirm every changed line maps to the approved specification, no file exceeds the project's decomposition threshold because of this change, and the two user-owned untracked UI documents remain untouched.

```bash
git add docs/architecture docs/operations
git commit -m "docs: establish schema evolution rules"
```
