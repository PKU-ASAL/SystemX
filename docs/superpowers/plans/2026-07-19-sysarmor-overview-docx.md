# SysArmor Overview DOCX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a first-principles Chinese SysArmor overview as reviewable Markdown and an editable, rendered DOCX with the approved diagrams.

**Architecture:** Markdown is the factual source; Pandoc converts it to DOCX using explicit page breaks and image sizing. LibreOffice headless converts the DOCX to a temporary PDF for page-count and rendering validation.

**Tech Stack:** Markdown, Pandoc, DOCX/OpenXML, LibreOffice, SVG, `unzip`, `pdfinfo`, Git.

## Global Constraints

- Target 12–15正文 pages plus 2–3 appendix pages.
- Use plain Chinese and introduce technical terms only when needed.
- Distinguish implemented, productizing, and planned capabilities.
- Do not include a financing amount, unsupported maturity claims, or an existing open-source license claim.
- Use B diagrams in the main narrative and A diagrams in appendices.
- Preserve the user's unrelated uncommitted Pitch change.

---

### Task 1: Write The Reviewable Source

**Files:**
- Create: `docs/business/sysarmor-overview.zh-CN.md`

**Interfaces:**
- Consumes: `docs/superpowers/specs/2026-07-19-sysarmor-overview-docx-design.md`, current architecture docs, test docs, and approved business facts.
- Produces: a standalone Markdown document with 14正文 sections, four appendices, and valid relative image paths.

- [ ] **Step 1: Write the first-principles narrative**

Write sections in the approved order. Each section starts with a plain-language conclusion, then supplies only the facts needed to support it. Use the five necessary system conditions as the logical spine.

- [ ] **Step 2: Add figures and captions**

Use `product-flow.svg`, `technical-roadmap-b-narrative.svg`, and `correlation-method-b-lecture.svg` in the main text. Use `technical-roadmap-a-nsfc.svg` and `correlation-method-a-derivation.svg` in appendices. Add a one-sentence interpretation and current/planned boundary after each technical figure.

- [ ] **Step 3: Validate source facts and links**

Run `rg` for `TODO|TBD|融资金额|已开源` and verify every `diagrams/*.svg` path exists. Expected: no placeholder or license overclaim; all images exist.

### Task 2: Generate And Render The DOCX

**Files:**
- Create: `docs/business/sysarmor-overview.zh-CN.docx`
- Temporary: `/tmp/sysarmor-overview.pdf`

**Interfaces:**
- Consumes: Task 1 Markdown and the five SVG assets.
- Produces: one editable DOCX with embedded media and a temporary PDF rendering for inspection.

- [ ] **Step 1: Generate DOCX with Pandoc**

Run Pandoc from `docs/business` so relative image paths resolve. Enable table of contents, numbered sections, standalone metadata, and page breaks encoded in the Markdown source.

- [ ] **Step 2: Inspect the DOCX package**

Run `unzip -t` and list `word/media/`. Expected: a valid ZIP package containing all five referenced figures.

- [ ] **Step 3: Render through LibreOffice**

Convert the DOCX to PDF in `/tmp`. Run `pdfinfo` to verify page count and inspect LibreOffice stderr for import/render errors. Target: 14–18 pages.

- [ ] **Step 4: Correct layout defects**

If page count, figure placement, or headings are poor, adjust page breaks or image widths in the Markdown and regenerate. Do not remove factual caveats to shorten the document.

### Task 3: Review And Commit

**Files:**
- Review: `docs/business/sysarmor-overview.zh-CN.md`
- Review: `docs/business/sysarmor-overview.zh-CN.docx`

- [ ] **Step 1: Run final validations**

Run `git diff --check`, confirm Markdown image paths, validate DOCX ZIP integrity, and confirm the temporary PDF page count. Check that current/planned wording matches the two B diagrams.

- [ ] **Step 2: Independent content review**

Review for jargon density, unsupported claims, duplicated sections, and whether a reader can summarize SysArmor without knowing its component names.

- [ ] **Step 3: Commit only overview deliverables**

Stage the Markdown and DOCX only. Do not stage the user's existing modification to `sysarmor-investor-pitch.zh-CN.md`.

```bash
git add docs/business/sysarmor-overview.zh-CN.md docs/business/sysarmor-overview.zh-CN.docx
git commit -m "docs: add first-principles SysArmor overview"
```
