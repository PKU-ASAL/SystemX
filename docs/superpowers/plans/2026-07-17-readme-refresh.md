# README Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the repository front page with concise, accurate English and Chinese entry documents for users and contributors.

**Architecture:** `README.md` is the English source-facing entry point and `README.zh-CN.md` is its structural translation. Detailed system contracts remain in their existing owner documents and are linked rather than repeated.

**Tech Stack:** Markdown, Makefile targets, repository documentation links.

## Global Constraints

- Describe only repository-backed capabilities and commands.
- Keep both README files structurally equivalent.
- Do not claim production readiness or an unavailable license or policy.
- Do not modify application code or unrelated documentation.

---

### Task 1: Bilingual Repository Entry Points

**Files:**
- Modify: `README.md`
- Create: `README.zh-CN.md`

**Interfaces:**
- Consumes: current Makefile targets and existing architecture, deployment, and test documents.
- Produces: two equivalent repository entry points with valid local links and commands.

- [ ] **Step 1: Replace the English README**

Write the approved concise structure: status, capabilities, architecture,
prerequisites, quick start, development, documentation, contribution, and
license status.

- [ ] **Step 2: Add the matching Chinese README**

Translate the prose while preserving headings, links, and command examples.

- [ ] **Step 3: Verify commands, links, and structure**

Run:

```bash
rg -n '^## ' README.md README.zh-CN.md
rg -o '\[[^]]+\]\([^)]+\)' README.md README.zh-CN.md
git diff --check
```

Expected: both files have equivalent headings, all local links resolve, and
`git diff --check` reports no errors.

- [ ] **Step 4: Commit**

```bash
git add README.md README.zh-CN.md
git commit -m "docs: refresh bilingual project readme"
```

