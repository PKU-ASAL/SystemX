# Test Tools

`test/tools` contains reusable utilities used by one or more suites.

- `benchmarks/`: benchmark runners, perf samplers, and benchmark report builders.
- `assertions/`: reusable expected.yaml and local capture assertion helpers.
- `diagnostics/`: perf/pprof/strace style diagnostics.
- `fixtures/`: synthetic event or scenario fixture generators.
- `recorder/`: long-running VM performance recorder.
- `reports/`: result summarizers and local effectiveness report helpers.
- `vm/`: VM maintenance helpers such as syncing current agent/ctl binaries before benchmarks.

Tools should stay reusable and product-agnostic where possible. Suite-specific
pass/fail semantics belong in `test/suites/<suite>/`; process lifecycle glue
belongs in `test/harness/`.
