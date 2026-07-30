# GitHub Release Make Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two validated Make commands that trigger the existing RC and Stable GitHub Release workflows without exposing workflow implementation details.

**Architecture:** The root `Makefile` validates `VERSION`, `RC`, `gh` availability, and authentication before calling `gh workflow run`. The existing shell Release Workflow contract provides regression coverage with a fake `gh`, so tests never contact GitHub or trigger a release.

**Tech Stack:** GNU Make, Bash, GitHub CLI, existing GitHub Actions workflows.

## Global Constraints

- Keep existing `make release` local-package behavior unchanged.
- Add only `release-rc` and `release-stable`; do not add waiting, status, branch creation, merging, or approval bypass.
- `VERSION` must match `MAJOR.MINOR.PATCH`; `RC` must be a positive integer.
- Tests must not access GitHub or trigger workflows.

---

### Task 1: Release Command Contract and Make Targets

**Files:**
- Modify: `test/suites/distribution/package/release-workflow-contract.sh`
- Modify: `Makefile`

**Interfaces:**
- Produces: `make release-rc VERSION=x.y.z RC=n` and `make release-stable VERSION=x.y.z RC=n`.

- [ ] **Step 1: Extend the failing shell contract**

Assert the root Makefile defines both targets and contains these exact workflow calls:

```bash
gh workflow run release-candidate.yml --ref "release/v$(VERSION)" -f "rc_number=$(RC)"
gh workflow run release-stable.yml --ref main -f "version=$(VERSION)" -f "accepted_rc_tag=v$(VERSION)-rc.$(RC)"
```

Create a temporary fake `gh` executable. Use it to verify valid Make calls emit the expected arguments; missing/invalid parameters, missing `gh`, and failed `gh auth status` return nonzero without invoking `workflow run`.

- [ ] **Step 2: Verify RED**

Run: `bash test/suites/distribution/package/release-workflow-contract.sh`

Expected: fail because `release-rc` and `release-stable` do not exist.

- [ ] **Step 3: Implement the Make targets**

Add a shared `check-github-release-inputs` target that validates version and RC with shell `case`/regex checks, runs `command -v gh`, and runs `gh auth status`. Make both public targets depend on it and call their exact workflows.

- [ ] **Step 4: Verify GREEN**

Run:

```bash
bash test/suites/distribution/package/release-workflow-contract.sh
make -n release-rc VERSION=1.0.0 RC=1
make -n release-stable VERSION=1.0.0 RC=1
```

Expected: contract passes and dry runs show exact workflow arguments.

- [ ] **Step 5: Commit**

```bash
git add Makefile test/suites/distribution/package/release-workflow-contract.sh
git commit -m "feat(release): add GitHub release commands"
```

### Task 2: Documentation and Final Verification

**Files:**
- Modify: `Makefile`
- Modify: `docs/development/development.md`

**Interfaces:**
- Consumes: Task 1 commands.
- Produces: concise operator documentation and Make help.

- [ ] **Step 1: Update help and release documentation**

Document the two recommended commands, their generated tags, branch requirements, and the GitHub Actions page fallback. Keep production Environment and signing-secret requirements intact.

- [ ] **Step 2: Verify documentation and existing release contracts**

Run:

```bash
make help
bash test/suites/distribution/package/release-workflow-contract.sh
make test-distribution SOURCE=local
git diff --check
```

Expected: help shows both commands; all Distribution package contracts pass.

- [ ] **Step 3: Commit**

```bash
git add Makefile docs/development/development.md
git commit -m "docs(release): document simplified release flow"
```
