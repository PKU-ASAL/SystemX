# SysArmor MVP

SysArmor MVP is a Go prototype of the endpoint-to-manager detection path described in `references/docs/design-mvp.md`.

Current product path:

```text
Tetragon JSONL or replayed SensorEvent JSONL
  -> sysarmor-agent normalize + endpoint fastpath
  -> Link1 upload to sysarmor-manager
  -> manager analytics + store
  -> sysarmorctl JSON queries
  -> test harness assertions
```

## Components

- `cmd/sysarmor-agent`: reads replay JSONL or live Tetragon JSONL, normalizes events, emits endpoint signals, uploads batches.
- `cmd/sysarmor-manager`: exposes Link1 HTTP/gRPC ingest plus query endpoints for agents, events, signals, incidents, metrics, and recompute controls.
- `cmd/sysarmorctl`: stable CLI/query boundary used by tests.
- `api/proto`: source of truth for generated protobuf contracts.
- `internal/endpoint`: normalizer, fastpath rules, raw event ring buffer, upload clients.
- `internal/analytics`: MVP entity/evidence/convergence logic.
- `internal/store`: file-backed MVP store with idempotent upsert for retry tolerance.
- `test`: container and VM e2e topologies.

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

Expected MVP result:

- `apt-fileless-c2`: endpoint terminal, cloud signals, exactly one incident.
- `apt-staged-drop`: no endpoint terminal, cross-lineage cloud stitch, exactly one incident.
- `benign-ci-noise`: no incident in normal convergence mode; additive-threshold control can produce one.

`test/.results/` contains regenerated captures and summary JSON/CSV files and is ignored.

## Useful Debug Commands

Container manager:

```bash
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 status --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario apt-fileless-c2 --layer endpoint --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 incidents --scenario apt-fileless-c2 --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 metrics --json
```

VM manager:

```bash
cd test/env/vm
vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 status --json"
vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 incidents --scenario apt-staged-drop --json"
```

Control recompute checks do not mutate the store:

```bash
sysarmorctl --mgr 127.0.0.1:9443 recompute --scenario apt-staged-drop --disable cloud.cross_lineage --json
sysarmorctl --mgr 127.0.0.1:9443 recompute --scenario benign-ci-noise --mode additive_threshold --json
```

## Notes

- Container e2e runs `sysarmor-manager` in the `mgr` container and streams live Tetragon output through `sysarmor-agent` inside the Tetragon container.
- VM e2e runs `sysarmor-manager`/`sysarmorctl` inside the `mgr` VM and runs the agent stream on `node-a`.
- Link1 HTTP is the default e2e transport; Link1 gRPC is also implemented from generated protobuf service code.
