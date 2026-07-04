# Module Benchmarks

This suite contains local module-level performance checks that do not require a
container or VM topology.

Use this area for focused implementation benchmarks, such as endpoint rule
engine processing, matcher algorithms, parsers, or store projection
microbenchmarks.

Matrix-style product benchmarks remain under the endpoint/topology benchmark
entrypoints:

- `make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=business-normal`
- `make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=business-normal SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'`
- `make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=business-normal`
- `make -C test bench-topology`

Run focused local checks:

```bash
make -C test bench-rule-engine
make -C test bench-matcher
BENCHTIME=1s COUNT=3 make -C test bench-rule-engine
```

Results are written to:

```text
test/.results/rule-engine/<run-id>/
```
