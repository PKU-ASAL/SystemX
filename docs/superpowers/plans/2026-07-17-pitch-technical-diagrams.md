# SysArmor Pitch Technical Diagrams Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an editable end-to-end technical roadmap and a correlation/root-cause method diagram to the investor pitch without changing the existing three business diagrams.

**Architecture:** Each diagram has one Draw.io source and one reviewed SVG delivery asset. The pitch references SVG, while visible labels and structure remain consistent with the Draw.io source. Solid styling represents implemented behavior; dashed styling plus explicit text represents planned or validating work.

**Tech Stack:** Draw.io XML, SVG 1.1, Markdown, `xmllint`, `rg`, Git.

## Global Constraints

- Preserve the existing `product-flow`, `defensibility`, and `business-flywheel` diagrams.
- Use solid borders and arrows for implemented mechanisms.
- Use dashed borders and arrows and the literal label `规划/验证中` for future mechanisms.
- Do not claim unimplemented algorithms or unverified performance and detection numbers.
- Keep Chinese Markdown prose one paragraph per source line.
- Keep the diagrams understandable without relying on color alone.

---

### Task 1: End-to-End Technical Roadmap

**Files:**
- Create: `docs/business/diagrams/technical-roadmap.drawio`
- Create: `docs/business/diagrams/technical-roadmap.svg`
- Modify: `docs/business/sysarmor-investor-pitch.zh-CN.md`

**Interfaces:**
- Consumes: the implemented runtime contracts in `docs/architecture/agent-runtime.md` and `docs/architecture/platform-runtime.md`.
- Produces: an SVG referenced after the technical barriers discussion and an editable Draw.io source with matching visible structure.

- [ ] **Step 1: Create the Draw.io source**

Create one landscape page with four horizontal bands: scientific/engineering problems; five numbered processing stages; implemented constraints and a dashed planned feedback branch; outputs and three validation systems. Use the exact stage titles `端点行为获取`, `本地实时判断与有界保存`, `可信传输与可靠交接`, `跨批次关联与证据投影`, and `查询与调查基础`.

- [ ] **Step 2: Validate the Draw.io XML**

Run:

```bash
xmllint --noout docs/business/diagrams/technical-roadmap.drawio
```

Expected: exit code 0 with no output.

- [ ] **Step 3: Create the matching SVG**

Create an accessible SVG with `title` and `desc`. Preserve the same stages, arrows, solid/dashed semantics, and explicit current product boundary as the Draw.io source. Use a stable `viewBox` so Markdown can scale the asset responsively.

- [ ] **Step 4: Add the Pitch reference**

After the research-direction table in section 4, add the `technical-roadmap.svg` image and one sentence explaining that solid lines are implemented and dashed lines are planned or validating.

- [ ] **Step 5: Validate the first deliverable**

Run:

```bash
xmllint --noout docs/business/diagrams/technical-roadmap.drawio docs/business/diagrams/technical-roadmap.svg
git diff --check
```

Expected: both commands exit 0 with no output; the Pitch contains one valid relative reference to `technical-roadmap.svg`.

### Task 2: Correlation And Root-Cause Method Diagram

**Files:**
- Create: `docs/business/diagrams/correlation-method.drawio`
- Create: `docs/business/diagrams/correlation-method.svg`
- Modify: `docs/business/sysarmor-investor-pitch.zh-CN.md`
- Modify: `docs/business/diagrams/README.md`

**Interfaces:**
- Consumes: the 15-minute scoped history behavior and deterministic projection contract in `internal/workers/ingest/processor.go`.
- Produces: an SVG referenced in section 5, an editable Draw.io source, and maintenance documentation covering both new asset pairs.

- [ ] **Step 1: Create the Draw.io source**

Create one landscape page with inputs on the left, five numbered method stages in the center, and verifiable outputs on the right. Include the staged example `Batch N -> Batch N+1 -> 历史窗口合并`, the analysis boundary `tenant + case_type/scenario/workload`, the endpoint Signal document identity `tenant + agent + signal`, and the implemented relationship `Incident -> contributing Signals + Evidence`.

- [ ] **Step 2: Mark method boundaries**

Use solid shapes for scope isolation, history merge, Signal/entity aggregation, rule-based Signal composition, stable projection, and structured Incident output. Put temporal reasoning, candidate path ranking, attack-stage inference, natural-language enhancement, and analyst-feedback learning in one dashed area labeled `规划/验证中`; show that feedback cannot mutate raw events or existing evidence.

- [ ] **Step 3: Create and validate the matching SVG**

Create an accessible SVG with matching labels and geometry, then run:

```bash
xmllint --noout docs/business/diagrams/correlation-method.drawio docs/business/diagrams/correlation-method.svg
```

Expected: exit code 0 with no output.

- [ ] **Step 4: Add the Pitch reference and maintenance inventory**

In section 5, add the `correlation-method.svg` image and explain that the method scopes by tenant and analysis labels (`case_type`, `scenario`, or `workload`) before merging current and historical data. State that complete raw-event traceability is planned rather than implemented. Update `docs/business/diagrams/README.md` to list all five diagram pairs and state that Draw.io and SVG visible labels must be reviewed together.

- [ ] **Step 5: Run repository-level documentation checks**

Run:

```bash
xmllint --noout docs/business/diagrams/*.drawio docs/business/diagrams/*.svg
rg -n "technical-roadmap.svg|correlation-method.svg" docs/business/sysarmor-investor-pitch.zh-CN.md
git diff --check
git status --short
```

Expected: XML validation and whitespace validation succeed; `rg` returns exactly two image references; Git lists only the two new Draw.io files, two new SVG files, the Pitch, and diagram README as implementation changes.

- [ ] **Step 6: Commit the implementation**

```bash
git add docs/business/diagrams/technical-roadmap.drawio docs/business/diagrams/technical-roadmap.svg docs/business/diagrams/correlation-method.drawio docs/business/diagrams/correlation-method.svg docs/business/diagrams/README.md docs/business/sysarmor-investor-pitch.zh-CN.md
git commit -m "docs: add SysArmor technical route diagrams"
```

Expected: one atomic documentation commit containing both complementary technical diagrams and their Pitch integration.
