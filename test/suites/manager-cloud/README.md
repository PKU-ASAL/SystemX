# Manager Cloud Suite

Scope: `manager`

System under test:

- manager data ingest;
- cloud analytics;
- manager store;
- manager HTTP query/control APIs accessed through `sysarmorctl manager ...`.

This suite verifies manager-visible events, cloud signals, incidents, graph/evidence, policy, response, and query behavior.

Migrated case scripts in this suite include manager idempotency, agent health query, and response audit. Remaining legacy manager-cloud cases may still be called through wrappers until they are moved.

Out of scope:

- local agent CPU/RSS benchmark;
- local-only spool watch semantics;
- low-level perf/pprof diagnostics.
