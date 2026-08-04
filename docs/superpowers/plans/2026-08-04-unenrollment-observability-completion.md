# Unenrollment Observability and Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose recoverable Agent unenrollment state and let Manager durably confirm that a revoked endpoint completed its local transition to standalone.

**Architecture:** Agent keeps enrollment authority in the existing singleton and completion delivery in a separate SQLite outbox. Manager atomically revokes the certificate and creates an `UnenrollmentRecord`, then accepts a narrowly scoped token-authenticated HTTP completion callback after Agent credentials have been deleted. Health and Manager APIs are read-only projections of those durable facts.

**Tech Stack:** Go, protobuf/gRPC, SQLite, PostgreSQL, `net/http`, existing `internal/store` backend abstraction, existing `test/` functional topology.

## Global Constraints

- Preserve the existing `standalone -> enrolling -> managed -> unenrolling -> standalone` authority state machine.
- Health is a read-only projection and must never trigger transitions or retries.
- Use a CSPRNG token of at least 256 bits; Manager persists only its SHA-256 hash.
- Certificate revocation and Manager pending-completion creation are one durable transaction.
- Standalone activation, managed-authority cleanup, and outbox `ready` are one SQLite transaction.
- All retries are idempotent and all Store failures are explicit and fail closed.
- Do not change Sensor ownership, policy authority rules, Web Console, break-glass, or data ingestion behavior.
- Preserve protobuf field numbers and add only optional fields/messages.

---

### Task 1: Agent Management Lifecycle Health Projection

**Files:**
- Modify: `api/proto/controlplane/v1/agentcontrol.proto`
- Regenerate: `api/proto/controlplane/v1/agentcontrol.pb.go`
- Modify: `internal/agent/daemon/local_control.go`
- Test: `internal/agent/daemon/local_control_test.go`
- Test: `cmd/sysarmorctl/main_test.go`

**Interfaces:**
- Produces: `HealthResponse.management_lifecycle` containing `ManagementLifecycleStatus`.
- Consumes: `localstore.Store.Enrollment(context.Context)`.

- [ ] **Step 1: Add failing Health projection tests**

```go
func TestHealthReportsUnenrollmentLifecycle(t *testing.T) {
    // Arrange a durable unenrolling enrollment with a revocation error.
    // Assert mode, transition phase, confirmation, error, and updated_at.
}

func TestHealthDegradesWhileManagementTransitionIsPending(t *testing.T) {
    // Assert HealthResponse.status == "degraded" for unenrolling state.
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/agent/daemon ./cmd/sysarmorctl -run 'TestHealth.*(Unenrollment|ManagementTransition)' -count=1`

Expected: FAIL because `management_lifecycle` does not exist.

- [ ] **Step 3: Add the protobuf projection and minimal mapper**

```proto
message ManagementLifecycleStatus {
  string mode = 1;
  string transition_phase = 2;
  bool revocation_confirmed = 3;
  string manager_completion_status = 4;
  string last_transition_error = 5;
  string updated_at = 6;
}

message HealthResponse {
  // existing fields 1-20 unchanged
  ManagementLifecycleStatus management_lifecycle = 21;
}
```

Add `managementLifecycleStatus(ctx)` beside `localStoreHealth(ctx)`. Map only durable state and set the response status to degraded when transition phase/error is non-empty.

- [ ] **Step 4: Regenerate protobuf and verify GREEN**

Run: `make api`

Run: `go test ./internal/agent/daemon ./cmd/sysarmorctl -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/proto/controlplane/v1/agentcontrol.proto api/proto/controlplane/v1/agentcontrol.pb.go internal/agent/daemon/local_control.go internal/agent/daemon/local_control_test.go cmd/sysarmorctl/main_test.go
git commit -m "feat(agent): expose management lifecycle health"
```

### Task 2: Durable Agent Completion Outbox

**Files:**
- Modify: `internal/agent/localstore/schema.go`
- Modify: `internal/agent/localstore/enrollment.go`
- Create: `internal/agent/localstore/unenrollment_completion.go`
- Test: `internal/agent/localstore/enrollment_test.go`
- Test: `internal/agent/localstore/store_test.go`

**Interfaces:**
- Produces: `PrepareUnenrollment(ctx, token, tokenHash)`, `UnenrollmentCompletion(ctx)`, `RecordCompletionAttempt(ctx, err)`, and `AcknowledgeCompletion(ctx, enrollmentID)`.
- Changes: `CompleteUnenrollment(ctx, kind)` atomically changes enrollment and the prepared outbox to `ready`.
- Consumes: normalized `Enrollment.ManagerURL` persisted during enrollment.

- [ ] **Step 1: Add failing schema, outbox, and atomicity tests**

```go
func TestPrepareUnenrollmentPersistsCompletionBeforeRevocation(t *testing.T) {
    // Assert state=unenrolling and outbox=prepared in one committed operation.
}

func TestCompleteUnenrollmentAtomicallyMarksCompletionReady(t *testing.T) {
    // After revocation confirmation, assert enrollment=standalone and outbox=ready.
}

func TestCompletionAcknowledgementRequiresMatchingEnrollment(t *testing.T) {
    // A mismatch must not delete the outbox.
}
```

Add a v3 fixture migration assertion that `manager_url` and the outbox table are available at schema version 4.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/agent/localstore -run 'Test(PrepareUnenrollment|CompleteUnenrollmentAtomically|CompletionAcknowledgement|Migrate)' -count=1`

Expected: FAIL because schema v4 and outbox APIs do not exist.

- [ ] **Step 3: Implement schema v4 and focused outbox API**

```go
type UnenrollmentCompletion struct {
    TenantID, AgentID, EnrollmentID, CertificateSerial string
    ManagerURL, RevocationReceipt                      string
    Token, TokenHash, Status, LastError                string
    AttemptCount                                       uint64
    CreatedAt, UpdatedAt                               time.Time
}
```

Add `manager_url` to enrollment, add a singleton `unenrollment_completion` table, and make the local completion transaction require a prepared outbox and mark it ready before commit. Keep token values out of errors and formatting.

- [ ] **Step 4: Verify GREEN and migration safety**

Run: `go test ./internal/agent/localstore -count=1`

Expected: PASS, including v1→v4, v2→v4, and v3→v4 migration tests.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/localstore/schema.go internal/agent/localstore/enrollment.go internal/agent/localstore/unenrollment_completion.go internal/agent/localstore/enrollment_test.go internal/agent/localstore/store_test.go
git commit -m "feat(agent): persist unenrollment completion outbox"
```

### Task 3: Durable Manager Unenrollment Records

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/backend.go`
- Create: `internal/store/unenrollment.go`
- Modify: `internal/store/migrations/postgres.go`
- Modify: `internal/store/postgres/snapshot.go`
- Test: `internal/store/store_test.go`
- Create: `internal/store/postgres/unenrollment_test.go`

**Interfaces:**
- Produces: `AuthorizeAgentUnenrollment(...) (UnenrollmentRecord, bool, error)`.
- Produces: `CompleteAgentUnenrollment(...) (UnenrollmentRecord, bool, error)`.
- Produces: `GetUnenrollmentWithError(tenantID, enrollmentID)` and `ListUnenrollmentsWithError(tenantID)`.
- Backend implements the same operations with PostgreSQL transactions.

- [ ] **Step 1: Add failing in-memory and PostgreSQL transaction tests**

```go
func TestAuthorizeAgentUnenrollmentIsIdempotent(t *testing.T) {
    // Same identity/hash returns the stable receipt; changed hash returns ErrConflict.
}

func TestCompleteAgentUnenrollmentUsesConstantBoundIdentity(t *testing.T) {
    // Hash/receipt/identity mismatch does not advance the state.
}

func TestPostgresAuthorizationRollsBackCertificateAndRecordTogether(t *testing.T) {
    // Inject the second write failure and assert neither durable fact commits.
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/store ./internal/store/postgres -run 'Test(AuthorizeAgentUnenrollment|CompleteAgentUnenrollment|PostgresAuthorization)' -count=1`

Expected: FAIL because `UnenrollmentRecord` and transactional backend methods do not exist.

- [ ] **Step 3: Implement the narrow Store domain API**

```go
type UnenrollmentRecord struct {
    TenantID, AgentID, EnrollmentID, CertificateSerial string
    RevocationReceipt, CompletionTokenHash, Status      string
    RevokedAt, EndpointCompletedAt, CreatedAt, UpdatedAt time.Time
}
```

Use status constants `revoked_endpoint_pending`, `endpoint_completed`, and `unknown_legacy`. Compare decoded SHA-256 bytes with `subtle.ConstantTimeCompare`; never expose the stored hash through public enrollment JSON.

- [ ] **Step 4: Implement PostgreSQL migration and transactions**

Create `agent_unenrollments` keyed by `(tenant_id, enrollment_id)`. Authorization locks the certificate row, validates replay identity/hash, and writes certificate plus record in one transaction. Completion locks the record, validates all bindings, and advances status once.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./internal/store ./internal/store/postgres -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/store.go internal/store/backend.go internal/store/unenrollment.go internal/store/migrations/postgres.go internal/store/postgres/snapshot.go internal/store/store_test.go internal/store/postgres/unenrollment_test.go
git commit -m "feat(manager): persist endpoint unenrollment lifecycle"
```

### Task 4: Idempotent Revocation and Completion Transport

**Files:**
- Modify: `api/proto/controlplane/v1/agentcontrol.proto`
- Regenerate: `api/proto/controlplane/v1/agentcontrol.pb.go`
- Modify: `internal/gateway/backend.go`
- Modify: `internal/gateway/control_grpc.go`
- Modify: `internal/gateway/grpc_test.go`
- Modify: `internal/manager/api/store_capabilities.go`
- Modify: `internal/manager/api/http.go`
- Modify: `internal/manager/api/http_auth.go`
- Create: `internal/manager/api/http_unenrollment_completion.go`
- Create: `internal/manager/api/http_unenrollment_completion_test.go`

**Interfaces:**
- Adds: `RevokeEnrollmentRequest.completion_token_hash`.
- Adds: `RevokeEnrollmentResponse.completion_required`.
- Adds: anonymous token-gated `POST /api/v1/unenrollment-completions`.
- Consumes: Task 3 Store methods.

- [ ] **Step 1: Add failing RPC replay and HTTP completion tests**

```go
func TestRevokeEnrollmentPersistsPendingCompletion(t *testing.T) {
    // Assert stable receipt and completion_required=true.
}

func TestRevokedCertificateCanOnlyReplayMatchingRevocation(t *testing.T) {
    // Same request succeeds; changed hash fails; ordinary control remains denied.
}

func TestUnenrollmentCompletionIsTokenAuthenticatedAndIdempotent(t *testing.T) {
    // First and repeated request succeed; any binding mismatch returns generic denial.
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/gateway ./internal/manager/api -run 'Test(RevokeEnrollmentPersists|RevokedCertificateCanOnly|UnenrollmentCompletion)' -count=1`

Expected: FAIL because protocol fields and endpoint do not exist.

- [ ] **Step 3: Extend protobuf and Gateway handler**

```proto
message RevokeEnrollmentRequest {
  // fields 1-4 unchanged
  string completion_token_hash = 5;
}

message RevokeEnrollmentResponse {
  // fields 1-3 unchanged
  bool completion_required = 4;
}
```

Treat a missing hash as a legacy Agent: preserve the old revocation behavior, record `unknown_legacy`, and return
`completion_required=false`. Reject malformed non-empty hashes. For a valid hash, permit revoked-certificate replay
only inside `RevokeEnrollment`; all existing control/data certificate validation remains unchanged. A new Agent
requires `completion_required=true` and remains unenrolling if Manager cannot provide it.

- [ ] **Step 4: Implement the bounded completion endpoint**

Decode at most the configured anonymous body limit, require the exact schema version, hash the raw token, call `CompleteAgentUnenrollment`, and return only status plus server completion time. Add the endpoint to the anonymous-path allowlist without granting it a principal.

- [ ] **Step 5: Regenerate and verify GREEN**

Run: `make api`

Run: `go test ./internal/gateway ./internal/manager/api -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/proto/controlplane/v1/agentcontrol.proto api/proto/controlplane/v1/agentcontrol.pb.go internal/gateway internal/manager/api
git commit -m "feat(control-plane): close endpoint unenrollment protocol"
```

### Task 5: Agent Completion Reporter and Coordinator Integration

**Files:**
- Modify: `internal/agent/daemon/daemon.go`
- Modify: `internal/agent/daemon/enrollment_client.go`
- Modify: `internal/agent/daemon/enrollment_revocation_client.go`
- Modify: `internal/agent/daemon/enrollment_coordinator.go`
- Create: `internal/agent/daemon/unenrollment_completion_client.go`
- Create: `internal/agent/daemon/unenrollment_completion_reporter.go`
- Test: `internal/agent/daemon/enrollment_coordinator_test.go`
- Create: `internal/agent/daemon/unenrollment_completion_reporter_test.go`

**Interfaces:**
- Produces: `completionReporter.Run(ctx)` and `completionReporter.ReportOnce(ctx) (bool, error)`.
- Changes: revocation client accepts the prepared completion hash and requires `completion_required=true`.
- Consumes: Task 2 outbox and Task 4 transport.

- [ ] **Step 1: Add failing ordering, retry, and restart tests**

```go
func TestCoordinatorPersistsCompletionBeforeRevocation(t *testing.T) {
    // The injected revoker observes a prepared durable outbox.
}

func TestCoordinatorDoesNotReportBeforeLocalCompletion(t *testing.T) {
    // Sensor/credential/SQLite failure leaves zero completion calls.
}

func TestCompletionReporterRetriesReadyOutboxAfterRestart(t *testing.T) {
    // First call fails, reconstructed reporter succeeds and clears the outbox.
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/agent/daemon -run 'Test(CoordinatorPersistsCompletion|CoordinatorDoesNotReport|CompletionReporter)' -count=1`

Expected: FAIL because token preparation and Reporter do not exist.

- [ ] **Step 3: Implement token preparation and coordinator ordering**

Generate a base64url 32-byte token, persist it before revocation, pass only its hex SHA-256 hash through gRPC, and require Manager completion capability. Do not hold `policyAuthorityMu` or the Coordinator mutex during HTTP completion reporting.

- [ ] **Step 4: Implement the minimal Reporter**

Reporter reads only `ready` outbox rows, derives `/api/v1/unenrollment-completions` from persisted Manager origin,
posts with a bounded timeout, records attempts/errors, and deletes the outbox only after an idempotent success
response. Reject non-loopback HTTP unless the existing Manager insecure-transport setting explicitly permits it.
Run Reporter on startup with bounded exponential backoff under the daemon lifecycle context.

- [ ] **Step 5: Extend Health and CLI completion status tests**

Assert `prepared -> revocation_pending`, `ready -> endpoint_completion_pending`, error propagation, and `Unenroll` returning pending until the synchronous first completion attempt succeeds.

- [ ] **Step 6: Verify GREEN and race safety**

Run: `go test ./internal/agent/localstore ./internal/agent/daemon ./cmd/sysarmorctl -count=1`

Run: `go test -race ./internal/agent/daemon ./internal/agent/localstore -count=1`

Expected: PASS with no race reports.

- [ ] **Step 7: Commit**

```bash
git add internal/agent/daemon internal/agent/localstore cmd/sysarmorctl
git commit -m "feat(agent): report durable unenrollment completion"
```

### Task 6: Manager Projection and End-to-End Closure

**Files:**
- Modify: `internal/manager/api/http_enrollments.go`
- Test: `internal/manager/api/http_enrollments_test.go`
- Modify: `test/suites/functional/topology/e2e-systemd-vm.sh`
- Modify: `test/suites/functional/topology/test_e2e_contract.py`
- Modify: `docs/guides/agent-management.md`

**Interfaces:**
- Produces: enrollment API fields `unenrollment_status`, `revoked_at`, and `endpoint_completed_at` without token material.
- Extends: topology acceptance to wait for Manager `endpoint_completed`.

- [ ] **Step 1: Add failing API privacy and topology contract tests**

```go
func TestEnrollmentQueryExposesCompletionWithoutTokenHash(t *testing.T) {
    // Assert lifecycle fields exist and serialized response contains no token/hash.
}
```

Add topology contract assertions for `endpoint_completed` and completion retry recovery.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/manager/api -run TestEnrollmentQueryExposesCompletionWithoutTokenHash -count=1`

Run: `python3 -m unittest test.suites.functional.topology.test_e2e_contract`

Expected: FAIL because projection and topology assertions do not exist.

- [ ] **Step 3: Implement the read-only Manager projection and topology flow**

Enrich public enrollment responses from `ListUnenrollmentsWithError`; never add token/hash to `store.Enrollment`. Extend the VM flow to poll Manager until `endpoint_completed`, then retain all existing standalone-policy, credential-removal, old-certificate, and restart checks.

- [ ] **Step 4: Run focused and full regression gates**

Run: `go test ./... -count=1`

Run: `go test -race ./internal/agent/daemon ./internal/agent/localstore ./internal/gateway ./internal/manager/api ./internal/store ./internal/store/postgres -count=1`

Run: `go vet ./...`

Run: `make test-functional DOMAIN=platform`

Run: `make test-functional DOMAIN=topology`

Run: `git diff --check`

Expected: all commands PASS. If the VM environment is unavailable, record the exact unavailable prerequisite; do not substitute unit tests for topology evidence.

- [ ] **Step 5: Commit**

```bash
git add internal/manager/api/http_enrollments.go internal/manager/api/http_enrollments_test.go test/suites/functional/topology docs/guides/agent-management.md
git commit -m "test(e2e): verify endpoint unenrollment completion"
```

### Task 7: Final Architecture Review

**Files:**
- Review: all files changed since `4bcb30a2`

**Interfaces:**
- Consumes: completed Tasks 1-6 and the approved design spec.
- Produces: review findings resolved or explicitly reported.

- [ ] **Step 1: Review security and ownership boundaries**

Confirm no token logging, no arbitrary callback URL, no Health side effects, no completion network I/O under authority locks, and no Store fallback after backend errors.

- [ ] **Step 2: Review failure matrix against tests**

Map every row in the design failure matrix to a named automated test. Add a failing test first for any uncovered behavior, then implement the smallest correction.

- [ ] **Step 3: Run final gates and inspect diff**

Run: `git diff 4bcb30a2 --check`

Run: `git status --short`

Expected: only intentional tracked changes/commits plus the pre-existing ignored generated artifacts.
