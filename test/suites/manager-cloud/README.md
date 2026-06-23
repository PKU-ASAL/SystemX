# Manager Cloud Suite

Scope: `manager`

System under test:

- manager data ingest;
- cloud analytics;
- manager store;
- manager HTTP query/control APIs accessed through `sysarmorctl manager ...`.

This suite verifies manager-visible events, cloud signals, incidents, graph/evidence, policy, response, and query behavior.

Migrated case scripts in this suite include manager idempotency, agent health query, policy control, response control, and incident/graph evidence tests. Scenario container tests may still call legacy container harness scripts until the container runtime suite is moved.

Out of scope:

- local agent CPU/RSS benchmark;
- local-only spool watch semantics;
- low-level perf/pprof diagnostics.
