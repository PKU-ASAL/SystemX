# Release Real Detection Scenarios Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the synthetic Release marker smoke test with a real Node.js workload that exercises `web_runtime_spawns_shell`, `download_by_lolbin`, and `payload_lifecycle`, while removing restart testing and preventing stale result reuse.

**Architecture:** A dependency-free Node business service launches real child processes through three HTTP endpoints. A sibling Node attack service provides download and control endpoints on the built-in IOC ports. The detection engine resolves Web Runtime ancestry from observed stable process identities, while Release scripts map each scenario to rule-specific Event/Signal assertions.

**Tech Stack:** Go, Bash, Node.js standard library, Docker, jq, sysarmorctl.

## Global Constraints

- Test Ubuntu 22.04, Ubuntu 24.04, and Debian 12 serially.
- Do not add npm dependencies or public attack targets.
- Cover exactly `web_runtime_spawns_shell`, `download_by_lolbin`, and `payload_lifecycle`.
- Do not enable or depend on `file.chmod`.
- Remove `RESTART_TEST` from the Release test surface.
- Accept `RUN_ID` only when it matches `[A-Za-z0-9][A-Za-z0-9._-]*`.
- Preserve `namespace/self` sibling-container and host isolation checks.

---

### Task 1: Detect Real Web Runtime Parentage

**Files:**
- Modify: `internal/endpoint/detection/engine.go`
- Modify: `internal/endpoint/detection/engine_test.go`

**Interfaces:**
- Consumes: canonical `process.exec` events with `subjectProc.stableId`, `subjectProc.binary`, and `parentStableId`.
- Produces: `lineageState.processBinaryByStableID map[string]string` and parent-aware `looksLikeWebRuntime(binary string) bool` behavior.

- [ ] **Step 1: Write failing unit tests**

Add tests that process a Node exec followed by a Shell exec whose `ParentStableId` is the Node stable ID, and assert one `web_runtime_spawns_shell` Signal. Add negative tests proving a Shell with `node` only in argv and a Shell parented by a non-Web binary do not trigger.

```go
func TestWebRuntimeShellUsesObservedParentBinary(t *testing.T) {
    engine, _ := New(policymodel.DefaultDetectionPolicy())
    engine.Process(execEvent("node", "lin-web", "node-proc", "init", "/usr/bin/node", []string{"/usr/bin/node", "/srv/server.js"}))
    shell := execEvent("shell", "lin-web", "shell-proc", "node-proc", "/bin/sh", []string{"/bin/sh", "-c", "id"})
    if got := countSignals(engine.Process(shell), "web_runtime_spawns_shell"); got != 1 {
        t.Fatalf("web runtime shell signals = %d, want 1", got)
    }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/endpoint/detection -run 'TestWebRuntimeShell' -count=1`

Expected: the real-parent test fails because the current detector only searches Runtime tokens in Shell argv.

- [ ] **Step 3: Implement minimal parent identity tracking**

Extend `lineageState` with a stable-ID-to-binary map. Record normalized process binaries in `remember`. Change `detectWebRuntimeShell` to resolve the direct parent by `ParentStableId` and classify only that binary as a supported Web Runtime. Remove argv token matching.

```go
type lineageState struct {
    processBinaryByStableID map[string]string
    // existing state remains unchanged
}

func (s *lineageState) remember(ev *eventv1.CanonicalEvent) {
    proc := ev.GetSubjectProc()
    if proc == nil || eventBehavior(ev) != eventmodel.BehaviorProcessExec.String() {
        return
    }
    if stableID := proc.GetStableId(); stableID != "" {
        s.lastExecByStableID[stableID] = ev.GetId()
        s.processBinaryByStableID[stableID] = proc.GetBinary()
    }
}
```

- [ ] **Step 4: Verify GREEN and regression suite**

Run: `go test ./internal/endpoint/detection -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/endpoint/detection/engine.go internal/endpoint/detection/engine_test.go
git commit -m "fix(detection): resolve web runtime parent identity"
```

### Task 2: Add Real Node Business and Attack Services

**Files:**
- Create: `test/release/fixtures/web-app/server.js`
- Create: `test/release/fixtures/payload-server/server.js`
- Create: `test/release/fixtures/test-fixtures.sh`
- Modify: `test/release/images/ubuntu2204/Dockerfile`
- Modify: `test/release/images/ubuntu2404/Dockerfile`
- Modify: `test/release/images/debian12/Dockerfile`
- Delete: `test/release/attacks/web-runtime-spawns-shell.sh`
- Create: `test/release/attacks/web-runtime-shell.sh`
- Create: `test/release/attacks/download-by-lolbin.sh`
- Create: `test/release/attacks/payload-lifecycle.sh`

**Interfaces:**
- Business service listens on `127.0.0.1:3000` and provides `/healthz`, `/rce`, `/download`, and `/payload`.
- Attack service listens on `0.0.0.0:8080` and `0.0.0.0:8443`.
- Each attack script consumes `CONTAINER MARKER ATTACK_HOST` and calls one business endpoint through `docker exec`.

- [ ] **Step 1: Write failing fixture contract**

Create `test-fixtures.sh` that starts both Node services on temporary ports, verifies `/healthz`, rejects an invalid marker, invokes all three endpoints with a valid marker, and checks that the payload and control request were observed.

```bash
curl -fsS "http://127.0.0.1:$web_port/healthz" | jq -e '.status == "ok"'
if curl -fsS "http://127.0.0.1:$web_port/rce?marker=bad%20marker"; then
  echo "invalid marker accepted" >&2
  exit 1
fi
curl -fsS "http://127.0.0.1:$web_port/rce?marker=sysarmor-fixture-rce"
```

- [ ] **Step 2: Verify RED**

Run: `bash test/release/fixtures/test-fixtures.sh`

Expected: FAIL because the fixture files do not exist yet.

- [ ] **Step 3: Implement dependency-free Node services and attack scripts**

Use `node:http`, `node:child_process`, and `node:url`. Validate markers with `/^[A-Za-z0-9._-]+$/`. Spawn commands without interpolating unvalidated text. The payload server must generate a Shell payload that requests `http://ATTACK_HOST:8443/control?marker=<marker>`.

Add `nodejs` to each image, copy fixtures under `/opt/sysarmor-release-test`, expose no host ports, and replace `CMD ["sleep", "infinity"]` with `CMD ["node", "/opt/sysarmor-release-test/web-app/server.js"]`.

- [ ] **Step 4: Verify GREEN**

Run: `bash test/release/fixtures/test-fixtures.sh`

Expected: `[release-fixtures] ok`.

- [ ] **Step 5: Commit**

```bash
git add test/release/fixtures test/release/attacks test/release/images
git commit -m "test(release): add real Node attack workload"
```

### Task 3: Add Rule-Aware Scenario Assertions

**Files:**
- Create: `test/release/scenarios.sh`
- Modify: `test/release/assert.sh`
- Create: `test/release/test-assert.sh`
- Modify: `test/suites/product/endpoint/release-container-e2e-contract.sh`

**Interfaces:**
- `scenario_rule NAME`, `scenario_severity NAME`, `scenario_behaviors NAME`, and `scenario_attack NAME` return fixed metadata for the three approved scenarios.
- `assert.sh detected CONTAINER SCENARIO MARKER OUTPUT` validates rule, severity, complete references, required behavior coverage, marker binding, and scenario network port.
- `assert.sh absent CONTAINER MARKER OUTPUT` polls for the full isolation window and fails immediately on a marker hit.

- [ ] **Step 1: Write failing assertion tests**

Create a fake `docker` executable that emits fixed health, Event, and Signal NDJSON. Test one success document per scenario and failures for wrong severity, missing Event refs, missing required behavior, wrong port, and external marker presence.

```bash
PATH="$fake_bin:$PATH" "$RELEASE/assert.sh" detected fake payload-lifecycle marker-1 "$tmp/out.jsonl"
if FAKE_CASE=missing-ref PATH="$fake_bin:$PATH" \
  "$RELEASE/assert.sh" detected fake payload-lifecycle marker-1 "$tmp/out.jsonl"; then
  echo "missing Event reference accepted" >&2
  exit 1
fi
```

- [ ] **Step 2: Verify RED**

Run: `bash test/release/test-assert.sh`

Expected: FAIL because scenario-aware interfaces are absent.

- [ ] **Step 3: Implement scenario metadata and structured jq assertions**

Define exactly these mappings:

```text
web-runtime-shell -> web_runtime_spawns_shell, high, process.exec
download-by-lolbin -> download_by_lolbin, medium, network.connect, port 8080
payload-lifecycle -> payload_lifecycle, high, file.write process.exec network.connect, ports 8080/8443
```

Make the jq predicate collect associated Event behaviors and require every configured behavior. Bind the marker through process argv, file path, or socket/request metadata. Preserve JSONL and stderr output for every attempt.

- [ ] **Step 4: Verify GREEN and static contract**

Run:

```bash
bash test/release/test-assert.sh
bash test/suites/product/endpoint/release-container-e2e-contract.sh
```

Expected: both print `ok`.

- [ ] **Step 5: Commit**

```bash
git add test/release/scenarios.sh test/release/assert.sh test/release/test-assert.sh test/suites/product/endpoint/release-container-e2e-contract.sh
git commit -m "test(release): assert multi-rule event evidence"
```

### Task 4: Orchestrate Real Services and Clean Results

**Files:**
- Modify: `test/release/run.sh`
- Modify: `test/release/config.sh`
- Modify: `test/release/doctor.sh`
- Modify: `test/release/Makefile`
- Modify: `test/release/README.md`
- Modify: `test/suites/product/endpoint/release-container-e2e-contract.sh`

**Interfaces:**
- `run.sh` creates one network, one attack server, and one business container per image.
- `prepare_result_root` validates `RUN_ID`, removes only the exact validated result directory, and recreates it.
- No Release file exposes `RESTART_TEST`.

- [ ] **Step 1: Extend the failing contract**

Require the three scenarios, Node readiness, dedicated Docker network, attacker cleanup, strict `RUN_ID` regex, old result removal, and absence of `RESTART_TEST`. Add a test that pre-creates a sentinel under a valid `RUN_ID`, runs the result preparation helper, and asserts the sentinel is removed. Add invalid IDs such as `../escape` and `/tmp/escape` and assert rejection.

- [ ] **Step 2: Verify RED**

Run: `bash test/suites/product/endpoint/release-container-e2e-contract.sh`

Expected: FAIL on missing lifecycle and result preparation contracts.

- [ ] **Step 3: Implement minimal orchestration**

Remove all restart settings and functions. Source `scenarios.sh`, prepare a clean result root before writing `install-url.txt`, create a per-image network, start the attacker without host port mappings, wait for business health through `docker exec curl`, execute the three attack scripts, and invoke scenario-aware assertions. Extend the trap to capture logs/inspect and remove both containers and the network.

- [ ] **Step 4: Update documentation and verify local contracts**

Run:

```bash
bash -n test/release/*.sh test/release/attacks/*.sh test/release/fixtures/*.sh
bash test/release/fixtures/test-fixtures.sh
bash test/release/test-assert.sh
bash test/suites/product/endpoint/release-container-e2e-contract.sh
bash test/suites/product/endpoint/dev-prerelease-workflow.sh
git diff --check
```

Expected: all commands exit zero.

- [ ] **Step 5: Run Go regression tests**

Run: `go test ./internal/endpoint/detection ./internal/agent/daemon ./internal/sensors/linux/tetragon`

Expected: PASS.

- [ ] **Step 6: Run fixed-Release real matrix**

Run:

```bash
make -C test/release test \
  URL='https://github.com/PKU-ASAL/sysarmor/releases/download/v0.1.0-dev.202607241044%2B73c71ce3/install.sh' \
  IMAGES='ubuntu2204 ubuntu2404 debian12' \
  RUN_ID='real-scenarios-v0.1.0-dev.202607241044-73c71ce3' \
  FRESH_DOWNLOAD=1 \
  RELEASE_PROXY_URL='https://gh-proxy.org'
```

Expected: all three images pass all three scenarios and isolation checks.

- [ ] **Step 7: Commit**

```bash
git add test/release test/suites/product/endpoint/release-container-e2e-contract.sh
git commit -m "test(release): run real multi-rule container scenarios"
```

### Task 5: Final Review

**Files:**
- Review all changes since `5cbba19b`.

**Interfaces:**
- Consumes: completed Tasks 1-4.
- Produces: review findings resolved, clean worktree, and final verification record.

- [ ] **Step 1: Run complete focused verification**

Run all commands from Task 4 Steps 4-6 and record exact outcomes.

- [ ] **Step 2: Request independent code review**

Review the Git range `5cbba19b..HEAD` for false positives, unsafe cleanup, shell injection, flaky timing, and missing evidence assertions.

- [ ] **Step 3: Fix Critical and Important findings with TDD**

For every accepted finding, add or tighten a failing test first, verify RED, apply the minimal fix, and rerun the focused suite.

- [ ] **Step 4: Final status check**

Run: `git status --short && git log --oneline 5cbba19b..HEAD`

Expected: clean worktree and atomic Conventional Commits for each task.
