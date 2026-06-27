# Module Benchmarks

This suite contains local module-level performance checks that do not require a
container or VM topology.

Use this suite for focused implementation benchmarks, such as endpoint rule
engine matching, matcher algorithms, parsers, or store projection microbenchmarks.

Matrix-style product benchmarks remain under the local-agent and benchmark tool
paths:

- `test/suites/local-agent/bench-matrix-vm.sh`
- `test/tools/benchmarks/bench-collection-vm.sh`
- `test/tools/benchmarks/bench-matrix-vm.sh`

Run the endpoint rule-engine benchmark:

```bash
make -C test bench-rule-engine
BENCHTIME=1s COUNT=3 make -C test bench-rule-engine
```

Results are written to:

```text
test/.results/rule-engine/<run-id>/
```
