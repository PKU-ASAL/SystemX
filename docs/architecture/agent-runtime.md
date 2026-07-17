# Agent Runtime

The Agent is useful before it is enrolled. Installation starts one local
runtime that owns collection, detection, persistence, and its Unix-socket API.

## Runtime Modes

In **standalone** mode the Agent creates a local device identity, runs the
managed sensor, stores bounded telemetry, and emits endpoint signals. It makes
no Manager or Gateway connection.

Enrollment atomically changes the Agent to **managed** mode. The Agent keeps
the same local collection path and adds authenticated upload and control
channels. Unenrollment removes cloud credentials and returns to standalone
without stopping local collection.

## Filesystem Contract

```text
/opt/sysarmor/agent/                 binaries, sensor bundle, and runtime assets
/etc/sysarmor/agent/agent.yaml      runtime configuration
/etc/sysarmor/agent/policy.json     unified endpoint policy
/var/lib/sysarmor/agent/            identity, enrollment, SQLite state, segments
/run/sysarmor/agent/control.sock    local operator API
```

`sysarmorctl` uses the control socket. It does not read SQLite or event
segments directly.

## Configuration And Policy

Runtime configuration owns local storage, export retry limits, the control
socket, sensor lifecycle, health cadence, and the policy path. The endpoint
policy owns collection, detection, telemetry, and response intent. There is no
second startup collection policy.

Policy updates are compiled before they replace the effective policy. The
effective policy is persisted so restart does not silently return to the
packaged default.

## Local Persistence

SQLite stores device identity, enrollment state, effective policy, signals,
segment metadata, and upload checkpoints. High-volume events use bounded
append-only segments. Capacity limits and minimum free-space checks prevent
unbounded disk growth.

The packaged limits are 10 GiB total local state, 2 GiB minimum filesystem
free space, 64 MiB segments, and 100,000 retained signals. Capacity enforcement
prunes excess signals, then removes the oldest sealed segments that are already
uploaded before pending segments. If pressure requires dropping unuploaded
batches, the Agent increments its storage-drop counter; the loss is never
reported as a successful upload.

On startup, SQLite metadata is reconciled with segment files. An incomplete
tail in the active `.open` segment is truncated to the last complete record.
Invalid headers, invalid records, corruption in sealed segments, or multiple
open segments fail startup instead of silently discarding data. An append or
metadata write is successful only when local persistence succeeds.

Upload checkpoints advance only after an accepted or duplicate data-plane
ack. By default enrollment uploads data created after the enrollment boundary;
history upload must be requested explicitly.

## Local Control Boundary

The Unix socket exposes health, policy explain/apply, event and signal watch,
content operations, and enrollment lifecycle commands. Queries are bounded by
the local store and in-memory stream windows. The socket is the supported local
operator boundary; SQLite tables and segment files are internal formats.

## Identity

The private key is generated on the endpoint. Enrollment submits a CSR and
stores the issued certificate under the Agent state directory. The certificate
URI identity is:

```text
spiffe://sysarmor.local/tenant/<tenant_id>/agent/<agent_id>
```

Gateway verifies that identity against every reported tenant and Agent ID.

Operational commands are documented in [Deployments](../../deployments/README.md).
