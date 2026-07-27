# Release Reliability Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Execute each task with a failing test before implementation.

**Goal:** Close rollback data-loss, tetra tail-loss, and default detection coverage gaps before Release.

**Architecture:** Keep each fix in its existing owner: Bash transaction engine, Tetragon supervisor, and
default collection policy. Do not add compatibility paths or unrelated refactors.

### Task 1: Preserve backups when rollback fails

- [ ] Extend `transactional-agent-installation.sh` so fake `mv` can fail during backup restoration.
- [ ] Verify the test fails because the backup is deleted and the error is swallowed.
- [ ] Make rollback return failure, retain failed backups, and propagate a nonzero exit.
- [ ] Run installation and standalone package contracts; commit `fix(agent): preserve failed rollback backups`.

### Task 2: Drain tetra stdout before process completion

- [ ] Add repeated tests that emit many final events and a final dropped-events record before immediate exit.
- [ ] Verify the current `StdoutPipe`/`Wait` race loses tail records.
- [ ] Replace concurrent `StdoutPipe` waiting with a supervised `io.Pipe` lifecycle.
- [ ] Run Tetragon tests with repetition and race detection; commit `fix(sensor): drain tetra output before wait`.

### Task 3: Cover default chmod dependencies

- [ ] Add a contract asserting default policy and content produce coverage status `covered`.
- [ ] Verify it reports missing `file.chmod` for payload rules.
- [ ] Add `file.chmod` to the default collection behavior list.
- [ ] Run coverage, content, and Release contracts; commit `fix(policy): collect default chmod events`.

### Task 4: Final verification

- [ ] Run `go test ./... -count=1` and all installation/Release contracts.
- [ ] Rebuild one immutable signed package and run the three-image no-cache matrix.
- [ ] Require zero missing EventRefs and `detection.lastApplyStatus=applied` in all health snapshots.
