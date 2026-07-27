# Credential Read Low-Noise Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Default-enable credential file read detection with precise system baselines and cross-process-lineage Signal suppression while retaining every Event.

**Architecture:** Signed content owns detection semantics and trusted values; the existing generic condition tree and suppression engine execute them. The benchmark Recorder stops creating privileged self-noise, and regression tests prove sensitive readers still alert.

**Tech Stack:** Go tests, signed JSON content, Bash benchmark harness, Tetragon VM performance suite.

## Global Constraints

- Do not change Event collection or retention.
- Do not add Go rule syntax or Signal schema fields.
- Missing binary or argv fields remain suspicious.
- Do not include the staged Release experiment report in commits.

---

### Task 5: Trust the exact sudo sysarmorctl target

**Files:**
- Modify: `api/proto/sensor/v1/sensor.proto`
- Modify: `api/proto/event/v1/event.proto`
- Modify: `internal/sensors/linux/tetragon/adapter.go`
- Modify: `internal/endpoint/normalize/normalize.go`
- Modify: `internal/endpoint/detection/compiled.go`
- Modify: `internal/endpoint/detection/engine.go`
- Modify: `internal/endpoint/detection/engine_test.go`
- Modify: `deployments/agent/content/rulepack-cep-endpoint.json`

**Interfaces:**
- Consumes: canonical `process.binary` and tokenized `process.argv`
- Produces: trusted-boundary process metadata and derived condition field `process.sudo_command`
  containing the basename of sudo's target command

- [ ] Add failing tests for direct, absolute-path, and option-bearing `sudo sysarmorctl`, plus Shell and dangerous-command near misses.
- [ ] Add failing adapter and detection tests for quoted sudo prompt injection, tabs, repeated spaces,
  untrusted boundaries, and sudoedit mode.
- [ ] Run `go test ./internal/endpoint/detection -run TestCredentialReadUsesExactSystemCommandBaselines -count=1` and verify the new trusted cases fail with one Signal.
- [ ] Carry `argv_boundaries_trusted` from the Tetragon adapter through normalization, then implement
  the fail-closed sudo argv parser and expose `process.sudo_command` through both field resolvers.
- [ ] Replace the health-only substring baseline with `process.binary == /usr/bin/sudo` and `process.sudo_command == sysarmorctl`; increment the rule version.
- [ ] Run focused detection tests, `go test ./...`, and Release content/package contracts.
- [ ] Commit only the parser, rule, tests, design, and plan as `fix(detection): trust explicit sysarmorctl sudo commands`.

### Task 6: Split credential secrets from account enumeration

**Files:**
- Modify: `deployments/agent/content/context-credential-path-prefixes.json`
- Create: `deployments/agent/content/context-account-database-path-prefixes.json`
- Create: `deployments/agent/content/context-account-enumeration-binaries.json`
- Modify: `deployments/agent/content/rulepack-cep-endpoint.json`
- Test: `internal/endpoint/detection/engine_test.go`

- [ ] Add failing tests proving shadow/sudoers remain medium, suspicious or unknown passwd readers
  produce low `account_database_read`, and health curl passwd reads produce no Signal.
- [ ] Split signed contexts and add the new rule without retaining a duplicate legacy path.
- [ ] Run detection/content tests and commit as `fix(detection): split account enumeration signals`.

---

### Task 1: Lock rule behavior

**Files:**
- Modify: `internal/endpoint/detection/engine_test.go`
- Modify: `internal/agent/content/store_test.go`

**Interfaces:**
- Consumes: existing condition-tree and suppression runtime
- Produces: regression contracts for baseline exclusion and lineage suppression

- [ ] Add failing tests for exact binary/argv baseline readers, near-miss commands, dangerous binaries, missing fields, same-lineage suppression, and different-lineage alerts.
- [ ] Run focused tests and verify failures describe the old content semantics.

### Task 2: Update signed content and default policy

**Files:**
- Modify: `deployments/agent/content/rulepack-cep-endpoint.json`
- Modify: `deployments/agent/policy.json`

**Interfaces:**
- Consumes: `process.binary`, `process.argv`, `lineage_id`, and `file.path`
- Produces: default-enabled low-noise `credential_file_read`

- [ ] Add the Boolean binary/argv baseline exclusion.
- [ ] Change suppression fields to lineage, binary, and path.
- [ ] Remove the default disable override.
- [ ] Run focused content and detection tests until green.
- [ ] Commit as `fix(detection): reduce credential read noise`.

### Task 3: Remove benchmark observer noise

**Files:**
- Modify: `test/shared/recorder/recorder-vm.sh`
- Modify: `test/suites/performance/endpoint/run.sh`

**Interfaces:**
- Consumes: root-owned Recorder process
- Produces: health snapshots without spawning repeated sudo processes

- [ ] Add a contract rejecting `sudo sysarmorctl` inside the remote Recorder loop.
- [ ] Remove redundant sudo from Recorder-local health calls.
- [ ] Preserve watcher, health snapshot, and error behavior.
- [ ] Commit as `test(performance): remove recorder credential noise`.

### Task 4: Verify regression and performance

**Files:**
- No production changes unless a failing test identifies a scoped defect.

**Interfaces:**
- Consumes: updated default content and Recorder
- Produces: unit, Release, quick, and medium evidence

- [ ] Run `go test ./...` and content/Release contracts.
- [ ] Run quick on a fresh VM and inspect summaries and errors.
- [ ] Run medium with `collection-balanced`, `business-normal`, and `apt-fileless-c2-local` on a fresh VM.
- [ ] Verify credential Signal reduction, dangerous reader coverage, EventRefs resolution, and zero drop/parse/watch errors.
