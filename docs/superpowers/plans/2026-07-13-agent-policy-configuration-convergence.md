# Agent Policy And Configuration Convergence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use test-driven-development task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Replace fragmented Agent configuration and policy handling with one runtime config, one durable four-section endpoint policy, and one cloud export pipeline.

**Architecture:** A strict config parser produces local runtime boundaries. `ResolveTelemetry` combines aligned local defaults and optional TelemetryPolicy overrides. One transactional policy service validates, prepares, persists, and applies Collection, Detection, Telemetry, and Response; local and managed calls share it.

**Tech Stack:** Go 1.24, Protobuf, SQLite, JSON, YAML-like strict Agent parser, gRPC Unix socket, systemd.

## Global Constraints

- Fresh deployments only; remove old schema/config/path compatibility.
- Collection determines content; Telemetry never filters or projects content.
- Local disk/resource limits cannot be changed by remote policy.
- Enrollment alone controls cloud identity, endpoint, and credentials.
- Keep `CGO_ENABLED=0` and one ordered export checkpoint.

### Task 1: Unified Policy Schema

- [ ] Add failing model/protobuf tests for four policy sections and optional TelemetryPolicy fields.
- [ ] Remove DataPlanePolicy and replace all API/model usage with TelemetryPolicy.
- [ ] Regenerate protobuf and run policy, Manager, Agent, and ctl tests.
- [ ] Commit `refactor(policy): define unified endpoint policy`.

### Task 2: Runtime Configuration

- [ ] Add failing parser tests for `local`, aligned `telemetry`, `control`, `sensor`, and `policy` sections.
- [ ] Add rejection tests for old `agent.state_path`, `storage`, `data_plane`, and old telemetry field names.
- [ ] Implement the strict fresh-deploy parser and centralized defaults/validation.
- [ ] Commit `refactor(agent): consolidate runtime configuration`.

### Task 3: Effective Telemetry

- [ ] Add failing tests for defaults, config baseline, partial policy override, bounds, and atomic rejection.
- [ ] Implement `ResolveTelemetry` and make Batcher consume only effective values.
- [ ] Prove a policy update changes only future batches.
- [ ] Commit `feat(agent): resolve telemetry policy`.

### Task 4: Durable Unified Policy

- [ ] Add restart tests proving bootstrap policy is persisted and restored.
- [ ] Add atomic update tests for each section and rollback on preparation/persistence failure.
- [ ] Route local RPC and managed control updates through one policy service.
- [ ] Stamp effective policy identity/version into DataBatch.
- [ ] Commit `feat(agent): persist effective endpoint policy`.

### Task 5: Export Pipeline

- [ ] Add tests for no standalone exporter, managed CloudExporter, ACK/checkpoint semantics, and enrollment transitions.
- [ ] Refactor spool uploader into ExportPipeline plus CloudExporter without behavior expansion.
- [ ] Keep managed control channel separate and remove legacy direct transport paths.
- [ ] Commit `refactor(agent): unify cloud export pipeline`.

### Task 6: Filesystem And Installation

- [ ] Add installation/config tests for the final `/etc`, `/var/lib`, `/run`, and `/opt` paths.
- [ ] Package `agent.yaml` and `policy.json`; use systemd RuntimeDirectory.
- [ ] Make installer use `systemctl enable --now` and verify health.
- [ ] Remove legacy policy/config paths and update documentation.
- [ ] Commit `refactor(agent): standardize installation layout`.

### Task 7: Acceptance

- [ ] Run API generation, full tests, vet, localstore/daemon race, product standalone, and performance local-store.
- [ ] Verify fresh install, restart recovery, policy update, enroll, upload boundary, and unenroll.
- [ ] Scan changed functions/files against repository size rules and run `git diff --check`.
- [ ] Commit `test(agent): verify converged policy runtime`.
