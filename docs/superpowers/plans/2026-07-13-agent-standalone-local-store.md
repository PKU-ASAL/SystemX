# Agent Standalone Local Store Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make SysArmor Agent standalone by default with durable local Event/Signal storage, stable device identity, bounded disk usage, and an opt-in managed uploader.

**Architecture:** `internal/agent/localstore` owns SQLite state and Protobuf segment files behind focused interfaces. Agent detection commits DataBatch locally before publishing live data; managed upload consumes committed segments using a durable checkpoint, while standalone starts no external connection. Enrollment changes only the network supervisor and never restarts the sensor.

**Tech Stack:** Go 1.24, `modernc.org/sqlite`, `database/sql`, Protobuf, Zstd, CRC32C, gRPC Unix socket, systemd.

## Global Constraints

- Preserve `CGO_ENABLED=0` static Agent builds.
- Event sustained target is 1,000 EPS; burst target is 5,000 EPS for 60 seconds.
- Signal sustained target is 100 EPS; burst target is 1,000 EPS for 60 seconds.
- Default local data hard limit is 10GiB and Signal limit is 100,000 rows.
- SQLite never stores Event payloads or one row per Event.
- Local Store is the sole durable telemetry source of truth.
- Standalone mode creates no Manager/Gateway connection attempts.
- Registration defaults to data created after enrollment; history requires explicit opt-in.
- No compatibility path for legacy `manager.transport: local`; new Agent installs only.
- Every behavior change follows red-green-refactor and ends in an atomic Conventional Commit.

---

### Task 1: SQLite Baseline and Device Identity

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/agent/localstore/store.go`
- Create: `internal/agent/localstore/schema.go`
- Create: `internal/agent/localstore/identity.go`
- Create: `internal/agent/localstore/store_test.go`

**Interfaces:**
- Produces: `Open(ctx context.Context, Options) (*Store, error)`.
- Produces: `Store.DeviceIdentity(ctx) (DeviceIdentity, error)`.
- Produces: `Store.Close() error`.
- `Options` contains `RootDir`, `MaxBytes`, `MinFreeBytes`, `SegmentSize`, and `SignalMaxCount`.

- [ ] Write failing tests proving an empty directory creates mode-0700 directories, a SQLite baseline, one stable device identity, standalone enrollment, WAL mode, foreign keys, busy timeout, and idempotent reopen.
- [ ] Run `GOCACHE=/tmp/sysarmor-go-cache go test ./internal/agent/localstore -run 'TestOpen|TestDeviceIdentity' -count=1`; expect missing package failure.
- [ ] Add `modernc.org/sqlite` with `go get modernc.org/sqlite` and implement the minimal schema transaction. Generate device ID from timestamp plus cryptographic randomness in canonical UUIDv7 form without adding another dependency.
- [ ] Re-run focused tests and `CGO_ENABLED=0 go test ./internal/agent/localstore`; expect PASS.
- [ ] Commit with `feat(agent): add local state baseline`.

### Task 2: Policy, Enrollment, and Signal Repository

**Files:**
- Create: `internal/agent/localstore/policy.go`
- Create: `internal/agent/localstore/policy_test.go`
- Create: `internal/agent/localstore/enrollment.go`
- Create: `internal/agent/localstore/enrollment_test.go`
- Create: `internal/agent/localstore/signals.go`
- Create: `internal/agent/localstore/signals_test.go`

**Interfaces:**
- Produces: `PutPolicy`, `Policy`, `SetManaged`, `SetStandalone`, `Enrollment`.
- Produces: `AppendSignals(ctx, []*signalv1.Signal) error`.
- Produces: `QuerySignals(ctx, SignalQuery) ([]*signalv1.Signal, error)`.
- Produces: `PruneSignals(ctx) (uint64, error)`.

- [ ] Write failing tests for policy atomic replacement, restart recovery, standalone/managed constraints, protobuf Signal round-trip, rule/severity/time filters, deterministic ordering, and max-count pruning.
- [ ] Run focused tests and observe missing methods.
- [ ] Implement parameterized SQL, bounded query limits, three specified indexes, protobuf payloads, and batched transactions.
- [ ] Re-run package tests and verify malformed protobuf/invalid enrollment fails explicitly.
- [ ] Commit with `feat(agent): persist local policy signals and enrollment`.

### Task 3: Segment Codec and Crash Recovery

**Files:**
- Create: `internal/agent/localstore/segment_format.go`
- Create: `internal/agent/localstore/segment_writer.go`
- Create: `internal/agent/localstore/segment_reader.go`
- Create: `internal/agent/localstore/segment_test.go`
- Create: `internal/agent/localstore/recovery_test.go`

**Interfaces:**
- Produces: `Store.AppendBatch(ctx, *dataplanev1.DataBatch) (Position, error)`.
- Produces: `Store.ReadBatches(ctx, ReadOptions) (BatchIterator, error)`.
- Produces: `Store.Seal(ctx) error`.
- `Position` contains segment ID, record offset, batch sequence, and batch ID.

- [ ] Write failing golden tests for `SYSASEG1`, format version, Zstd payload, record length limits, CRC32C, 64MiB rotation override, `.open`/`.seg` rename, and protobuf round-trip.
- [ ] Implement fixed-width big-endian headers and bounded decompression. Keep each codec function below 50 lines.
- [ ] Write failing recovery tests for a complete `.open`, truncated tail, bad middle CRC, SQLite metadata divergence, and deterministic segment ordering.
- [ ] Implement recovery: truncate only incomplete tail, reject middle corruption, and reconcile SQLite from files.
- [ ] Run `go test ./internal/agent/localstore -run 'TestSegment|TestRecover' -count=1`; expect PASS.
- [ ] Commit with `feat(agent): add durable event segment spool`.

### Task 4: Capacity Governance and Checkpoints

**Files:**
- Create: `internal/agent/localstore/capacity.go`
- Create: `internal/agent/localstore/capacity_test.go`
- Create: `internal/agent/localstore/checkpoint.go`
- Create: `internal/agent/localstore/checkpoint_test.go`
- Create: `internal/agent/localstore/stats.go`

**Interfaces:**
- Produces: `EnforceCapacity(ctx) (CapacityResult, error)`.
- Produces: `Checkpoint`, `SaveCheckpoint`, and `Stats`.
- `CapacityResult` reports removed segments, Event/Batch counts, and whether unuploaded data was removed.

- [ ] Write failing tests with a reduced quota proving cleanup order: uploaded sealed segments, excess Signals, then oldest unuploaded sealed segments; never current `.open` or credentials/content.
- [ ] Implement directory accounting, free-space injection for tests, checkpoint advancement over forced deletion, and persistent dropped counters.
- [ ] Write checkpoint tests proving periodic persistence permits bounded duplicate replay but never skips acknowledged data.
- [ ] Run all localstore tests and a race test; expect PASS.
- [ ] Commit with `feat(agent): bound local telemetry storage`.

### Task 5: Local-first Agent Runtime

**Files:**
- Modify: `internal/agent/config/config.go`
- Modify: `internal/agent/config/config_test.go`
- Modify: `configs/agent.example.yaml`
- Modify: `internal/agent/daemon/daemon.go`
- Modify: `internal/agent/daemon/transport_runtime.go`
- Modify: `internal/agent/daemon/transport_runtime_component.go`
- Modify: `internal/agent/daemon/daemon_test.go`
- Delete runtime use of `localBatchSender`.

**Interfaces:**
- Agent config introduces `agent.state_path` and `storage.*`.
- `LocalStoreAppender.SendBatch` commits Event segment and Signal rows before returning accepted.
- Network supervisor consumes Local Store only in managed state.

- [ ] Write config tests proving standalone defaults do not require agent ID, tenant, token, Manager address, or TLS; reject legacy `manager.transport: local`.
- [ ] Implement new config baseline and map default storage settings.
- [ ] Write daemon tests proving standalone starts with fake sensor, creates no network sender/control channel, persists a DataBatch, and restores it after restart.
- [ ] Replace in-memory local ACK with Local Store commit. Publish watchers only after commit.
- [ ] Ensure managed disconnection cannot block sensor/local persistence and remove transport branching from the detection path.
- [ ] Run `go test ./internal/agent/... ./cmd/sysarmor-agent -count=1`; expect PASS.
- [ ] Commit with `refactor(agent): make telemetry local first`.

### Task 6: Local Query and Health API

**Files:**
- Modify: `api/proto/controlplane/v1/controlplane.proto`
- Regenerate: `api/proto/controlplane/v1/controlplane.pb.go`
- Regenerate: `api/proto/controlplane/v1/controlplane_grpc.pb.go`
- Modify: `internal/agent/daemon/local_control.go`
- Modify: `internal/agent/daemon/local_control_test.go`
- Modify: `internal/agent/health/health.go`
- Modify: `cmd/sysarmorctl/main.go`
- Modify: `cmd/sysarmorctl/main_test.go`

**Interfaces:**
- Add local `Status`, `QueryEvents`, and `QuerySignals` RPC request/response fields without reusing protobuf numbers.
- CLI exposes `status`, `event query`, and `signal query`; existing watch reads durable recent data before live subscription.

- [ ] Write protobuf/API tests for storage stats, Event limited reverse scan, Signal indexed filters, pagination bounds, and sequence de-duplication between recent and live data.
- [ ] Regenerate API and implement local RPCs against Local Store only; ctl must not open files or SQLite.
- [ ] Add health fields for mode, storage, checkpoint, dropped counts, and degraded reason. Standalone backlog is healthy.
- [ ] Run `make api`, focused daemon/ctl tests, then `git diff --check`.
- [ ] Commit with `feat(agent): expose durable local telemetry`.

### Task 7: Managed Uploader Supervisor

**Files:**
- Create: `internal/agent/daemon/network_supervisor.go`
- Create: `internal/agent/daemon/network_supervisor_test.go`
- Create: `internal/agent/daemon/spool_uploader.go`
- Create: `internal/agent/daemon/spool_uploader_test.go`
- Modify: `internal/agent/daemon/transport_runtime.go`
- Modify: `internal/agent/daemon/control_channel.go`

**Interfaces:**
- Supervisor supports idempotent `ApplyEnrollment(Enrollment)` and `StopManaged()`.
- Uploader reads segment positions, sends DataBatch, and checkpoints only accepted/duplicate ACKs.

- [ ] Write tests proving standalone has zero dials, managed starts upload/control, repeated apply is idempotent, enrollment change restarts only network components, and unset stops them.
- [ ] Write uploader tests for ACK semantics, retry, restart checkpoint, duplicate replay, segment deletion after checkpoint, and default managed-from sequence.
- [ ] Implement supervisor and uploader without changing sensor runtime context.
- [ ] Run daemon, gateway, and dataappend tests; expect PASS.
- [ ] Commit with `feat(agent): upload managed local telemetry`.

### Task 8: Enrollment UX and Standalone Installation

**Files:**
- Modify: `api/proto/controlplane/v1/controlplane.proto`
- Modify: `internal/agent/daemon/local_control.go`
- Modify: `cmd/sysarmorctl/main.go`
- Create: `cmd/sysarmorctl/enroll_test.go`
- Modify: `deployments/agent/install-agent.sh`
- Modify: `deployments/agent/systemd/sysarmor-agent.service`
- Create: `deployments/agent/standalone.yaml`
- Modify: `deployments/pki/agent-plane-mtls/agent.yaml.example`
- Modify: `README.md`
- Modify: `deployments/README.md`

**Interfaces:**
- `sysarmorctl enroll --manager URL [--upload-history]` orchestrates CSR and enrollment.
- `sysarmorctl unenroll` switches local state and removes credential files after commit.
- Installer creates/uses standalone config and starts without network readiness ordering.

- [ ] Write failing ctl/daemon tests for CSR, certificate validation, atomic credential write, enrollment rollback, history choice, and local unenroll semantics.
- [ ] Implement enrollment without placing private keys in SQLite, CLI args, logs, or responses.
- [ ] Change systemd ordering so network-online is not required for standalone startup.
- [ ] Add installation test proving no Manager config is needed and no external dial occurs.
- [ ] Commit with `feat(agent): add standalone enrollment lifecycle`.

### Task 9: Performance and Full Acceptance

**Files:**
- Create: `test/suites/performance/endpoint/local-store.sh`
- Create: `test/suites/product/endpoint/standalone-local-store.sh`
- Modify: `test/Makefile`
- Modify: `test/DETAILS.md`
- Modify: `README.md`

- [ ] Add deterministic reduced-quota product tests for restart recovery, capacity pressure, registration upload boundary, disconnect/reconnect, and no standalone egress.
- [ ] Add performance workload for 1,000 EPS sustained, 5,000 EPS burst, and 100 Signal EPS, recording CPU, RSS, disk throughput, compression ratio, fsync latency, drops, and backlog.
- [ ] Run `make api`, `go test ./...`, `go vet ./...`, localstore race tests, product suite, and performance smoke.
- [ ] Inspect changed files for functions over 50 lines and files over 500 lines; split by responsibility.
- [ ] Run final `git diff --check`, verify only the two pre-existing untracked UI documents remain, and commit with `test(agent): verify standalone local storage`.
