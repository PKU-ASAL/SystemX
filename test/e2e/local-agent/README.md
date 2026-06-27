# Local Agent Suite

Scope: `local`

System under test:

- `sysarmor-agent`;
- sensor runtime;
- local agent spool/WAL;
- local `sysarmorctl --socket` APIs.

This suite compares endpoint collection and endpoint detection behavior under different policies and workloads. It also measures local cost: agent/sensor CPU, RSS, EPS, drops, and parse errors.

Effectiveness reports are generated with `evaluation_scope=local` from `labels.yaml` ground truth. They compare workload-window local events and endpoint signals against labels and report event/signal precision-recall. Manager cloud signals, incidents, and graph/evidence remain out of scope for this suite.

Entrypoints:

```bash
make -C test test-local-agent
make -C test bench-local-agent
make -C test capture TOPO=vm SCENARIO=apt-staged-drop
make -C test e2e-agent-real-tetragon-owned-container
make -C test e2e-agent-real-tetragon-owned-vm
```

`bench-local-agent` / `bench-matrix-vm` sync the current VM `sysarmor-agent` and `sysarmorctl` by default, then apply endpoint detection content before running the collection policy matrix. Set `SYSARMOR_BENCH_SYNC_VM_AGENT=0` only when intentionally testing the VM's installed version.

Out of scope:

- manager cloud signals;
- incidents and graph/evidence;
- manager storage/query behavior;
- control-plane downlink semantics.
