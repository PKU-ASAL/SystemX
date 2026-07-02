# SysArmor Test Items

Run targets from the repository root with:

```bash
make -C test <target>
```

## Environment Lifecycle

| Target | Purpose |
|---|---|
| `up ENV=container` | Start the container environment. |
| `up ENV=vm-endpoint` | Start the single endpoint VM. |
| `up ENV=vm-topology` | Start the three-node VM topology. |
| `down ENV=...` | Stop the selected environment. |
| `status ENV=...` | Show selected environment status. |
| `provision ENV=vm-endpoint|vm-topology` | Rsync and provision `node-a`. |
| `clean ENV=...` | Stop an environment and remove generated results. |

## Scope Suites

| Target | Scope | Purpose |
|---|---|---|
| `test-unit` | unit | Run local Go tests. |
| `test-endpoint` | endpoint | Run single-VM endpoint-owned sensor smoke. |
| `test-topology` | topology | Run multi-VM manager/agent/scenario smoke. |
| `test-platform` | platform | Run local-process gateway, manager, storage, policy, response, and control contracts. |
| `test-platform-full` | platform | Run container gateway/worker/manager product path with Kafka and storage dependencies. |
| `test-all` | all | Run all scope suites. |

## Capture

| Target | Purpose |
|---|---|
| `capture-endpoint SCENARIO=...` | Capture one scenario on `vm-endpoint`. |
| `capture-topology SCENARIO=...` | Capture one scenario on `vm-topology`. |

## Benchmark And Diagnostics

| Target | Purpose |
|---|---|
| `bench-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=...` | Short fresh single-VM endpoint benchmark smoke. |
| `bench-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=... SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local` | Medium fresh endpoint detection and performance correlation benchmark. |
| `bench-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=...` | Long fresh single-VM endpoint CPU/RSS benchmark. |
| `bench-topology` | Multi-VM workload/scenario matrix. |
| `bench-module` | Run module microbenchmarks. |
| `bench-rule-engine` | Run endpoint detection engine microbenchmarks. |
| `bench-matcher` | Run matcher microbenchmarks. |
| `diag-endpoint SYSARMOR_BENCH_WORKLOAD=...` | Capture endpoint diagnostics while running a workload. |
| `recorder-start RUN_ID=...` | Start the endpoint VM timeline recorder. |
| `recorder-mark RUN_ID=... PHASE=... DETAIL=...` | Add a recorder marker. |
| `recorder-stop RUN_ID=...` | Stop recorder and pull timeline artifacts. |
| `recorder-report RUN_ID=...` | Build recorder summary. |
| `effectiveness-report RUN_ID=...` | Generate effectiveness metrics from benchmark outputs. |

Endpoint benchmark reports use standard phases: `startup`, `steady`,
`workload`, `activity`, `persistence`, and `overall`.

## Data Sets

| Name | Kind |
|---|---|
| `apt-fileless-c2` | malicious scenario |
| `apt-fileless-c2-local` | endpoint-local malicious scenario |
| `apt-staged-drop` | malicious scenario |
| `benign-ci-noise` | benign scenario |
| `business-normal` | endpoint workload |
| `host-activity-heavy` | endpoint workload |
| `edr-activity-heavy` | endpoint workload |
