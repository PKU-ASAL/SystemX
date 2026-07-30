# Agent Test Contract Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove obsolete Agent test paths and align every retained VM test entry with the current configuration, installation, and build contracts.

**Architecture:** Treat `internal/agent/config`, `deployments/agent/install-agent.sh`, and `deployments/agent/install-core.sh` as the sources of truth. Delete legacy entry points only where current topology/enrollment tests replace them, then enforce the retained shell assets with repository-level Go contract tests and focused Python execution contracts.

**Tech Stack:** Go tests, Python `unittest`, Bash, Make, Vagrant/libvirt, Docker.

## Global Constraints

- Do not modify Agent runtime behavior or installer defaults.
- Do not edit historical release result documents.
- Preserve temporary policy paths for tests that run Agent directly; installer-driven VM tests must use `/etc/sysarmor/agent/policy.json`.
- Use TDD for every behavior change and keep commits atomic.

---

### Task 1: Remove Obsolete Test Paths

**Files:**
- Delete: `test/shared/diagnostics/capture-container.sh`
- Delete: `test/suites/product/endpoint/e2e-real-tetragon-owned-container.sh`
- Delete: `test/suites/product/platform/e2e-gateway-local-ingest.sh`
- Delete: `configs/agent.fake.yaml`
- Modify: `test/Makefile`
- Modify: `test/contracts/agent-test-coverage.tsv`
- Modify: `internal/contracts/schema/agent_test_assets_test.go`
- Modify: `test/shared/harness/lib/common.sh`

**Interfaces:**
- Consumes: current enrollment path in `e2e-agent-gateway-manager-local.sh`, namespace container test, and topology scenario runner.
- Produces: a product suite and coverage inventory with no references to deleted legacy paths.

- [ ] **Step 1: Extend the inventory contract to reject obsolete paths**

Add a table-driven test to `internal/contracts/schema/agent_test_assets_test.go`:

```go
func TestObsoleteAgentTestAssetsAreRemoved(t *testing.T) {
    root := repositoryRoot(t)
    for _, path := range []string{
        "configs/agent.fake.yaml",
        "test/shared/diagnostics/capture-container.sh",
        "test/suites/product/endpoint/e2e-real-tetragon-owned-container.sh",
        "test/suites/product/platform/e2e-gateway-local-ingest.sh",
    } {
        if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); !os.IsNotExist(err) {
            t.Errorf("obsolete Agent test asset still exists: %s", path)
        }
    }
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
go test ./internal/contracts/schema -run '^TestObsoleteAgentTestAssetsAreRemoved$' -count=1
```

Expected: FAIL listing all four existing obsolete assets.

- [ ] **Step 3: Delete obsolete assets and references**

Delete the four files. Remove `e2e-gateway-local-ingest.sh` from `product-platform`, remove their rows from `agent-test-coverage.tsv`, remove `TestContainerCaptureCreatesWorkDirectory`, and remove the unreferenced `sa_build_all` function.

- [ ] **Step 4: Verify replacement suite references remain**

Run:

```bash
rg -n 'e2e-agent-gateway-manager-local|e2e-namespace-self-container|e2e-scenarios-container' test/Makefile test/contracts/agent-test-coverage.tsv
go test ./internal/contracts/schema -run '^(TestObsoleteAgentTestAssetsAreRemoved|TestAgentTestAssetsUseCurrentSchema)$' -count=1
```

Expected: replacement references are present and both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add configs/agent.fake.yaml test/Makefile test/contracts/agent-test-coverage.tsv internal/contracts/schema/agent_test_assets_test.go test/shared/harness/lib/common.sh test/shared/diagnostics/capture-container.sh test/suites/product/endpoint/e2e-real-tetragon-owned-container.sh test/suites/product/platform/e2e-gateway-local-ingest.sh
git commit -m "test: remove obsolete agent test paths"
```

### Task 2: Enforce Current VM Contracts

**Files:**
- Create: `test/shared/harness/test_start_vm_contract.py`
- Create: `test/suites/product/endpoint/test_e2e_contract.py`
- Modify: `internal/contracts/schema/agent_test_assets_test.go`
- Modify: `test/shared/harness/start-vm.sh`
- Modify: `test/shared/diagnostics/capture-vm.sh`
- Modify: `test/suites/product/endpoint/e2e-real-tetragon-owned-vm.sh`

**Interfaces:**
- Consumes: `make build-binary`, `SYSARMOR_CTL_BIN`, and `/etc/sysarmor/agent/policy.json` installer contracts.
- Produces: VM scripts accepted by the strict Agent config parser and unified installer.

- [ ] **Step 1: Add failing execution and asset contracts**

The start-VM Python test executes `start-vm.sh vm-endpoint` with temporary fake `make` and `vagrant` commands, then expects:

```text
-C <repository-root> build-binary
```

The Endpoint Python test requires:

```python
self.assertIn("SYSARMOR_CTL_BIN=/tmp/sysarmorctl.upload", self.script)
self.assertIn("  label.scenario: $SCENARIO", self.script)
self.assertIn("  path: /etc/sysarmor/agent/policy.json", self.script)
self.assertNotIn("  transport: local", self.script)
```

Extend the Go asset contract with exact multiline legacy patterns for shell-generated YAML and a focused installer check for both retained VM scripts.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
python3 test/shared/harness/test_start_vm_contract.py -v
python3 test/suites/product/endpoint/test_e2e_contract.py -v
go test ./internal/contracts/schema -run '^TestAgentTestAssetsUseCurrentSchema$' -count=1
```

Expected: failures identify `make build`, old VM diagnostic identity/manager fields, missing CTL input, and old policy path.

- [ ] **Step 3: Align retained VM scripts**

Apply only these changes:

```bash
# start-vm.sh
make -C "$REPO" build-binary
```

Generated standalone YAML must contain:

```yaml
agent:
  label.scenario: <scenario>

policy:
  path: /etc/sysarmor/agent/policy.json
```

Both installer calls must pass `SYSARMOR_CTL_BIN=/tmp/sysarmorctl.upload`. Wrap each install call so failure prints `/tmp/sysarmor-install-agent.log` and exits nonzero. Remove unused token variables, old manager blocks, cloud identity fields, and duplicate post-install CTL copies.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run:

```bash
python3 test/shared/harness/test_start_vm_contract.py -v
python3 test/suites/product/endpoint/test_e2e_contract.py -v
go test ./internal/contracts/schema -run '^TestAgentTestAssetsUseCurrentSchema$' -count=1
git diff --check
```

Expected: all tests PASS and `git diff --check` emits no output.

- [ ] **Step 5: Commit**

```bash
git add internal/contracts/schema/agent_test_assets_test.go test/shared/harness/start-vm.sh test/shared/harness/test_start_vm_contract.py test/shared/diagnostics/capture-vm.sh test/suites/product/endpoint/e2e-real-tetragon-owned-vm.sh test/suites/product/endpoint/test_e2e_contract.py
git commit -m "fix(test): align vm tests with agent install contract"
```

### Task 3: Full Validation and Cleanup

**Files:**
- Verify only; remove test-generated untracked files and ignored result/deployment artifacts created during this run.

**Interfaces:**
- Consumes: Tasks 1 and 2.
- Produces: evidence that retained local, container, and VM paths work end to end.

- [ ] **Step 1: Run unit and repository contracts**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run the retained platform suite outside the sandbox**

```bash
make -C test product-platform
```

Expected: all retained platform scripts PASS without the deleted direct-Gateway test.

- [ ] **Step 3: Run Endpoint outside the sandbox**

```bash
make -C test product-endpoint
```

Expected: Agent installs, applies content and policy, detects the VM scenario, restarts under systemd, and reports `ok`.

- [ ] **Step 4: Run Topology outside the sandbox**

```bash
make -C test product-topology
```

Expected: signed artifact upload, enrollment, installation, restart, and systemd assertions PASS.

- [ ] **Step 5: Clean generated artifacts and verify the worktree**

Use `git status --short --ignored` and modification timestamps to identify artifacts created by these tests. Run the repository cleanup target only for the exact test environments involved, then verify:

```bash
git status --short --branch
git diff --check
git log --oneline -3
```

Expected: no generated files remain; only the planned commits are ahead of the starting branch.
