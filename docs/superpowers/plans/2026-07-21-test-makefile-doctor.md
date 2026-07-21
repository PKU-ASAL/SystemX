# Test Makefile Doctor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide one test environment doctor and concise root-level test commands with actionable remediation messages.

**Architecture:** Keep all test logic in `test/Makefile`. The root `Makefile` delegates to it so there is one authoritative implementation and no duplicated environment logic.

**Tech Stack:** GNU Make, Bash, existing Go contract tests.

## Global Constraints

- `test/Makefile` exposes only one `doctor` target.
- Failed checks must print a direct remediation command or configuration instruction.
- Existing test targets and variable names remain compatible.

---

### Task 1: Test environment doctor

**Files:**
- Modify: `test/Makefile`
- Test: `internal/contracts/schema/agent_test_assets_test.go`

- [ ] Add a failing contract test requiring `doctor`, tool checks, libvirt provider validation, Tetragon archive discovery, and remediation text.
- [ ] Run the focused Go test and confirm it fails because `doctor` is absent.
- [ ] Implement the minimal `doctor` target and make VM-dependent targets depend on it.
- [ ] Run `make -C test doctor` and the focused Go test.

### Task 2: Root test delegates

**Files:**
- Modify: `Makefile`
- Test: `internal/contracts/schema/agent_test_assets_test.go`

- [ ] Extend the contract test to require root delegates for help, doctor, unit, and performance.
- [ ] Run the focused test and confirm it fails because delegates are absent.
- [ ] Add thin root targets forwarding variables to `test/Makefile`.
- [ ] Run root help, doctor, dry-run performance, schema tests, and `git diff --check`.
