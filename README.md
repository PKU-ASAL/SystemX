# SysArmor Next

SysArmor Next is a Go prototype for an EDR/XDR platform path described in `references/docs/architecture.md`.

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
- `cmd/sysarmorctl`: CLI/control boundary used by local agent workflows and tests.
- `api/proto`: source of truth for generated protobuf contracts.
- `internal/endpoint`: normalizer, detection engine, ring buffers, upload clients.
- `internal/analytics`: entity, evidence, correlation, convergence, and incident logic.
- `internal/store`: platform state store prototypes and Postgres foundations.
- `test`: container and VM e2e topologies.

Core docs:

- `references/docs/architecture.md`
- `references/docs/endpoint-agent.md`
- `references/docs/policy-content.md`
- `references/docs/testing-benchmark.md`
- `references/docs/roadmap.md`

## Build And Test

```bash
make api
make build
make test
```

`make api` requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` on `PATH` or under `$(go env GOPATH)/bin`.

The build writes static binaries to `bin/`, which is ignored because it is regenerated.

## E2E

Run from `test/`:

```bash
make e2e TOPO=container SCENARIO=apt-fileless-c2 DUR=12
make e2e TOPO=container SCENARIO=apt-staged-drop DUR=12
make e2e TOPO=container SCENARIO=benign-ci-noise DUR=12

make e2e TOPO=vm SCENARIO=apt-fileless-c2 DUR=12
make e2e TOPO=vm SCENARIO=apt-staged-drop DUR=12
make e2e TOPO=vm SCENARIO=benign-ci-noise DUR=12

make report
```

Expected result:

- `apt-fileless-c2`: endpoint terminal, cloud signals, exactly one incident.
- `apt-staged-drop`: no endpoint terminal, cross-lineage cloud stitch, exactly one incident.
- `benign-ci-noise`: no incident in normal convergence mode; additive-threshold control can produce one.

`test/.results/` contains regenerated captures and summary JSON/CSV files and is ignored.

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

`sysarmorctl --agent-sock ...` is a local operator/debug boundary. It talks to the local agent over Unix socket gRPC and reads the local spool/WAL as a side channel for watch/query commands. Cloud or manager communication must use `AgentDataPlaneService.AppendBatch` and `AgentControlPlaneService.Connect`; local ctl is not a second production data plane.

See [references/docs/agent-manager-contract.md](references/docs/agent-manager-contract.md) for the table-form contract.

## Useful Debug Commands

Container manager:

```bash
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 status --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 signals --label scenario=apt-fileless-c2 --layer endpoint --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 incidents --label scenario=apt-fileless-c2 --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 metrics --json
```

VM manager:

```bash
cd test/environments/vm
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
sysarmorctl --agent-sock /var/run/sysarmor/agent.sock --json policy explain collection --file test/data/policies/collection-edr-balanced.json
sysarmorctl --agent-sock /var/run/sysarmor/agent.sock --json policy explain collection --file test/data/policies/collection-edr-balanced.json --report-only
sysarmorctl --agent-sock /var/run/sysarmor/agent.sock --json agent health
```

`policy explain collection` performs a dry-run compile: it resolves content refs, reports backend mappings, pushdown/agent-side selectors, unsupported selectors, and detection coverage gaps without applying the policy. `agent health` includes spool/WAL backlog, cursor, watcher, backpressure, and upload drain status.

## Notes

- Container e2e runs `sysarmor-manager` in the `mgr` container and streams live Tetragon output through `sysarmor-agent` inside the Tetragon container.
- VM e2e runs `sysarmor-manager`/`sysarmorctl` inside the `mgr` VM and runs the agent stream on `node-a`.
- Older e2e paths still exercise manager upload/query flows; endpoint refinement should prefer the local agent control path documented in `references/docs/endpoint-agent.md`.
