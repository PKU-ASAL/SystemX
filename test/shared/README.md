# Shared Test Utilities

`test/shared` contains reusable utilities used by one or more suites.

- `assertions/`: reusable expected.yaml and local capture assertion helpers.
- `diagnostics/`: perf/pprof/strace style diagnostics.
- `fixtures/`: synthetic event or scenario fixture generators.
- `harness/`: topology lifecycle and generic shell helpers.
- `recorder/`: long-running VM performance recorder.
- `reports/`: result summarizers and local effectiveness report helpers.
- `vm/`: VM maintenance helpers such as syncing current agent/ctl binaries before benchmarks.

Tools should stay reusable and product-agnostic where possible. Suite-specific
pass/fail semantics belong in `test/e2e/<suite>/`; process lifecycle glue
belongs in `test/shared/harness/`. Benchmark runners live under
`test/benchmarks/`.

Rule-engine and matcher checks are local Go tests/benchmarks and do not require
a test topology:

```bash
make -C test test-rule-engine
make -C test test-rule-engine-effectiveness
make -C test bench-rule-engine
make -C test test-matcher
make -C test bench-matcher
BENCHTIME=1s COUNT=3 make -C test bench-rule-engine
```

Outputs are written to `test/.results/rule-engine/<run_id>/`.
