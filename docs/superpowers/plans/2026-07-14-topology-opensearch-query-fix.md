# Topology OpenSearch Query Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 OpenSearch 精确查询、endpoint signal projection 身份和 topology 查询稳定性，使 apt、staged、benign linux-container installer 场景全部通过。

**Architecture:** `SearchRequest.Exact` 直接表示 term 查询字段，动态 labels 单独保留 `.keyword`。endpoint signal 文档 ID 绑定 tenant、agent 和本地 signal ID；worker history 复用修复后的精确查询完成跨 batch 关联。topology 继续通过 Manager API 验收，并用窄事件查询避免退出事件噪声。

**Tech Stack:** Go、OpenSearch、Bash、Docker Compose、Go testing

## Global Constraints

- 不修改 OpenSearch mappings，不引入索引迁移。
- 不改变 cloud signal projection key 的幂等语义。
- 所有生产代码修改必须先有能稳定复现缺陷的失败测试。
- 只提交本计划涉及的文件，不包含工作区既有改动。

---

### Task 1: Correct Exact Field Queries

**Files:**
- Modify: `internal/platform/opensearch/opensearch_test.go`
- Modify: `internal/platform/opensearch/opensearch.go`
- Modify: `internal/contracts/schema/agent_test_assets_test.go`

**Interfaces:**
- Consumes: `SearchRequest.Exact map[string]string` and deployment mapping JSON.
- Produces: term filters on exact field names; labels remain `labels.<key>.keyword`.

- [ ] **Step 1: Write the failing query test**

Change the expected query fragments to require exact top-level fields:

```go
for _, want := range []string{
    `"labels.scenario.keyword":"apt-staged-drop"`,
    `"where":"SIGNAL_WHERE_CLOUD"`,
    `"tenant_id":"default"`,
} {
    if !strings.Contains(got, want) {
        t.Fatalf("search body missing %s: %s", want, got)
    }
}
if strings.Contains(got, `"where.keyword"`) || strings.Contains(got, `"tenant_id.keyword"`) {
    t.Fatalf("keyword fields must not gain a .keyword suffix: %s", got)
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/platform/opensearch -run TestHTTPIndexerSearchPushesFilters -count=1`

Expected: FAIL because the query contains `where.keyword` and `tenant_id.keyword`.

- [ ] **Step 3: Implement direct Exact term fields**

```go
for field, value := range search.Exact {
    if field = strings.TrimSpace(field); field != "" {
        filters = append(filters, termFilter(field, value))
    }
}
```

- [ ] **Step 4: Add the mapping contract test**

Read `deployments/opensearch/mappings/events-v1.json` and `signals-v1.json`; assert `behavior`, `tenant_id`, and `where` are declared as `keyword`, while query tests continue to require `.keyword` for dynamic labels.

- [ ] **Step 5: Verify Task 1**

Run: `go test ./internal/platform/opensearch ./internal/contracts/schema -count=1`

Expected: PASS.

### Task 2: Isolate Endpoint Signal Projection IDs

**Files:**
- Modify: `internal/workers/ingest/processor_test.go`
- Modify: `internal/workers/ingest/processor.go`

**Interfaces:**
- Consumes: batch header tenant/agent identity and endpoint signal ID.
- Produces: deterministic OpenSearch document ID unique across tenant and agent.

- [ ] **Step 1: Write failing projection identity tests**

Add tests asserting that identical local signal IDs from two agents produce different document IDs, while a retry from the same tenant and agent produces the same ID.

```go
first := EndpointSignalDocumentID("default", "agent-a", "sig-1")
second := EndpointSignalDocumentID("default", "agent-b", "sig-1")
if first == second { t.Fatal("endpoint signal IDs collide across agents") }
if first != EndpointSignalDocumentID("default", "agent-a", "sig-1") {
    t.Fatal("endpoint signal ID is not deterministic")
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/workers/ingest -run 'TestEndpointSignalDocumentID|TestSignalDocument' -count=1`

Expected: FAIL because endpoint projection currently returns only `sig.GetId()`.

- [ ] **Step 3: Implement the minimal composite projection key**

Pass batch tenant and agent identity into endpoint signal document construction. Hash `tenantID + "\x00" + agentID + "\x00" + signalID` with SHA-256 and prefix it with `endpoint-signal:`. Keep cloud signals on `store.SignalProjectionKey`.

- [ ] **Step 4: Verify Task 2**

Run: `go test ./internal/workers/ingest -count=1`

Expected: PASS, including existing idempotency and cloud projection tests.

### Task 3: Prove Cross-Batch History Correlation

**Files:**
- Modify: `internal/workers/ingest/history_test.go`
- Modify: `internal/workers/ingest/worker_test.go`

**Interfaces:**
- Consumes: corrected Exact queries and OpenSearch history documents.
- Produces: staged cloud signal and incident when payload/connect signals arrive in separate batches.

- [ ] **Step 1: Strengthen history request assertions**

Assert history requests use exact `tenant_id` and `where` values and that query serialization uses the mapping-compatible field names.

- [ ] **Step 2: Add a split-batch staged regression test**

Process a batch containing `payload_dropped`, expose it through the history reader, then process a second batch containing `suspicious_exec_connect`. Assert one `dropped_payload_executed_and_connects` document with `crossLineage=true` and one incident document.

- [ ] **Step 3: Run the regression tests**

Run: `go test ./internal/workers/ingest -run 'TestOpenSearchHistory|TestProcessor.*Staged|TestSignalDocument' -count=1`

Expected: PASS after Tasks 1 and 2; any failure must be resolved without broadening production scope.

### Task 4: Stabilize Topology Assertions

**Files:**
- Modify: `test/suites/product/topology/scenario-container.sh`
- Modify: `internal/contracts/schema/agent_test_assets_test.go`

**Interfaces:**
- Consumes: Manager events/signals/incidents API.
- Produces: bounded, behavior-specific event readiness checks and preserved layer/terminal assertions.

- [ ] **Step 1: Write a failing asset contract**

Assert the runner checks a scenario-relevant event behavior with an explicit limit instead of fetching an unfiltered default page dominated by `process.exit`.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/contracts/schema -run TestContainerTopologyUsesProtectedContainerInstaller -count=1`

Expected: FAIL because the runner currently calls events list with only a label.

- [ ] **Step 3: Narrow the event readiness query**

Use `network.connect` for apt/staged and `process.exec` for benign, with an explicit bounded limit supported by `sysarmorctl`. Keep endpoint/cloud layer and terminal assertions unchanged.

- [ ] **Step 4: Verify Task 4**

Run: `go test ./internal/contracts/schema ./cmd/sysarmorctl -count=1`

Expected: PASS.

### Task 5: Full Verification And Review

**Files:**
- Verify only: all files modified above.

**Interfaces:**
- Consumes: current source tree and Docker topology.
- Produces: unit-test and product-scenario evidence.

- [ ] **Step 1: Run focused and package tests**

Run: `go test ./internal/platform/opensearch ./internal/workers/ingest ./internal/contracts/schema ./internal/manager/api ./cmd/sysarmorctl -count=1`

Expected: PASS.

- [ ] **Step 2: Run formatting and diff checks**

Run: `gofmt -w <modified-go-files>`

Run: `git diff --check`

Expected: no output from `git diff --check`.

- [ ] **Step 3: Rebuild and run all topology scenarios**

Run: `bash test/suites/product/topology/e2e-scenarios-container.sh`

Expected: `topology apt passed`, `topology staged passed`, and `topology benign passed`.

- [ ] **Step 4: Review the scoped diff**

Review only the plan files and implementation files against the design. Resolve all critical and important findings, then rerun affected tests.

- [ ] **Step 5: Commit implementation atomically**

```bash
git add internal/platform/opensearch internal/workers/ingest internal/contracts/schema/agent_test_assets_test.go test/suites/product/topology/scenario-container.sh docs/superpowers/plans/2026-07-14-topology-opensearch-query-fix.md
git commit -m "fix: restore topology signal correlation"
```
