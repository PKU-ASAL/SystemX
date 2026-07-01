# SysArmor Next Architecture

SysArmor Next is an EDR/XDR platform prototype. The endpoint collects kernel facts, tags them with provenance, produces local security signals, and keeps enough evidence for later investigation. The cloud side connects endpoint, workload, identity, network, and cloud facts into an evidence graph and turns related signals into incidents.

This document describes stable architecture. Version history lives in `changelogs/`; implementation sequencing lives in `roadmap.md`.

## Problem

Modern attacks are rarely a single obviously malicious action. They are chains of ordinary-looking operations:

```text
web runtime spawns shell
  -> downloader writes payload
  -> chmod / exec
  -> reads credential or token
  -> connects outward
  -> pivots through cloud, identity, or workload control planes
```

SysArmor therefore should not be only a log shipper or a collection of single-event rules. It needs:

- trusted endpoint visibility close to the kernel;
- stable event, signal, entity, and incident contracts;
- policy and content that can be versioned and hot-updated;
- response and enforce paths that are authorized and auditable;
- cloud-side graph correlation for evidence, rarity, and incident convergence.

## Principles

| Principle | Design consequence |
|---|---|
| Kernel facts are the most trusted endpoint source | Sensors should collect from eBPF/LSM-capable backends where possible |
| Attacks are causal chains | Incidents should be graph-backed, not just single signal alerts |
| Source-side provenance is cheap; later reconstruction is expensive | Endpoint must tag events with lineage, scope, labels, and entities |
| Endpoint resources compete with business workloads | Endpoint does lightweight CEP and evidence seeding; cloud does global graph analytics |
| Detection content changes faster than runtime code | Rules, context, and IOC content must be versioned and separately distributed |
| Response is dangerous | Enforce requires explicit policy authorization, scope validation, result ack, and audit |
| Deployment topology varies | Host, container, cgroup, namespace, and pod map into one Runtime Scope contract |

## System Layers

```text
Control Plane
  policy, content, rollout, response authorization, tenant and operator boundaries

Cloud Analytics
  entity normalization, graph, rarity, correlation, converge, incident, evidence

Agent-Facing Plane
  data batch append, control plane connection, agent session, ack/resume, downlink policy/response/evidence requests

Endpoint Core
  AgentRuntime, EndpointRuntime, TelemetryBus, TelemetryBatcher, TelemetrySender, TransportRuntime, LocalRuntime

Sensor Runtime
  Tetragon or native sensor capability, collection compiler, subscribe, health, enforce

Kernel / Workload
  process, file, network, container, namespace, pod, host
```

Layering rules:

- Upper layers depend on contracts, not concrete implementations.
- Cloud and manager code should not consume raw sensor JSON directly.
- Collection semantics are behavior-first and sensor-neutral.
- Response cannot bypass policy authorization.
- External export is not a replacement for the native agent data/control protocols.
- The endpoint agent keeps ordinary event/signal telemetry lightweight: EndpointRuntime publishes frames to TelemetryBus for local watch and TelemetryBatcher/TelemetrySender for upload. Durable evidence transport is a separate concern.

## Core Facts

### Event

An Event is an objective fact observed by a sensor and normalized by the endpoint. It should not contain detection semantics.

Examples:

- `process.exec`
- `process.fork`
- `process.exit`
- `file.open`
- `file.read`
- `file.write`
- `file.chmod`
- `network.connect`

Important fields:

- id and sequence;
- tenant, agent, host, scope;
- behavior;
- subject process;
- object entity;
- lineage id;
- labels;
- raw reference;
- observed/occurred timestamp.

### Signal

A Signal is a rule-derived security fact. It explains what an Event or short event window means.

Important fields:

- rule id, rule version, ruleset ref;
- where: endpoint, cloud, or xdr;
- severity, confidence, risk;
- lineage id;
- entity refs;
- event refs;
- context refs and IOC refs;
- optional response intent;
- labels inherited from contributing events.

Signals are building blocks. They do not automatically become user-facing alerts.

### Incident

An Incident is the analyst-facing unit. It is produced when signals and evidence converge into an attack story.

An incident should include:

- terminal anchors;
- contributing signals;
- evidence graph;
- timeline;
- converge trace;
- response history;
- lifecycle state.

## Provenance And Entity Model

Lineage answers: "which execution chain did this fact come from?"

Entity graph answers: "which process, file, socket, IP, identity, container, pod, host, or cloud resource is connected to which other entity?"

Endpoint responsibilities:

- produce stable process, file, socket, and scope identifiers;
- tag events with lineage and labels;
- keep short-window state for local rules;
- emit evidence seeds for cloud graph reconstruction.

Cloud responsibilities:

- build long-lived cross-scope graphs;
- connect endpoint facts with cloud, identity, network, and workload facts;
- maintain rarity/baseline state;
- extract evidence subgraphs and incident timelines.

## Runtime Scope

Runtime Scope describes the protected boundary:

```text
scope:
  type: host | container | cgroup | namespace | pod
  selector: ...
```

Examples:

- VM or bare metal: `host`.
- Single container workload: `container` or `cgroup`.
- Kubernetes workload: `pod` or `namespace`.

Containers should normally be observed by a privileged agent + sensor container that protects selected workload scopes. Installing an agent inside every business container is useful for debugging or restricted environments, but it is not the default product architecture.

## Storage And Infrastructure

Storage responsibilities should be explicit:

- Postgres: platform state, control plane, metadata, policy, response, audit, incident lifecycle, agent sessions and durable cursors.
- Kafka: high-volume durable event stream and async worker handoff.
- Redis: hot state only, such as online status, last seen, pending downlink, rate limits, leases, and hot cursors.
- OpenSearch: searchable events, signals, findings, evidence, timelines, entities, and hunting views.

Raw high-frequency telemetry should not be forced into Postgres as the primary store.

## Agent-Facing Plane

The agent-facing plane is the native Agent-to-cloud boundary. It is split into:

- data plane: AgentDataPlaneService.AppendBatch(DataBatch) for endpoint events, signals, health-adjacent telemetry, evidence seeds, and response results;
- control plane: AgentControlPlaneService.Connect(ControlFrame stream) for policy, content, response commands, evidence requests, health reports, and capability reports.

Together they should provide:

- session establishment;
- batch id, ack cursor, resume, idempotency, and backpressure;
- authentication, authorization, and version negotiation.

The production control plane is a secure bidirectional gRPC stream. The production data plane is the durable DataBatch append path.

## Security Boundaries

Agent identity and manager agent-facing communication should be based on mTLS or equivalent workload identity. Operator actions require tenant-aware authorization and audit.

Every destructive or potentially disruptive action must record:

- actor or policy source;
- tenant, agent, and scope;
- command and mode;
- decision;
- result ack;
- timestamp;
- reason.

## Honest Boundaries

SysArmor should not pretend the endpoint alone can solve global XDR correlation. Endpoint is responsible for high-quality facts, local fast response, and evidence anchors. Cloud is responsible for long-window memory, cross-domain graphing, incident convergence, and analyst workflows.
