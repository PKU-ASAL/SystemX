# Platform Performance

`performance-platform` measures the platform side of a real `vm-topology` run.
It is separate from endpoint performance:

- endpoint performance: agent/sensor CPU and RSS on `node-a`
- platform performance: manager, gateway, worker, Kafka, Postgres, Redis and OpenSearch resource usage on `mgr`

Run:

```bash
make -C test performance-platform
```

Useful knobs:

```bash
SYSARMOR_PLATFORM_PERF_DURATION=600
SYSARMOR_PLATFORM_PERF_INTERVAL=5
RUN_ID=my-run
```

Outputs are stored under:

```text
test/.results/performance-platform/<run-id>/
  platform.resources.csv
  platform.summary.json
  raw/
    manager.healthz.start.json
    manager.healthz.end.json
    gateway.healthz.start.json
    gateway.healthz.end.json
    manager.metrics.start.json
    manager.metrics.end.json
```

This suite intentionally stores raw data first. Reports can be derived from
these files after the workload or effectiveness matrix is known.
