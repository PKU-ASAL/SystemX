# Test Taxonomy Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将当前 Product、Effectiveness 和散落的 Release 测试迁移为 Unit、Functional、Detection、Performance、Distribution 五类测试，并用 Release 聚合发布门禁。

**Architecture:** 保留现有测试脚本行为、环境 harness 和数据契约，只调整实现归属、公共 Make 入口、活动文档和 CI 引用。旧 Make 目标作为单向兼容别名调用新目标；历史结果文档保持原样。

**Tech Stack:** GNU Make、Bash、Python unittest、GitHub Actions、Go test、Vagrant/libvirt、Docker。

## Global Constraints

- Unit 表示基础逻辑，Functional 表示产品功能，Detection 表示检测质量，Performance 表示资源成本，Distribution 表示发行包和安装兼容性。
- Release 只聚合测试门禁，不拥有或复制测试实现。
- Local、container、vm-endpoint 和 vm-topology 是运行环境，不是测试分类。
- 不建立通用矩阵编排器，不要求所有分类接受相同参数。
- 旧 Make 入口在迁移期调用新入口和同一套默认值。
- 历史结果文档不修改。
- 删除旧路径前必须确认全部活动引用已迁移。

---

### Task 1: 建立分类契约测试

**Files:**
- Create: `test/test_taxonomy_contract.py`

**Interfaces:**
- Consumes: `test/Makefile`、根 `Makefile`、`.github/workflows/release-build.yml` 和测试目录结构。
- Produces: 自动验证新分类目录、公共入口、兼容别名和 CI 路径的契约。

- [ ] **Step 1: 写入失败契约测试**

测试必须断言：

```python
expected_dirs = [
    "suites/functional/endpoint",
    "suites/functional/platform",
    "suites/functional/topology",
    "suites/detection/topology",
    "suites/distribution/package",
    "suites/distribution/published",
]
```

并断言 `test/Makefile` 包含：

```text
functional-endpoint
functional-platform
functional-topology
detection-topology
distribution-package
distribution-published
release-candidate
release-published
```

兼容目标 `product-*`、`effectiveness-topology` 必须依赖对应新目标，不得复制执行命令。发布工作流不得引用 `test/suites/product/` 或 `test/release/`。

- [ ] **Step 2: 运行测试并确认 RED**

Run:

```bash
python3 test/test_taxonomy_contract.py -v
```

Expected: FAIL，提示 `suites/functional`、`suites/detection` 或新 Make 目标不存在。

- [ ] **Step 3: 提交测试与后续实现一起完成**

该测试在 Task 2 和 Task 3 完成前保持失败，不单独提交红色工作树。

---

### Task 2: 迁移 Functional 与 Detection

**Files:**
- Move: `test/suites/product/endpoint` -> `test/suites/functional/endpoint`
- Move: `test/suites/product/platform` -> `test/suites/functional/platform`
- Move: `test/suites/product/topology` -> `test/suites/functional/topology`
- Move: `test/suites/effectiveness/topology` -> `test/suites/detection/topology`
- Modify: `test/Makefile`
- Modify: `test/contracts/agent-test-coverage.tsv`
- Modify: active references under `test/`, `docs/`, and root `Makefile`

**Interfaces:**
- Consumes: 现有 Product 和 Effectiveness 脚本，不改变脚本行为。
- Produces: `functional-*`、`detection-topology` 新入口及旧目标兼容别名。

- [ ] **Step 1: 使用 `git mv` 迁移目录**

```bash
git mv test/suites/product test/suites/functional
git mv test/suites/effectiveness test/suites/detection
```

- [ ] **Step 2: 更新脚本内部路径和活动引用**

所有活动代码、契约清单和非历史文档引用统一改为：

```text
test/suites/functional/...
test/suites/detection/...
```

检测结果内部标识和输出目录同步由 `effectiveness-*` 改为 `detection-*`；评分器文件名可在本任务保留，避免无关 Python API 重命名。

- [ ] **Step 3: 建立新 Make 入口和兼容别名**

新目标直接执行脚本：

```make
functional-endpoint: doctor
functional-platform:
functional-topology: doctor
detection-topology:
```

旧目标只依赖新目标并输出废弃提示，不包含第二份脚本命令：

```make
product-endpoint: functional-endpoint
effectiveness-topology: detection-topology
```

- [ ] **Step 4: 运行聚焦契约**

```bash
python3 test/test_taxonomy_contract.py -v
python3 test/suites/functional/endpoint/test_e2e_contract.py -v
python3 test/suites/functional/topology/test_e2e_contract.py -v
bash -n test/suites/functional/endpoint/*.sh test/suites/functional/platform/*.sh test/suites/functional/topology/*.sh test/suites/detection/topology/run.sh
```

Expected: 分类契约仅剩 Distribution/Release 相关失败，其余全部通过。

---

### Task 3: 收拢 Distribution 实现

**Files:**
- Create directory: `test/suites/distribution/package`
- Move: `test/release` -> `test/suites/distribution/published`
- Move selected package scripts from `test/suites/functional/endpoint/` to `test/suites/distribution/package/`
- Modify: `.github/workflows/release-build.yml`
- Modify: `test/Makefile`
- Modify: active references under `test/` and `docs/`

**Interfaces:**
- Consumes: 当前本地包契约和公开三镜像测试。
- Produces: `distribution-package` 与 `distribution-published` 两个唯一实现入口。

- [ ] **Step 1: 迁移 Package 脚本**

迁移以下文件：

```text
unified-agent-installation.sh
transactional-agent-installation.sh
standalone-release-package.sh
standalone-github-assets.sh
release-workflow-contract.sh
release-container-e2e-contract.sh
```

`standalone-local-store.sh`、`container-entrypoint.sh` 和真实 Agent 场景保留在 Functional Endpoint，因为它们主要验证运行行为。

- [ ] **Step 2: 迁移 Published 实现**

```bash
git mv test/release test/suites/distribution/published
```

历史结果 Markdown 移动到新目录但内容不修改。

- [ ] **Step 3: 修正相对根目录和引用**

迁移脚本必须继续从自身位置正确解析仓库根、测试根、fixtures 和 `.results`。发布工作流只调用 `test/suites/distribution/package/*.sh`。

- [ ] **Step 4: 建立 Distribution 入口**

```make
distribution-package:
	bash suites/distribution/package/unified-agent-installation.sh
	bash suites/distribution/package/transactional-agent-installation.sh
	bash suites/distribution/package/standalone-release-package.sh
	bash suites/distribution/package/standalone-github-assets.sh
	bash suites/distribution/package/release-workflow-contract.sh
	bash suites/distribution/package/release-container-e2e-contract.sh

distribution-published:
	$(MAKE) -C suites/distribution/published test
```

旧 `product-endpoint-standalone` 和 `product-endpoint-release-container` 作为兼容聚合入口调用新目标或对应 Functional 目标。

- [ ] **Step 5: 运行 Distribution 聚焦测试**

```bash
bash test/suites/distribution/package/standalone-release-package.sh
bash test/suites/distribution/package/release-workflow-contract.sh
bash test/suites/distribution/package/release-container-e2e-contract.sh
bash test/suites/distribution/published/test-assert.sh
python3 test/test_taxonomy_contract.py -v
```

Expected: PASS。

---

### Task 4: 建立 Release 门禁和公共入口

**Files:**
- Modify: root `Makefile`
- Modify: `test/Makefile`
- Modify: `.github/workflows/release-build.yml`

**Interfaces:**
- Consumes: Unit、Functional、Detection、Performance 和 Distribution 入口。
- Produces: `test-release-candidate` 和 `test-release-published`，不包含重复测试命令。

- [ ] **Step 1: 添加测试 Makefile 门禁**

```make
release-candidate: test-unit functional-endpoint functional-platform functional-topology detection-topology performance-modules distribution-package
release-published: distribution-published
```

`performance-modules` 是候选门禁中选定的低成本性能基线；长期 VM 性能仍由周期性门禁运行。

- [ ] **Step 2: 添加根 Makefile 公共入口**

根入口只委托 `test/Makefile`：

```make
test-functional-endpoint:
	$(MAKE) -C test functional-endpoint

test-functional-platform:
	$(MAKE) -C test functional-platform

test-functional-topology:
	$(MAKE) -C test functional-topology

test-detection:
	$(MAKE) -C test detection-topology

test-performance:
	$(MAKE) -C test performance-endpoint

test-distribution-package:
	$(MAKE) -C test distribution-package

test-distribution-published:
	$(MAKE) -C test distribution-published

test-release-candidate:
	$(MAKE) -C test release-candidate

test-release-published:
	$(MAKE) -C test release-published
```

- [ ] **Step 3: 验证 Make 调用图**

```bash
make -n test-functional-endpoint
make -n test-detection
make -n test-distribution-package
make -n test-release-candidate
```

Expected: 每个命令只展开新路径，Release 门禁通过依赖聚合现有入口。

---

### Task 5: 更新活动文档并完成验收

**Files:**
- Modify: `docs/development/testing.md`
- Modify: `docs/development/development.md`
- Modify: `docs/roadmap.md`
- Modify: `test/README.md`
- Modify: `test/data/README.md`
- Preserve without content changes: historical result Markdown under `test/.results/` and migrated Published `results/`

**Interfaces:**
- Consumes: 新目录和 Make 入口。
- Produces: 与实现一致的六类术语、命令示例和发布门禁说明。

- [ ] **Step 1: 更新活动文档**

测试指南使用 Unit、Functional、Detection、Performance、Distribution 和 Release；不再将 Product、Effectiveness 作为公开分类名称。

- [ ] **Step 2: 扫描活动旧引用**

```bash
rg -n 'suites/product|suites/effectiveness|test/release|product-|effectiveness-' .github Makefile test docs \
  --glob '!test/.results/**' \
  --glob '!test/suites/distribution/published/results/**' \
  --glob '!docs/superpowers/**'
```

Expected: 只剩兼容目标、兼容提示和必须描述旧名称的迁移说明。

- [ ] **Step 3: 运行静态和聚焦验收**

```bash
python3 test/test_taxonomy_contract.py -v
python3 test/suites/functional/endpoint/test_e2e_contract.py -v
python3 test/suites/functional/topology/test_e2e_contract.py -v
python3 test/shared/diagnostics/test_diagnostics_contract.py -v
go test ./...
git diff --check
```

Expected: PASS。

- [ ] **Step 4: 运行完整功能验收**

```bash
make -C test functional-endpoint
make -C test functional-topology
make -C test functional-platform
```

Expected: PASS。

- [ ] **Step 5: 清理生成物并确认状态**

删除 `bin/`、VM deploy 缓存和 `.results` 非 Markdown 生成物；保留历史 Markdown。`git status --short` 只显示计划内提交，最终工作树干净。

- [ ] **Step 6: 按关注点提交**

```text
test: rename product suites to functional and detection
test: group package tests under distribution
ci(test): define release test gates
docs(test): document unified test taxonomy
```
