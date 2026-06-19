# SysArmor Next

SysArmor Next is a Go prototype for an EDR/XDR platform path described in `references/docs/architecture.md`.

Current product path:

```text
agent-owned sensor runtime
  -> sysarmor-agent normalize + endpoint detection
  -> local sysarmorctl control/watch during endpoint refinement
  -> Agent Gateway / manager / workers as the platform path matures
  -> incident, evidence, response, and benchmark workflows
```

## Components

- `cmd/sysarmor-agent`: owns endpoint runtime, sensor lifecycle, normalization, local detection, local control, and upload.
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
- Older e2e paths still exercise manager upload/query flows; endpoint refinement should prefer the local agent control path documented in `references/docs/endpoint-agent.md`.
