# Unified Agent Installation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Execute this plan task-by-task with tests written and observed failing before implementation.

**Goal:** Make Release, source-development, and VM-test installation use one transactional Agent installation engine with signed default content present before first startup.

**Architecture:** `install-core.sh` owns validation, staging, commit, rollback, service lifecycle, and health checks. `install-release.sh` adapts a packaged signed source; `install-agent.sh` creates an ephemeral Ed25519 development trust root, signs source content, prepares a standard source tree, and invokes the same core.

**Tech Stack:** Bash, jq, OpenSSL, Go `sysarmor-content-sign`, systemd/Vagrant integration tests.

## Global Constraints

- Missing rulesets remain fatal; no unsigned-default compatibility switch is added.
- Development private keys exist only in a temporary directory and are deleted by `trap`.
- Both wrappers must delegate target mutation and service lifecycle to `install-core.sh`.
- Existing user configuration and Release content rollback contracts remain intact.
- The staged Release experiment report must not be modified or included in implementation commits.

---

### Task 1: Lock the unified-entry contract

**Files:**
- Create: `test/suites/product/endpoint/unified-agent-installation.sh`
- Modify: `test/Makefile`

**Interfaces:**
- Consumes: `deployments/agent/install-agent.sh`, `deployments/agent/install-release.sh`
- Produces: a product contract requiring both wrappers and `install-core.sh`

- [ ] Write a shell test that requires an executable `install-core.sh`, requires both wrappers to invoke it, and rejects service-start/transaction code in wrappers.
- [ ] Run `bash test/suites/product/endpoint/unified-agent-installation.sh` and verify it fails because `install-core.sh` does not exist.
- [ ] Add the test to the endpoint product target after it passes.

### Task 2: Extract the canonical transaction engine

**Files:**
- Create: `deployments/agent/install-core.sh`
- Modify: `deployments/agent/install-release.sh`
- Modify: `deployments/agent/package-agent.sh`

**Interfaces:**
- Consumes: explicit `SYSARMOR_INSTALL_*_SOURCE` paths plus existing destination variables
- Produces: one validation, stage, commit, rollback, service-start, and health-check implementation

- [ ] Move the existing Release transaction implementation into `install-core.sh` and parameterize source paths without adding install modes.
- [ ] Make `install-release.sh` validate the packaged layout, map its files to core inputs, and `exec` the core.
- [ ] Include `install-core.sh` in Release packages and their signed manifest.
- [ ] Run the unified-entry contract and observe it pass.
- [ ] Run `bash test/suites/product/endpoint/standalone-release-package.sh` and preserve all current configuration/content rollback assertions.

### Task 3: Adapt development installation with ephemeral signing

**Files:**
- Modify: `deployments/agent/install-agent.sh`
- Modify: `Makefile`
- Modify: `test/shared/vm/sync-agent.sh`
- Modify: `test/shared/diagnostics/capture-vm.sh`
- Modify: `test/suites/product/endpoint/unified-agent-installation.sh`

**Interfaces:**
- Consumes: `SYSARMOR_CONTENT_SIGN_BIN`, raw `deployments/agent/content`, development config and policy
- Produces: signed temporary default content, matching trust-key configuration, then delegates to `install-core.sh`

- [ ] Extend the product test with fake installation destinations and assert development content is signed, installed, and referenced before service startup.
- [ ] Run it and verify failure because the development wrapper does not prepare default content.
- [ ] Build and pass `sysarmor-content-sign` alongside Agent/CLI binaries.
- [ ] Generate an ephemeral Ed25519 key in `mktemp -d`, sign every source content envelope, generate `content-manifest.json`, append the public trust key and default paths to a temporary config, and invoke the core.
- [ ] Ensure the wrapper trap removes private key material on success, failure, `INT`, and `TERM`.
- [ ] Run the product contract and relevant shell syntax checks.

### Task 4: Verify installation and performance regression

**Files:**
- No production changes unless a failing test identifies a scoped defect.

**Interfaces:**
- Consumes: unified installer from Tasks 2-3
- Produces: regression evidence for Release and performance paths

- [ ] Run `go test ./...`.
- [ ] Run `bash test/suites/product/endpoint/standalone-release-package.sh`.
- [ ] Run `make test-performance PROFILE=quick WORKLOAD=business-normal`; require a complete summary, not only exit code zero.
- [ ] Run `make test-performance PROFILE=medium WORKLOAD=business-normal SCENARIO=apt-fileless-c2-local POLICIES=test/data/policies/collection-balanced.json`; require it to enter and complete sampling.
- [ ] Inspect manifests, summaries, errors, and git status; report any residual infrastructure warnings separately.
