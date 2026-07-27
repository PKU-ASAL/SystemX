# Credential Read Low-Noise Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Default-enable credential file read detection with precise system baselines and cross-process-lineage Signal suppression while retaining every Event.

**Architecture:** Signed content owns detection semantics and trusted values; the existing generic condition tree and suppression engine execute them. The benchmark Recorder stops creating privileged self-noise, and regression tests prove sensitive readers still alert.

**Tech Stack:** Go tests, signed JSON content, Bash benchmark harness, Tetragon VM performance suite.

## Global Constraints

- Do not change Event collection or retention.
- Do not add Go rule syntax or Signal schema fields.
- Missing provenance fields remain suspicious.
- Do not include the staged Release experiment report in commits.

---

### Task 1: Lock rule behavior

**Files:**
- Modify: `internal/endpoint/detection/engine_test.go`
- Modify: `internal/agent/content/store_test.go`

**Interfaces:**
- Consumes: existing condition-tree and suppression runtime
- Produces: regression contracts for baseline exclusion and lineage suppression

- [ ] Add failing tests for root baseline readers, non-root readers, dangerous binaries, missing fields, same-lineage suppression, and different-lineage alerts.
- [ ] Run focused tests and verify failures describe the old content semantics.

### Task 2: Update signed content and default policy

**Files:**
- Modify: `deployments/agent/content/rulepack-cep-endpoint.json`
- Create: `deployments/agent/content/context-system-credential-reader-binaries.json`
- Modify: `deployments/agent/policy.json`

**Interfaces:**
- Consumes: `process.binary`, `process.uid`, `lineage_id`, and `file.path`
- Produces: default-enabled low-noise `credential_file_read`

- [ ] Add the context reference and Boolean baseline exclusion.
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

