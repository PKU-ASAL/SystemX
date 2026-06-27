# Module Benchmarks

This suite contains local module-level performance checks that do not require a
container or VM topology.

Use this area for focused implementation benchmarks, such as endpoint rule
engine processing, matcher algorithms, parsers, or store projection
microbenchmarks.

Matrix-style product benchmarks remain under the local-agent and benchmark tool
paths:

- `test/e2e/local-agent/bench-matrix-vm.sh`
- `test/benchmarks/matrix/collection-vm.sh`
- `test/benchmarks/matrix/matrix-vm.sh`

Run focused local checks:

```bash
make -C test test-rule-engine
make -C test test-rule-engine-effectiveness
make -C test bench-rule-engine
make -C test test-matcher
make -C test bench-matcher
BENCHTIME=1s COUNT=3 make -C test bench-rule-engine
```

Results are written to:

```text
test/.results/rule-engine/<run-id>/
```
