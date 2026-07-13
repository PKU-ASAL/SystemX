# Agent Policy And Configuration Convergence Design

## Conclusion

SysArmor Agent uses one runtime configuration, one endpoint policy document,
one bounded local state directory, and one telemetry pipeline. This redesign
supports fresh deployments only and removes the legacy `DataPlanePolicy`,
legacy configuration keys, and legacy filesystem paths.

## Filesystem Contract

```text
/etc/sysarmor/agent/agent.yaml   administrator-owned runtime configuration
/etc/sysarmor/agent/policy.json administrator/bootstrap endpoint policy
/var/lib/sysarmor/agent/        mutable SQLite, segments, content, credentials
/run/sysarmor/agent/control.sock local control socket
/opt/sysarmor/agent/            binaries and sensor bundles
```

Agent-generated credentials remain under the state directory. Runtime logs go
to journald; security telemetry never goes to journald.

## Runtime Configuration

```yaml
local:
  state_path: /var/lib/sysarmor/agent
  storage:
    max_bytes: 10GiB
    min_free_bytes: 2GiB
    segment_size: 64MiB
    signal_max_count: 100000
  export:
    retry_initial: 1s
    retry_max: 30s
    request_timeout: 10s
    max_inflight: 1
    wire_compression: none

telemetry:
  max_batch_items: 256
  max_batch_bytes: 256KiB
  flush_interval: 1s

control:
  socket_path: /run/sysarmor/agent/control.sock

sensor:
  backend: tetragon

policy:
  path: /etc/sysarmor/agent/policy.json
```

`max_inflight` accepts only `1` until ordered concurrent checkpoint commits are
implemented. Segment compression remains fixed Zstd and is not configurable.
Cloud endpoint, identity, and credentials come only from Enrollment.

## Unified Endpoint Policy

The policy document has one identity/version and four sections:

```json
{
  "policy_id": "standalone-default",
  "version": 1,
  "collection": {},
  "detection": {},
  "telemetry": {
    "max_batch_items": 256,
    "max_batch_bytes": 262144,
    "flush_interval": "1s"
  },
  "response": {}
}
```

- Collection decides which sensor behavior is observed.
- Detection decides which behavior produces Signals.
- Telemetry preserves data as-is and controls only DataBatch boundaries.
- Response decides which endpoint actions are allowed.

TelemetryPolicy replaces DataPlanePolicy. It never controls behavior filters,
field projection, storage limits, retry, exporter selection, endpoints, TLS,
credentials, or compression.

## Effective Telemetry

Runtime values resolve in this order:

```text
TelemetryPolicy > TelemetryConfig > code defaults
```

Policy fields are optional overrides; config and effective values are concrete.
Config and policy share names, units, range validation, and conversion code.
Batcher reads only EffectiveTelemetry.

## Policy Lifecycle

On first start, Agent validates the bootstrap policy, atomically persists it in
SQLite, and applies all four sections. On later starts, the persisted effective
policy wins. Local and managed updates use the same validation, persistence,
and application path. A failed validation or runtime preparation leaves the old
policy unchanged. Policy ID/version are stamped into new DataBatch headers.

## Export Pipeline

All telemetry follows:

```text
Sensor -> Normalize/Detect -> DataBatch -> Local Store -> optional ExportPipeline
```

Standalone has no active exporter. Enrollment activates CloudExporter plus the
separate managed control channel. The first release has one exporter and one
checkpoint; it does not implement OTLP, multiple exporters, or a plugin registry.

## Acceptance

- Old config keys and DataPlanePolicy are rejected or absent.
- Fresh standalone install starts immediately without network dependency.
- Policy survives restart and is applied atomically.
- Local and managed policy updates share one path.
- Enrollment starts cloud export without restarting the sensor.
- Unenrollment restores standalone without stopping collection.
- Full tests, race tests, vet, fresh product tests, and local-store performance pass.
