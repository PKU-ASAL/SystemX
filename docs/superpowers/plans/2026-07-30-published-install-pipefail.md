# Published Install Pipefail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make published package Docker builds fail immediately when either download or installer execution fails.

**Architecture:** Extend the existing Release container contract before changing implementation. Configure all three published Dockerfiles to use Bash with `pipefail`, preserving the existing `curl | bash` installation flow.

**Tech Stack:** Dockerfile, Bash, existing shell contract tests.

## Global Constraints

- Modify test infrastructure only.
- Merge only to `dev`.
- Do not change or republish `v0.1.0-rc.5`.

---

### Task 1: Define Fail-Closed Contract

**Files:**
- Modify: `test/suites/distribution/package/release-container-e2e-contract.sh`
- Test: `test/suites/distribution/package/release-container-e2e-contract.sh`

- [ ] Add assertions requiring `SHELL ["/bin/bash", "-o", "pipefail", "-c"]` in all three published Dockerfiles.
- [ ] Run the contract and confirm it fails on the first missing declaration.
- [ ] Commit with `test(distribution): require fail-closed published installs`.

### Task 2: Enable Pipefail

**Files:**
- Modify: `test/suites/distribution/published/images/ubuntu2204/Dockerfile`
- Modify: `test/suites/distribution/published/images/ubuntu2404/Dockerfile`
- Modify: `test/suites/distribution/published/images/debian12/Dockerfile`

- [ ] Add the same Bash `pipefail` `SHELL` declaration after each `FROM` instruction.
- [ ] Run the focused contract and confirm it passes.
- [ ] Run `make test-distribution SOURCE=local` and `git diff --check`.
- [ ] Commit with `fix(distribution): fail closed on published install errors`.

### Task 3: Verify and Merge

**Files:**
- No product source changes.

- [ ] Run Debian post-publish against the immutable rc.5 URL when GitHub downloads are available.
- [ ] Request independent code review and resolve findings.
- [ ] Push the fix branch and merge its PR to `dev`.
- [ ] Verify `release/v0.1.0` and `v0.1.0-rc.5` target remain unchanged.
