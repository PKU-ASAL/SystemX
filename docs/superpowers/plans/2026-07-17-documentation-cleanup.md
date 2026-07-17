# Documentation Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every maintained SysArmor document concise, current, and owned by one clear part of the documentation hierarchy.

**Architecture:** Root documentation routes readers to architecture, operations, and testing. Formal documents absorb durable decisions; completed plans, absorbed designs, and repeated subsystem explanations are removed.

**Tech Stack:** Markdown, Go source and protobuf contracts, Makefiles, shell deployment assets.

## Global Constraints

- Describe current repository behavior only.
- Preserve unique operational or architectural knowledge before deleting history.
- Keep commands and examples executable against current paths and schemas.
- Prefer links to duplication and explanations to mechanical inventories.

---

### Task 1: Audit And Ownership Map

**Files:** All tracked `*.md` files.

- [ ] Inventory headings, links, commands, legacy paths, and duplicated topics.
- [ ] Map each durable topic to one root, architecture, operations, or test owner.
- [ ] Identify historical files whose decisions already have a formal owner.

### Task 2: Formal Documentation

**Files:**
- Modify: `docs/architecture/*.md`
- Modify: `deployments/README.md`
- Modify: `docs/operations/*.md`
- Modify: `test/README.md`
- Modify: `test/DETAILS.md`

- [ ] Rewrite documents around reader questions and current contracts.
- [ ] Move unique Agent, identity, data reliability, UI API, and schema decisions into formal owners.
- [ ] Remove completed phases, repeated commands, and obsolete migration history.

### Task 3: Subsystem Documentation And History

**Files:** All remaining component README files and `docs/superpowers/`.

- [ ] Keep only directory-specific setup, inputs, outputs, and limits.
- [ ] Delete component documents fully covered by a formal parent document.
- [ ] Delete completed plans and absorbed design documents.

### Task 4: Repository Verification

- [ ] Resolve every relative Markdown link.
- [ ] Verify every documented Make target and repository path.
- [ ] Scan for removed paths, packages, configuration keys, and planning language.
- [ ] Run `git diff --check` and review the final documentation tree.

