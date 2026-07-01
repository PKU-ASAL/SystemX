# Endpoint Agent Suite

Scope: `endpoint`.

System under test:

- `sysarmor-agent`;
- owned sensor runtime;
- local agent spool/WAL;
- local `sysarmorctl --socket` APIs;
- endpoint events and endpoint signals.

Use `vm-endpoint` for clean endpoint behavior and resource conclusions.

Entrypoints:

```bash
make -C test test-endpoint
make -C test capture-endpoint SCENARIO=apt-staged-drop
make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=business-normal
make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=business-normal SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=business-normal
```

Endpoint benchmark reports use `startup`, `steady`, `workload`, `activity`,
`persistence`, and `overall` as the standard phase names.

Out of scope:

- manager cloud signals;
- incidents and graph/evidence;
- manager storage/query behavior;
- control-plane downlink semantics.
