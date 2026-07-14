# Agent Test Suite Convergence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use test-driven-development task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Preserve every legacy Agent test assertion while converging source tests on the current standalone-first runtime, unified EndpointPolicy, enrollment flow, and filesystem layout.

**Architecture:** Extend the existing shell harness with four Agent-specific libraries, keep fixtures in the current schema, and map every old assertion to a retained scenario before deleting duplicate scripts. Public test entry points remain in `test/Makefile`.

**Tech Stack:** Bash, Go 1.24 tests, JSON, strict Agent YAML, `sysarmorctl`, Docker, Vagrant, systemd.

## Global Constraints

- Fresh deployments only; do not add production or test compatibility for old config keys.
- Standalone tests start locally; cloud, topology, and systemd tests only execute the Manager-provided `install_url`.
- Tests use current four-section EndpointPolicy documents.
- Tests query Agent state through `sysarmorctl`, not SQLite.
- Preserve assertion coverage before deleting or merging scripts.
- Do not edit `test/environments/vm-topology/deploy/platform/` directly.

---

### Task 1: Coverage Inventory And Legacy Contract

**Files:**
- Create: `test/contracts/agent-test-coverage.tsv`
- Create: `internal/contracts/schema/agent_test_assets_test.go`

**Interfaces:**
- Produces: a row for each legacy source script with columns `source`, `capability`, `assertion`, `replacement`, and `status`.
- Produces: `TestAgentTestAssetsUseCurrentSchema`, which scans source assets while excluding the generated VM deployment mirror.

- [ ] **Step 1: Write the failing repository contract test**

The test resolves the repository root and rejects these source-test patterns:

```go
var legacyAgentTestPatterns = []string{
	`/etc/sysarmor/agent.yaml`,
	`/etc/sysarmor/policies`,
	`/run/sysarmor/agent.sock`,
	"\ndata_plane:",
	"\nmanager:",
	"\n  batch_size:",
	"\n  policy_path:",
}
```

It also parses `agent-test-coverage.tsv`, requires non-empty columns, unique
`source + assertion` pairs, and permits deletion only for rows whose status is
`covered`.

- [ ] **Step 2: Run the contract test and verify RED**

Run:

```bash
go test ./internal/contracts/schema -run TestAgentTestAssetsUseCurrentSchema -count=1
```

Expected: FAIL listing the current legacy source scripts.

- [ ] **Step 3: Add the initial inventory**

Inventory every assertion from the 30 reported source assets. Use one row per
assertion, not one row per file. Initial rows use `legacy` status and name the
planned retained runner, including:

```text
test/suites/product/endpoint/e2e-capability.sh	lifecycle	unsupported backend fails	test/suites/product/endpoint/capability.sh	legacy
test/suites/product/endpoint/e2e-sensor-restart.sh	lifecycle	sensor restarts once	test/suites/product/endpoint/sensor-lifecycle.sh	legacy
test/suites/product/topology/e2e-scenario-apt-container.sh	cloud	apt produces incident	test/suites/product/topology/scenario-container.sh	legacy
```

- [ ] **Step 4: Commit the RED contract and inventory**

```bash
git add test/contracts/agent-test-coverage.tsv internal/contracts/schema/agent_test_assets_test.go
git commit -m "test(agent): inventory legacy scenario coverage"
```

### Task 2: Current Fixtures And Agent Harness

**Files:**
- Create: `test/fixtures/agent/configs/fake.yaml`
- Create: `test/fixtures/agent/configs/tetragon.yaml`
- Create: `test/fixtures/agent/policies/default.json`
- Create: `test/shared/agent/install.sh`
- Create: `test/shared/agent/enroll.sh`
- Create: `test/shared/agent/policy.sh`
- Create: `test/shared/agent/inspect.sh`
- Create: `test/shared/agent/harness_test.sh`

**Interfaces:**
- Produces: `sa_agent_write_config OUTPUT STATE SOCKET POLICY SENSOR_BLOCK`.
- Produces: `sa_agent_start_process BINARY CONFIG LOG PID_REF` and `sa_agent_wait_ready CTL SOCKET OUTPUT`.
- Produces: `sa_agent_enroll CTL SOCKET MANAGER TOKEN TENANT AGENT GATEWAY SNI` and `sa_agent_unenroll CTL SOCKET`.
- Produces: `sa_agent_validate_policy FILE`, `sa_agent_apply_policy CTL SOCKET TYPE FILE`, and bounded inspection helpers.

- [ ] **Step 1: Write failing shell contract tests**

`harness_test.sh` uses a temporary directory and fake `sysarmorctl` to verify:

```text
config contains local/control/sensor/policy
config omits manager identity and data_plane
policy validator accepts four sections
policy validator rejects collection-only JSON
enroll passes token through argv without printing it
readiness timeout returns non-zero and prints bounded diagnostics
```

- [ ] **Step 2: Run helper tests and verify RED**

```bash
bash test/shared/agent/harness_test.sh
```

Expected: FAIL because the helper files do not exist.

- [ ] **Step 3: Implement the four narrow helpers**

Source `test/shared/harness/lib/common.sh` for generic waits and cleanup. Keep
all Agent defaults in the two runtime fixtures and the complete policy fixture;
do not accept legacy input or translate field names.

- [ ] **Step 4: Run helper tests and parser tests**

```bash
bash test/shared/agent/harness_test.sh
go test ./internal/agent/config ./internal/agent/policy -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fixtures/agent test/shared/agent
git commit -m "test(agent): add current-schema test harness"
```

### Task 3: Local Product Scenarios

**Files:**
- Create: `test/suites/product/endpoint/capability.sh`
- Create: `test/suites/product/endpoint/sensor-lifecycle.sh`
- Modify: `test/suites/product/endpoint/e2e-daemon.sh`
- Modify: `test/suites/product/endpoint/e2e-parse-health.sh`
- Modify: `test/suites/product/endpoint/e2e-dropped-health.sh`
- Modify: `test/suites/product/endpoint/e2e-daemon-container.sh`
- Delete after coverage: `e2e-capability.sh`, `e2e-capability-btf.sh`, `e2e-capability-bpffs.sh`, `e2e-sensor-restart.sh`, `e2e-sensor-recover.sh`
- Modify: `test/contracts/agent-test-coverage.tsv`

**Interfaces:**
- `capability.sh CASE` accepts `backend`, `btf`, or `bpffs` and retains each old exit-code and health assertion.
- `sensor-lifecycle.sh CASE` accepts `restart` or `recover` and retains process-count, health-transition, and event assertions.

- [ ] **Step 1: Add new runners while old scripts remain**

Use the shared fixtures and helpers. Replace direct Manager transport with local
socket assertions. Preserve all old result artifact names until their report
consumers are migrated.

- [ ] **Step 2: Run new and old scenarios side by side**

```bash
bash test/suites/product/endpoint/capability.sh backend
bash test/suites/product/endpoint/capability.sh btf
bash test/suites/product/endpoint/capability.sh bpffs
bash test/suites/product/endpoint/sensor-lifecycle.sh restart
bash test/suites/product/endpoint/sensor-lifecycle.sh recover
```

Expected: every assertion represented by the five old scripts passes.

- [ ] **Step 3: Migrate daemon and health scripts**

Remove legacy config blocks and duplicate wait functions. Use isolated temp
state and socket paths, complete EndpointPolicy fixtures, and the shared
inspection helpers.

- [ ] **Step 4: Mark inventory rows covered and delete duplicate scripts**

The inventory replacement paths must exist and every deleted assertion row must
be `covered` before deletion.

- [ ] **Step 5: Run product-local and contract tests**

```bash
go test ./internal/contracts/schema -count=1
make -C test product-endpoint-standalone
```

- [ ] **Step 6: Commit**

```bash
git add test/suites/product/endpoint test/contracts/agent-test-coverage.tsv
git commit -m "test(agent): converge local product scenarios"
```

### Task 4: Real Tetragon, Namespace, And VM Lifecycle

**Files:**
- Modify: `test/shared/vm/sync-agent.sh`
- Modify: `test/suites/product/endpoint/e2e-real-tetragon-owned-vm.sh`
- Modify: `test/suites/product/endpoint/e2e-real-tetragon-owned-container.sh`
- Modify: `test/suites/product/endpoint/e2e-namespace-self-container.sh`
- Create: `test/suites/product/endpoint/enrolled-container-lifecycle.sh`
- Delete after coverage: managed restart/recover container and VM duplicates
- Modify: `test/contracts/agent-test-coverage.tsv`

**Interfaces:**
- VM sync installs the current package/config/policy and remains standalone.
- Enrolled lifecycle runner parameterizes `steady`, `restart`, and `recover`.

- [ ] **Step 1: Migrate VM sync and verify standalone startup**

The generated runtime config contains no cloud identity. Topology tests do not
use VM sync; they create a Manager enrollment and execute its `install_url`.

- [ ] **Step 2: Migrate real sensor and namespace runners**

Preserve collection selector, local Event/Signal, namespace exclusion, process
restart, content update, and policy compile assertions.

- [ ] **Step 3: Consolidate managed lifecycle variants**

Run all old assertions through `enrolled-container-lifecycle.sh` before deleting
the duplicate container and VM scripts.

- [ ] **Step 4: Verify**

```bash
make -C test product-endpoint-namespace-container
make -C test product-endpoint
go test ./internal/contracts/schema -count=1
```

- [ ] **Step 5: Commit**

```bash
git add test/shared/vm test/suites/product/endpoint test/contracts/agent-test-coverage.tsv
git commit -m "test(agent): converge sensor lifecycle scenarios"
```

### Task 5: Cloud, Topology, And Systemd Scenarios

**Files:**
- Create: `test/suites/product/topology/scenario-container.sh`
- Modify: `test/suites/product/topology/e2e-systemd-vm.sh`
- Modify: `test/suites/product/platform/e2e-gateway-local-ingest.sh`
- Delete after coverage: three `e2e-scenario-*-container.sh` scripts
- Modify: `test/Makefile`
- Modify: `test/contracts/agent-test-coverage.tsv`

**Interfaces:**
- `scenario-container.sh CASE` accepts `apt`, `staged`, or `benign` and maps to
  the existing workload, expected signals, expected incidents, and terminal
  assertions.

- [ ] **Step 1: Add the table-driven topology runner**

Keep distinct expected outcomes: apt and staged must produce their named
detections; benign must produce Events but no Signal, Incident, or terminal
response.

- [ ] **Step 2: Migrate systemd and Gateway tests**

Systemd, Gateway, and topology tests create a Manager enrollment and execute the
returned `install_url`. They do not call `sysarmorctl enroll` directly.

- [ ] **Step 3: Update public Make targets and delete covered scripts**

No Make target may reference deleted filenames.

- [ ] **Step 4: Verify cloud suites**

```bash
make -C test product-platform
make -C test product-topology
go test ./internal/contracts/schema -count=1
```

- [ ] **Step 5: Commit**

```bash
git add test/Makefile test/suites/product test/contracts/agent-test-coverage.tsv
git commit -m "test(agent): converge enrolled topology scenarios"
```

### Task 6: Performance And Operational Tooling

**Files:**
- Modify: `test/suites/performance/endpoint/run.sh`
- Modify: `test/suites/performance/endpoint/lifecycle.sh`
- Modify: `test/shared/diagnostics/capture-container.sh`
- Modify: `test/shared/diagnostics/capture-vm.sh`
- Modify: `test/shared/diagnostics/diagnose-tetragon-vm.sh`
- Modify: `test/shared/recorder/recorder-vm.sh`
- Modify: `test/contracts/agent-test-coverage.tsv`

**Interfaces:**
- Operational tools consume the shared install and inspect helpers.
- Performance tools alter only current `local.storage`, `local.export`, or
  `telemetry` fields whose effect is measured.

- [ ] **Step 1: Migrate performance scripts**

Remove direct edits to `/etc/sysarmor/agent.yaml`; use the current config path
and restart through the shared lifecycle operation. Preserve throughput,
resource, restart, and report assertions.

- [ ] **Step 2: Migrate capture, diagnosis, and recorder**

Use the current socket path and `sysarmorctl` inspection. Remove replay/stream
startup paths that use removed Agent CLI flags only after their Event, Signal,
and report assertions are mapped to retained current-pipeline scenarios.

- [ ] **Step 3: Verify operations**

```bash
make -C test performance-local-store
make -C test performance-endpoint
make -C test diag-endpoint
make -C test recorder-start
make -C test recorder-stop
make -C test recorder-report
```

- [ ] **Step 4: Commit**

```bash
git add test/suites/performance test/shared/diagnostics test/shared/recorder test/contracts/agent-test-coverage.tsv
git commit -m "test(agent): migrate performance and diagnostics"
```

### Task 7: Generated Mirror And Final Acceptance

**Files:**
- Modify through generation only: `test/environments/vm-topology/deploy/platform/`
- Modify: `test/contracts/agent-test-coverage.tsv`

- [ ] **Step 1: Regenerate the VM deployment mirror**

Run its owning sync path from `test/shared/harness/start-vm.sh`; do not manually
patch copied source files.

- [ ] **Step 2: Run zero-legacy scans**

```bash
rg -n '/etc/sysarmor/agent\.yaml|/etc/sysarmor/policies|/run/sysarmor/agent\.sock|^data_plane:|  batch_size:|  policy_path:' test
```

Expected: no matches, including the regenerated mirror.

- [ ] **Step 3: Run complete acceptance**

```bash
make api
go test ./...
go vet ./...
go test -race ./internal/agent/localstore ./internal/agent/daemon
pnpm --dir web/manager test
make -C test product-endpoint-standalone
make -C test performance-local-store
git diff --check
```

Run environment-backed public Make targets when their Docker/VM prerequisites
are available; record an explicit skipped reason otherwise.

- [ ] **Step 4: Check coverage and file sizes**

Every inventory row is `covered`, every replacement exists, retained shell
files stay below 500 lines, and functions stay below 50 lines unless an
existing external command block makes separation less clear.

- [ ] **Step 5: Commit final generated and contract updates**

```bash
git add test/contracts test/environments/vm-topology/deploy/platform
git commit -m "test(agent): verify converged scenario coverage"
```
