# Technical Diagram Layouts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the two generic technical diagrams with four purpose-specific A/B layouts and make the two B versions the default Pitch illustrations.

**Architecture:** A versions use structured academic layouts; B versions use compact narrative layouts. All four SVGs are delivery assets paired with editable Draw.io sources and share the same current/planned technical boundary.

**Tech Stack:** Draw.io XML, SVG 1.1, Markdown, `xmllint`, `rg`, Git.

## Global Constraints

- A versions serve project proposals and technical due diligence.
- B versions serve the investor Pitch and lecture-style explanation.
- Solid lines represent implemented mechanisms; dashed lines plus `规划/验证中` represent planned capabilities.
- Endpoint Signal identity is `tenant + agent + signal`; history analysis scope is `tenant + case_type/scenario/workload` plus a 15-minute window.
- Do not claim unimplemented path inference, natural-language explanation, complete raw-event traceability, or complete Incident UI as current behavior.
- The Pitch references only the two B SVGs; the old unsuffixed diagrams are removed.

---

### Task 1: Create Four Diagram Pairs

**Files:**
- Create: `docs/business/diagrams/technical-roadmap-a-nsfc.drawio` and `.svg`
- Create: `docs/business/diagrams/technical-roadmap-b-narrative.drawio` and `.svg`
- Create: `docs/business/diagrams/correlation-method-a-derivation.drawio` and `.svg`
- Create: `docs/business/diagrams/correlation-method-b-lecture.drawio` and `.svg`
- Delete: `docs/business/diagrams/technical-roadmap.drawio` and `.svg`
- Delete: `docs/business/diagrams/correlation-method.drawio` and `.svg`

**Interfaces:**
- Consumes: `docs/superpowers/specs/2026-07-18-technical-diagram-layouts-design.md` and current runtime/worker contracts.
- Produces: four editable/delivery pairs with matching visible structure and no feedback path crossing a node or label.

- [ ] **Step 1: Build the A versions**

Use vertical sequences. The roadmap A sequence is `研究对象与边界 → 三个关键问题 → 统一研究目标 → 三个研究任务 → 输出与验证`; the method A sequence is `输入定义 → 作用域与窗口 → Signal 视图 V → 规则判定 F(V, Policy) → 稳定投影 Π → Incident + Evidence`. Put boundaries beside each step and keep planned extensions in one bottom dashed region.

- [ ] **Step 2: Build the B versions**

Use a single narrative spine. The roadmap B spine is `可信行为 → 局部 Signal → 跨批关联 → 结构化 Incident → 可验证结论`; the method B headline is `Current ∪ History --Scope(tenant, labels, 15 min)--> Signal View --Rules + Converge--> Stable Incident`. Use one staged example and short side annotations instead of a grid of component cards.

- [ ] **Step 3: Validate all XML and labels**

Run:

```bash
xmllint --noout docs/business/diagrams/technical-roadmap-a-nsfc.drawio docs/business/diagrams/technical-roadmap-a-nsfc.svg docs/business/diagrams/technical-roadmap-b-narrative.drawio docs/business/diagrams/technical-roadmap-b-narrative.svg docs/business/diagrams/correlation-method-a-derivation.drawio docs/business/diagrams/correlation-method-a-derivation.svg docs/business/diagrams/correlation-method-b-lecture.drawio docs/business/diagrams/correlation-method-b-lecture.svg
git diff --check
```

Expected: all commands exit 0 with no output.

### Task 2: Integrate Pitch And Diagram Inventory

**Files:**
- Modify: `docs/business/sysarmor-investor-pitch.zh-CN.md`
- Modify: `docs/business/diagrams/README.md`

**Interfaces:**
- Consumes: the two B SVGs from Task 1.
- Produces: Pitch references to `technical-roadmap-b-narrative.svg` and `correlation-method-b-lecture.svg`; README inventory for all four pairs and their audiences.

- [ ] **Step 1: Replace Pitch references**

Replace the two current technical diagram references with the B filenames. Keep one explanatory sentence per image stating that A versions are available for formal proposal and due diligence, while B versions are used for the Pitch narrative.

- [ ] **Step 2: Rewrite diagram inventory**

List the five remaining diagram families: product flow, defensibility, business flywheel, technical roadmap A/B, and correlation method A/B. Explain that each A/B pair shares facts but uses a different reading structure.

- [ ] **Step 3: Verify references and stale assets**

Run:

```bash
rg -n "technical-roadmap-b-narrative.svg|correlation-method-b-lecture.svg" docs/business/sysarmor-investor-pitch.zh-CN.md
! rg -n "technical-roadmap.svg|correlation-method.svg" docs/business/sysarmor/sysarmor-investor-pitch.zh-CN.md docs/business/diagrams/README.md
test ! -e docs/business/diagrams/technical-roadmap.svg
test ! -e docs/business/diagrams/correlation-method.svg
```

Expected: exactly two B references, no stale unsuffixed references, and no old unsuffixed files.

- [ ] **Step 4: Commit the layout migration**

```bash
git add docs/business/diagrams docs/business/sysarmor-investor-pitch.zh-CN.md
git commit -m "docs: add dual technical diagram layouts"
```

Expected: one atomic documentation commit containing the four pairs, old asset removal, inventory updates, and Pitch integration.
