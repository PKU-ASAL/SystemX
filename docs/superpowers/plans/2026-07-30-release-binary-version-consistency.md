# Release Binary Version Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure every GitHub Release binary reports the exact Release version and block publication when Agent, CLI, manifest, and tag versions can diverge.

**Architecture:** Keep the existing reusable Release Workflow and inject its validated `VERSION` directly into both Go `main.version` variables. Extend the existing shell contract test to require the injection and executable checks, then make the Workflow execute both freshly built binaries before packaging.

**Tech Stack:** GitHub Actions YAML, Bash, Go linker flags, existing Distribution package contract tests.

## Global Constraints

- Publish a new immutable `v0.1.0-rc.5`; never modify `v0.1.0-rc.4`.
- Preserve the existing RC and Stable workflow structure.
- Use exact version equality; do not accept `dev` or partial matches.
- Follow `fix -> dev -> release/v0.1.0` through PRs.

---

### Task 1: Define the Release Workflow Contract

**Files:**
- Modify: `test/suites/distribution/package/release-workflow-contract.sh`
- Test: `test/suites/distribution/package/release-workflow-contract.sh`

**Interfaces:**
- Consumes: `.github/workflows/release-build.yml` as text.
- Produces: contract assertions for both linker injection and executable version checks.

- [ ] **Step 1: Add failing contract assertions**

Require the reusable build workflow to contain both exact build commands:

```bash
CGO_ENABLED=0 go build -ldflags "-X main.version=$VERSION" -o dist/bin/sysarmor-agent ./cmd/sysarmor-agent
CGO_ENABLED=0 go build -ldflags "-X main.version=$VERSION" -o dist/bin/sysarmorctl ./cmd/sysarmorctl
```

Also require exact comparisons of `sysarmor-agent version` and `sysarmorctl version` with `$VERSION` before packaging.

- [ ] **Step 2: Run the contract and verify RED**

Run:

```bash
bash test/suites/distribution/package/release-workflow-contract.sh
```

Expected: non-zero exit because the current Workflow does not contain linker flags or executable version checks.

- [ ] **Step 3: Commit the failing test**

```bash
git add test/suites/distribution/package/release-workflow-contract.sh
git commit -m "test(release): require binary version consistency"
```

### Task 2: Inject and Verify Release Versions

**Files:**
- Modify: `.github/workflows/release-build.yml`
- Test: `test/suites/distribution/package/release-workflow-contract.sh`

**Interfaces:**
- Consumes: validated Workflow environment variable `VERSION`.
- Produces: `dist/bin/sysarmor-agent` and `dist/bin/sysarmorctl` whose `version` command outputs exactly `VERSION`.

- [ ] **Step 1: Add minimal Workflow implementation**

Build both binaries with:

```bash
-ldflags "-X main.version=$VERSION"
```

Before `package-agent.sh`, execute both binaries and compare their output exactly with `$VERSION`. Print a specific error naming the mismatched binary and fail before packaging.

- [ ] **Step 2: Run focused tests and verify GREEN**

Run:

```bash
bash test/suites/distribution/package/release-workflow-contract.sh
go test ./cmd/sysarmor-agent ./cmd/sysarmorctl
```

Expected: both commands pass.

- [ ] **Step 3: Run the complete local Release gate**

Run:

```bash
make test-release STAGE=pre-publish
git diff --check
```

Expected: all Release tests pass and no whitespace errors are reported.

- [ ] **Step 4: Commit the implementation**

```bash
git add .github/workflows/release-build.yml
git commit -m "fix(release): embed versions in release binaries"
```

### Task 3: Publish and Verify RC.5

**Files:**
- No source changes.
- Generated test evidence: `test/.results/release/<run-id>/` remains ignored.

**Interfaces:**
- Consumes: merged `release/v0.1.0` and public GitHub Release assets.
- Produces: immutable Pre-release `v0.1.0-rc.5` with consistent versions.

- [ ] **Step 1: Merge through protected branches**

Push the fix branch, merge its PR to `dev`, then merge `dev` to `release/v0.1.0`. Do not push commits directly to protected branches.

- [ ] **Step 2: Trigger and monitor RC.5**

Run:

```bash
make release-rc VERSION=0.1.0 RC=5
```

Expected: validation, tests, build/sign, attestation, upload, and Pre-release jobs all pass.

- [ ] **Step 3: Verify the public package versions**

Download `install.sh`, `SHA256SUMS`, and `sysarmor-agent-linux-amd64-v0.1.0-rc.5.tar.gz`; verify checksums, extract to a temporary directory, and assert:

```text
manifest.json = v0.1.0-rc.5
sysarmor-agent version = v0.1.0-rc.5
sysarmorctl version = v0.1.0-rc.5
```

- [ ] **Step 4: Run post-publish Distribution acceptance**

Run:

```bash
make test-release STAGE=post-publish \
  URL=https://github.com/PKU-ASAL/sysarmor/releases/download/v0.1.0-rc.5/install.sh
```

Expected: Ubuntu 22.04, Ubuntu 24.04, and Debian 12 all pass.
