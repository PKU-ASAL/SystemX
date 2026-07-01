# Endpoint Agent

This document defines the endpoint-side contract: agent runtime, sensor ownership, local control, event/signal flow, and endpoint response.

## Role

The endpoint agent is the first trusted runtime in the EDR/XDR path. It should:

- manage sensor lifecycle;
- apply collection, detection, response, resource, and data-plane policies;
- normalize sensor events;
- add lineage, scope, labels, and entity refs;
- run lightweight endpoint detection;
- emit local Signals;
- expose local control APIs for `sysarmorctl`;
- batch and send lightweight telemetry;
- validate and execute authorized response commands.

## Sensor Runtime

Sensor Runtime hides the backend. Current backend is Tetragon; a thinner native SysArmor sensor can be added later.

Required contract:

```go
Capability(ctx) -> Capability
Apply(ctx, CollectionIntent) -> Ack
Subscribe(ctx, CollectionIntent) -> EventEnvelope stream
Enforce(ctx, EnforcementCommand) -> EnforcementAck
Health(ctx) -> Health
Stop(ctx) -> error
```

The agent must treat Tetragon as an implementation, not the product model. Product-facing behavior names are SysArmor behaviors such as `file.write` and `network.connect`; adapters map them to backend hooks/selectors.

## Agent Runtime Components

The endpoint agent stays lightweight and single-process, but its code boundaries follow the production data/control split:

```text
AgentRuntime
  SensorRuntime      owns backend lifecycle and collection pushdown
  EndpointRuntime    normalizes events and runs endpoint detection
  TelemetryBus       feeds local watch subscribers without disk persistence
  TelemetryBatcher   batches event/signal frames for upload
  TelemetrySender    sends batches and maintains bounded retry/backoff
  TransportRuntime   maintains data sender and manager control connection
  LocalRuntime       exposes local sysarmorctl side-channel APIs
```

The important rule is that ordinary event/signal telemetry is best-effort and lightweight. Local `sysarmorctl` watch commands observe the in-process telemetry bus; they do not depend on a durable spool or create a second persistence path.

## Agent-Owned Tetragon

For VM and host testing, the agent distribution owns the Tetragon bundle:

- install entry point installs agent and bundled sensor together;
- no separate "install Tetragon first" test path;
- source definition lives under `deployments/sensors/tetragon/`;
- collection policy is compiled into backend policy by the agent.

For containerized deployment, the preferred model is an agent + sensor container with privileged visibility over selected target scopes. The target workload containers are not treated as mini VMs.

## Local Control API

Before manager integration is complete, `sysarmorctl` acts as a local manager.

Transport:

- production local path: gRPC over Unix Domain Socket;
- optional dev/debug path: gRPC over `127.0.0.1` only when explicitly enabled.

The local protocol should match the future manager-to-agent semantics so that local validation is not thrown away.

Core APIs:

- health and capability;
- current policy;
- apply collection/detection/content/response policy;
- watch events;
- get event;
- watch signals;
- response command and ack;
- content list/get/apply.

Watch APIs support a generic filter:

```text
after_sequence
since_observed_at
until_observed_at
labels
```

There is no top-level `scenario` field in endpoint events, signals, or incidents. Test and benchmark dimensions such as workload and scenario are carried as labels, for example `labels["workload"]` and `labels["scenario"]`.

## sysarmorctl Model

`sysarmorctl` should be regular and symmetric:

```text
sysarmorctl agent health
sysarmorctl agent capability
sysarmorctl policy current
sysarmorctl policy apply collection --file ...
sysarmorctl content apply --file ...
sysarmorctl event watch --label key=value --after-seq N
sysarmorctl event get --event-id ...
sysarmorctl signal watch --include-events --label key=value
sysarmorctl manager policies assign --agent agent-a --policy-id balanced --version 3 --downlink
sysarmorctl manager control-commands create content --agent agent-a --file ioc.json
sysarmorctl manager control-commands list --agent agent-a
sysarmorctl manager control-commands cancel --command-id ctrl-a --agent agent-a
sysarmorctl manager roles upsert --actor alice --roles policy_admin,control_admin
```

Local agent operations stay at the top level and use the Unix socket. Manager administration is explicit under `manager`; desired-state policy changes are separate from auditable downlink commands. Resource names should describe product concepts, not test harness concepts.

## Event Pipeline

```text
SensorRuntime
  -> EventEnvelope
  -> EndpointRuntime
  -> CanonicalEvent + endpoint Signal
  -> DataBatch
  -> TelemetryBus + TelemetryBatcher
  -> TelemetrySender
  -> AgentDataPlaneService.AppendBatch
```

CanonicalEvent carries:

- behavior-first event type;
- lineage;
- subject and object entities;
- scope;
- labels;
- raw ref;
- timestamp.

Labels are generic context:

- `sensor_runtime`;
- `scope_type`;
- `scope_selector`;
- `policy_id`;
- `policy_version`;
- `policy_mode`;
- deployment or environment labels;
- benchmark labels when running tests.

## Detection Runtime

The endpoint detection runtime should remain lightweight:

- builtin rules for high-confidence local patterns;
- expression rules for single-event predicates;
- short sequence rules for small windows;
- small per-lineage state;
- terminal anchors and evidence seeds;
- no global graph reconstruction.

Global provenance graph reconstruction belongs to cloud analytics. Endpoint must produce enough identifiers and references for cloud reconstruction.

Policy and content hot updates are applied as a candidate runtime first. The agent only switches to the new detection engine after the candidate builds successfully. Missing collection dependencies may produce a `degraded` status while still applying; invalid rule/runtime content is `rejected` and the previous effective engine remains active. Agent health exposes the active detection policy/content refs and the last apply status.

## Response / Enforce

Endpoint response must be policy-gated.

Default mode is observe-only:

```text
Signal response intent
  -> response policy decision
  -> response command
  -> agent validates scope and mode
  -> sensor Enforce or observe-only ack
  -> audit/result append
```

The agent may support:

- collect evidence;
- kill process;
- block executable or path;
- quarantine file;
- network block;
- enhanced collection window.

Destructive actions require explicit authorization and must be auditable.

## Resource And DataPlane Policy

Endpoint runtime must be tunable:

- collection budget;
- event rate;
- local ring sizes;
- telemetry bus and batcher sizes;
- data batch size;
- retry and backoff;
- CPU/memory guardrails;
- deep collection windows.

Default policy should keep normal business workload overhead low and enable deeper collection only for risk or investigation windows.

## Current Gaps

Known gaps to keep visible:

- real enforce is still limited and should remain observe-only until policy, audit, and backend support are complete;
- Tetragon policy apply can cause short CPU spikes and needs lifecycle-aware benchmarking;
- live Tetragon policy replacement can still produce timing-sensitive visibility gaps and should be validated with the slim VM matrix after collection-policy changes;
- `balanced` must keep high-frequency hooks selective. Shell/interpreter matching belongs in lower-frequency `network.connect` selectors or short-lived deep windows, not broad always-on `process.exec`;
- native thin sensor path is not implemented yet;
- richer detection rule validation and signature/rotation workflows should continue to mature.
