# NSFC Technical Roadmaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the existing business diagram assets and add one overview plus three detailed, NSFC-style Draw.io technical roadmaps to the project proposal.

**Architecture:** Each figure has an editable, uncompressed Draw.io XML source and a matching accessible SVG delivery asset. The overview carries the complete problem-content-method-validation argument; the three topic figures expand dynamic strategy, information-efficiency balance, and endpoint-cloud collaboration without repeating the overview verbatim.

**Tech Stack:** Draw.io `mxGraphModel`, SVG 1.1, Markdown, FFmpeg/librsvg screenshot rendering, XML and repository consistency checks.

## Global Constraints

- Follow `docs/superpowers/specs/2026-07-21-nsfc-technical-roadmaps-design.md` exactly.
- Restore every pre-existing file under `docs/business/diagrams/`; do not remove old assets in this task.
- Use Chinese business and research language in visible diagram text; reserve engineering English terms for existing technical references outside the new figures.
- Preserve the current/planned capability boundaries from `docs/design-principles.zh-CN.md` and `docs/architecture.md`.
- Use white backgrounds, orthogonal connectors, small or zero corner radii, no gradients, no shadows, and no decorative icons.
- Keep each Draw.io/SVG pair visually equivalent and include SVG `title`, `desc`, and `viewBox`.

---

### Task 1: Restore And Inventory Existing Diagram Assets

**Files:**
- Restore: `docs/business/diagrams/README.md`
- Restore: `docs/business/diagrams/*.drawio`
- Restore: `docs/business/diagrams/*.svg`
- Modify: `docs/business/diagrams/README.md`

- [x] **Step 1: Restore the directory from the current branch baseline**

Run:

```bash
git restore --source=HEAD --staged --worktree docs/business/diagrams
```

Expected: all five existing Draw.io/SVG pairs and the inventory README return; diagram deletions disappear from `git status`.

- [x] **Step 2: Record the four new asset pairs in the inventory**

Add rows for:

```text
technical-roadmap-nsfc-overview.drawio/.svg
dynamic-game-policy-loop.drawio/.svg
efficiency-balance-information-refinement.drawio/.svg
endpoint-cloud-collaboration.drawio/.svg
```

Expected: the README distinguishes editable sources from Markdown delivery assets and states that the new series serves the project proposal.

### Task 2: Draw The Overall Technical Roadmap

**Files:**
- Create: `docs/business/diagrams/technical-roadmap-nsfc-overview.drawio`
- Create: `docs/business/diagrams/technical-roadmap-nsfc-overview.svg`

- [x] **Step 1: Build the portrait Draw.io source**

Use a 1200 × 1800 page with these fixed regions:

```text
Title and overall objective: y=40..180
Column headers: y=220..300
Four research stages: y=330..1320
Validation region: y=1380..1560
Outcome and maturity legend: y=1610..1760
```

Place critical questions at x=50..260, research content at x=310..880, and research methods at x=930..1150. Use four equally spaced research-stage groups and orthogonal arrows between their center boxes.

- [x] **Step 2: Build the matching accessible SVG**

Use the same 1200 × 1800 coordinate system. Include the four problem statements, four research contents with sub-tasks, four method groups, three verification boxes, final outcome, and maturity legend. Ensure no unsupported current capability is rendered as complete.

- [x] **Step 3: Validate source structure**

Run:

```bash
xmllint --noout docs/business/diagrams/technical-roadmap-nsfc-overview.drawio
xmllint --noout docs/business/diagrams/technical-roadmap-nsfc-overview.svg
```

Expected: both commands exit successfully.

### Task 3: Draw The Dynamic Strategy Loop

**Files:**
- Create: `docs/business/diagrams/dynamic-game-policy-loop.drawio`
- Create: `docs/business/diagrams/dynamic-game-policy-loop.svg`

- [x] **Step 1: Build the 1400 × 900 Draw.io source**

Place the strategy lifecycle across the top and center: threat change, strategy intent, validation, endpoint application, status feedback, and next-round adjustment. Put the unified strategy at the center with four surrounding control areas: observation, detection, transmission, and response.

- [x] **Step 2: Add boundaries and maturity labels**

Create a right-side security constraint region for scope, resource budget, version, approval, acknowledgement, and audit. Add a bottom boundary statement separating existing policy-control foundations from automatic deepening, automatic recovery, and real blocking research.

- [x] **Step 3: Produce and parse-check the matching SVG**

Expected: Draw.io and SVG use identical visible labels and connector semantics; XML parsing succeeds.

### Task 4: Draw The Information-Efficiency Balance

**Files:**
- Create: `docs/business/diagrams/efficiency-balance-information-refinement.drawio`
- Create: `docs/business/diagrams/efficiency-balance-information-refinement.svg`

- [x] **Step 1: Build the 1400 × 900 refinement chain**

Create four main stages from left to right: behavior records, behavior clues, investigation evidence, and comprehensive assessment. Add an upper arrow for increasing information value and a lower arrow for decreasing data volume.

- [x] **Step 2: Add per-stage semantics and feedback**

For each stage, show retained content, processing location, and resource constraint. Add a bottom feedback path for authorized, time-limited observation when material is insufficient. State that behavior clues are not alerts and that real raw-material pullback remains a research target.

- [x] **Step 3: Produce and parse-check the matching SVG**

Expected: the main flow reads without engineering English terms, feedback does not cross node text, and XML parsing succeeds.

### Task 5: Draw Endpoint-Cloud Collaboration

**Files:**
- Create: `docs/business/diagrams/endpoint-cloud-collaboration.drawio`
- Create: `docs/business/diagrams/endpoint-cloud-collaboration.svg`

- [x] **Step 1: Build the 1400 × 900 dual-domain structure**

Use a left endpoint domain, central controlled channel, and right management-platform domain. The endpoint contains observation, local filtering, preliminary judgment, bounded storage, and offline continuity. The platform contains identity validation, scoped historical correlation, entity-relationship analysis, centralized assessment, and unified query.

- [x] **Step 2: Add directional and operational loops**

Use an upper endpoint-to-platform arrow for policy-authorized high-value information and a lower platform-to-endpoint arrow for versioned policy and controlled commands. Add the investigation, authorized response, and strategy optimization operational loop plus the three validation categories.

- [x] **Step 3: Produce and parse-check the matching SVG**

Expected: the figure makes endpoint autonomy and platform scope limits explicit; XML parsing succeeds.

### Task 6: Integrate And Visually Verify The Series

**Files:**
- Modify: `docs/business/sysarmor-project-proposal.zh-CN.md`
- Modify: `docs/business/diagrams/README.md`
- Modify: `CATALOG.md`

- [x] **Step 1: Replace duplicate Mermaid blocks and add all four figures**

Place the dynamic strategy figure after section 4.3, endpoint-cloud figure in section 6.1, information-efficiency figure in section 6.2, and overview figure in section 6.3. Use concise Chinese captions and relative SVG links.

- [x] **Step 2: Render each SVG to PNG with FFmpeg/librsvg**

Render screenshots at each SVG's native viewport. Save temporary PNGs under `/tmp/sysarmor-roadmap-review/`; do not add them to Git.

Expected: four non-empty screenshots with complete outer borders and readable Chinese labels.

- [x] **Step 3: Inspect every screenshot**

Check title hierarchy, node alignment, connector routing, visible legend, edge margins, text wrapping, and current/target boundaries. Correct both the Draw.io and SVG assets for every visual issue.

- [x] **Step 4: Run final repository checks**

Run:

```bash
git diff --check
rg -n '```mermaid' docs/business/sysarmor-project-proposal.zh-CN.md
xmllint --noout docs/business/diagrams/*.drawio docs/business/diagrams/*.svg
```

Expected: no diff errors, no proposal Mermaid blocks, and all XML files parse.

- [x] **Step 5: Commit the implementation as one documentation change**

```bash
git add CATALOG.md docs/business docs/superpowers/plans/2026-07-21-nsfc-technical-roadmaps.md
git commit -m "docs: add project proposal and technical roadmaps"
```

Expected: the implementation commit contains the restored inventory, four new asset pairs, proposal integration, and this plan; unrelated files are not included.
