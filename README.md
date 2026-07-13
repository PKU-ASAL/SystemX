# SysArmor Next

SysArmor Next is a Go prototype for an EDR/XDR platform. The current repository
keeps product source, deployment assets, tests, and future packaging/UI work in
separate top-level areas.

Current product path:

```text
agent-owned sensor runtime
  -> sysarmor-agent normalize + endpoint detection
  -> local sysarmorctl control/watch during endpoint refinement
  -> AgentDataPlaneService.AppendBatch + AgentControlPlaneService.Connect
  -> incident, evidence, response, and benchmark workflows
```

## Components

- `cmd/sysarmor-agent`: owns endpoint runtime, sensor lifecycle, normalization, local detection, local control, and data append.
- `cmd/sysarmor-manager`: platform control/query prototype for agents, policies, responses, incidents, evidence, and metrics.
- `cmd/sysarmor-gateway`: agent-facing access layer for data/control plane traffic.
- `cmd/sysarmor-worker`: platform analytics and indexing worker.
- `cmd/sysarmorctl`: CLI/control boundary used by local agent workflows and tests.
- `api/proto`: source of truth for generated protobuf contracts.
- `internal/endpoint`: normalizer, detection engine, ring buffers, upload clients.
- `internal/sensors`: sensor contracts and platform-specific sensor adapters.
- `internal/manager/api`: operator-facing manager HTTP API.
- `internal/analytics`: entity, evidence, correlation, convergence, and incident logic.
- `internal/store`: platform state store prototypes and Postgres foundations.
- `packages`: product distribution package definitions for agent/sensor bundles.
- `deployments`: compose, Docker, systemd, PKI, and sensor deployment assets.
- `web`: reserved for future operator-facing UI projects.
- `test`: unit, endpoint, topology, and platform test scopes.

Core docs:

- `docs/architecture/repo-layout.md`
- `test/README.md`
- `test/DETAILS.md`

## Build And Test

```bash
make api
make build-binary
make release
make test
```

`make api` requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` on `PATH` or under `$(go env GOPATH)/bin`.

`make build-binary` writes static binaries to `dist/bin/`. `make release`
writes the signed agent package and package index to `dist/release/`. Both
directories are ignored because they are regenerated.

## Test Suites

Run from `test/`:

```bash
make test-unit
make product-endpoint
make product-topology SCENARIO=apt-fileless-c2
make product-platform

make performance-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=business-normal
make performance-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=business-normal SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
make performance-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=business-normal
make effectiveness-topology ENV=vm-topology
```

Environment choices:

- `container`: lightweight manager/platform checks.
- `vm-endpoint`: one fresh endpoint VM per benchmark run; source of truth for agent/sensor CPU and memory conclusions.
- `vm-topology`: three VMs (`mgr`, `node-a`, `attacker`) for manager-agent-C2 product path checks.

Performance/effectiveness reports use these standard phases: `startup`, `steady`,
`workload`, `activity`, `persistence`, and `overall`.

`test/.results/` contains regenerated captures and summary JSON/CSV files and is ignored. `vm-topology` deployment input cache lives under `test/environments/vm-topology/deploy/`, split into a lightweight platform bundle and a reusable Docker image bundle.

## Data And Control Plane Contract

Production agent-to-manager traffic is split into two gRPC services:

- `AgentDataPlaneService.AppendBatch(DataBatch)`: agent to manager data flow. Events and signals are appended as durable `DataBatch` units from the agent spool/WAL. A `DataAck` commits the batch cursor only when its status is `STATUS_ACCEPTED` or `STATUS_DUPLICATE`.
- `AgentControlPlaneService.Connect`: bidirectional control flow. Agent frames carry health, capability, response acks, and evidence results. Server frames carry policy updates, resume cursors, response commands, evidence pullbacks, and structured rejected acks.

The contract envelope is intentionally explicit. `Connect` uses `contract_version=1`, a required `request_id`, and per-stream sequence numbers starting at `1`; replayed frames are rejected as `AlreadyExists`, and sequence gaps are rejected as `FailedPrecondition`. Reusing the same `request_id` with a new valid sequence is idempotent and replays the prior response without re-running the command. `DataAck` uses stable status and reason classes: invalid payloads are terminal `invalid_data_batch` rejections, while transient server/storage/capacity failures are `STATUS_RETRYABLE` with `retry_after_ms`.

Both services share the same production mTLS identity model. The preferred agent certificate identity is:

```text
spiffe://sysarmor.local/tenant/<tenant_id>/agent/<agent_id>
```

The manager checks the certificate identity against `DataBatch.header.tenant_id/agent_id` and `ControlFrame.context.tenant_id/agent_id`, then binds the certificate principal into the agent registry. A later connection for the same tenant/agent with a different certificate principal is rejected.

`sysarmorctl --socket ...` is the local operator/debug boundary. It talks to the
Agent over Unix socket gRPC; the Agent alone reads its SQLite state and segment
spool. `sysarmorctl enroll` passes a one-time Manager token to that local RPC,
but never handles the generated private key. Cloud communication starts only
after enrollment and uses `AgentDataPlaneService.AppendBatch` plus
`AgentControlPlaneService.Connect`; local ctl is not a second production data
plane.

The API/protobuf contracts under `api/proto/` are the source of truth for this
boundary.

## Manager Authentication

The browser signs in to the Manager UI with the single deployment bootstrap
admin. Auth.js keeps an encrypted HttpOnly cookie session, and the server-side
Next.js BFF signs a five-minute `RS256` JWT for each Manager request. The
browser never receives that JWT or the Manager internal address.

Manager trusts only the BFF public key and converts verified `sub`, `tenant_id`,
and `roles` claims into a request-scoped Principal. Caller-provided identity
headers are never trusted. Run `make auth-init` once to create local secrets;
no user or session tables are created.

## Analytics Persistence

Production Worker analysis is stateless across Kafka messages. For each scope it
loads a tenant-bound 15-minute Event and endpoint Signal window from OpenSearch,
merges the current batch by deterministic ID, and submits one Bulk projection.
Kafka offsets are committed only after every Bulk item succeeds. Transient
failures are retried; permanent payload or projection errors are committed only
after a dead-letter message is durably published.

PostgreSQL owns control-plane state and uses ordered transactional migrations.
Legacy Incident tables are never dropped automatically; the operator-run cleanup
SQL is `internal/store/migrations/legacy_incident_cleanup.sql`.

## Useful Debug Commands

Container manager:

```bash
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 status --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 signals --label scenario=apt-fileless-c2 --layer endpoint --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 incidents --label scenario=apt-fileless-c2 --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 metrics --json
```

VM topology manager:

```bash
cd test/environments/vm-topology
vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 status --json"
vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 incidents --label scenario=apt-staged-drop --json"
```

Control recompute checks do not mutate the store:

```bash
sysarmorctl --mgr 127.0.0.1:9443 recompute --label scenario=apt-staged-drop --disable cloud.cross_lineage --json
sysarmorctl --mgr 127.0.0.1:9443 recompute --label scenario=benign-ci-noise --mode additive_threshold --json
```

Endpoint policy explain and WAL health:

```bash
sysarmorctl --socket /run/sysarmor/agent/control.sock --json policy explain collection --file test/data/policies/collection-balanced.json
sysarmorctl --socket /run/sysarmor/agent/control.sock --json policy explain collection --file test/data/policies/collection-balanced.json --report-only
sysarmorctl --socket /run/sysarmor/agent/control.sock --json agent health
```

`policy explain collection` performs a dry-run compile: it resolves content refs, reports backend mappings, pushdown/agent-side selectors, unsupported selectors, and detection coverage gaps without applying the policy. `agent health` includes spool/WAL backlog, cursor, watcher, backpressure, and upload drain status.

## Notes

- Container e2e runs `sysarmor-manager` in the `mgr` container and streams live Tetragon output through `sysarmor-agent` inside the Tetragon container.
- `vm-endpoint` runs only `node-a` and is the preferred environment for endpoint refinement and resource profiling.
- `vm-topology` runs `sysarmor-manager`/`sysarmorctl` inside the `mgr` VM and the agent on `node-a`.
