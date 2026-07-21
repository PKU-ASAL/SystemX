# Formal Business DOCX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the SysArmor project proposal as an A4, Chinese public-document-style DOCX with a neutral cover, Chinese TOC, body page numbering, compatible diagram media, and repeatable validation.

**Architecture:** Pandoc parses the Markdown and applies a committed reference DOCX. A Lua filter changes build-only document semantics, while a Python standard-library tool edits DOCX ZIP/XML through structured APIs for styles, sections, footer fields, and validation. A shell orchestrator owns dependency checks, temporary files, image rasterization, and atomic output.

**Tech Stack:** Pandoc 2.9+, Lua filter API, Python 3 standard library (`zipfile`, `xml.etree.ElementTree`), FFmpeg/librsvg, OpenXML, Make, LibreOffice UNO for TOC generation and visual regression.

## Global Constraints

- Implement `docs/superpowers/specs/2026-07-22-business-docx-design.md` without changing proposal facts.
- Use a neutral cover with no organization, logo, document number, seal, or endorsement.
- Generate into ignored `dist/docs/`; never commit a generated delivery artifact.
- Rasterize the four technical diagrams to PNG for Office/WPS compatibility; do not modify source SVGs.
- Use structured ZIP/XML APIs for DOCX changes; do not regex-edit XML.
- Every missing dependency must produce a direct Chinese repair instruction.
- Keep each script below 500 lines and each function below 50 lines.

---

### Task 1: Add A Failing End-To-End DOCX Contract

**Files:**
- Create: `test/suites/docs/business-docx.sh`
- Modify: `Makefile`

- [x] **Step 1: Add a test target that invokes the not-yet-existing builder**

Add `test-business-docx` to `.PHONY` and implement:

```make
test-business-docx:
	bash test/suites/docs/business-docx.sh
```

The shell test must build to a temporary path, require a non-empty DOCX, run `unzip -t`, and delegate structural assertions to `tools/docs/validate-business-docx.sh`.

- [x] **Step 2: Run the test and confirm the expected failure**

Run:

```bash
make test-business-docx
```

Expected: FAIL because `make business-docx` or the build script does not exist.

### Task 2: Implement Reproducible OpenXML Formatting

**Files:**
- Create: `tools/docs/business_docx.py`
- Create: `docs/business/templates/project-proposal-reference.docx`

- [x] **Step 1: Implement the reference-template subcommand**

`business_docx.py reference --output PATH` must obtain Pandoc's default `reference.docx`, then modify ZIP entries with `zipfile` and XML elements with `ElementTree`.

It must define A4 page size and margins plus these styles: `Normal`, `BodyText`, `FirstParagraph`, `Title`, `Subtitle`, `Author`, `Date`, `Heading1` through `Heading4`, `TOCHeading`, `TOC1` through `TOC3`, `ImageCaption`, `TableCaption`, and `Table`.

- [x] **Step 2: Implement the finalize subcommand**

`business_docx.py finalize --input RAW --output FINAL` must:

```text
insert a next-page section after the cover
insert a next-page section after the TOC
configure the final body section as A4
start body page numbering at 1
add a centered footer with — PAGE —
set updateFields=true
repeat table headers and prevent row splitting
```

- [x] **Step 3: Implement the validate subcommand**

Validation must fail unless the package contains:

```text
valid DOCX ZIP and XML
A4 page dimensions and required margins
three document sections
footer relationship and PAGE field
TOC field and updateFields=true
four PNG media files and no SVG media
Chinese and Latin font declarations
no leaked YAML field names
```

- [x] **Step 4: Generate and parse-check the committed template**

Run:

```bash
python3 tools/docs/business_docx.py reference \
  --output docs/business/templates/project-proposal-reference.docx
unzip -t docs/business/templates/project-proposal-reference.docx
```

Expected: both commands succeed.

### Task 3: Implement Content Filtering And Build Orchestration

**Files:**
- Create: `tools/docs/business-docx.lua`
- Create: `tools/docs/build-business-docx.sh`
- Create: `tools/docs/validate-business-docx.sh`

- [x] **Step 1: Implement the Pandoc Lua filter**

The filter must:

```text
set the neutral cover title, material type, status, and date
remove the duplicate first H1 from the body
replace each diagram .svg target with its temporary .png target
set overview width to 120 mm and topic widths to 154 mm
number “图：” paragraphs as “图 N　” and apply Image Caption style
insert page breaks around the overview roadmap
```

- [x] **Step 2: Implement the build script**

`build-business-docx.sh SOURCE OUTPUT` must preflight `pandoc`, `ffmpeg`, `python3`, `zip`, `unzip`, and `xmllint`; create a trap-cleaned temporary directory; rasterize four images; add 300 DPI PNG metadata; run Pandoc with the reference and Lua filter; finalize and validate the DOCX; and atomically move it to the requested output.

- [x] **Step 3: Implement the validation wrapper**

`validate-business-docx.sh FILE` must provide Chinese errors and delegate XML/package validation to `business_docx.py validate`.

- [x] **Step 4: Run shell and Lua syntax checks**

Run:

```bash
bash -n tools/docs/build-business-docx.sh
bash -n tools/docs/validate-business-docx.sh
pandoc --lua-filter=tools/docs/business-docx.lua --version
```

Expected: syntax checks and Lua loading succeed.

### Task 4: Expose The Makefile User Interface

**Files:**
- Modify: `Makefile`

- [x] **Step 1: Add variables and build target**

Add:

```make
BUSINESS_DOCX_SOURCE ?= docs/business/sysarmor-project-proposal.zh-CN.md
BUSINESS_DOCX_OUTPUT ?= dist/docs/sysarmor-project-proposal.zh-CN.docx

business-docx:
	bash tools/docs/build-business-docx.sh "$(BUSINESS_DOCX_SOURCE)" "$(BUSINESS_DOCX_OUTPUT)"
```

- [x] **Step 2: Add concise help output**

The root help must show the build command, output path, and `test-business-docx` validation target.

- [x] **Step 3: Run the end-to-end contract**

Run:

```bash
make test-business-docx
```

Expected: PASS with the temporary output removed after the test.

### Task 5: Build And Visually Validate The Formal Deliverable

**Files:**
- Generate, do not commit: `dist/docs/sysarmor-project-proposal.zh-CN.docx`

- [x] **Step 1: Build the real delivery file**

Run:

```bash
make business-docx
```

Expected: a validated DOCX at the default output path.

- [x] **Step 2: Convert the DOCX to PDF with LibreOffice**

Use an isolated LibreOffice user profile and write the PDF under `/tmp/sysarmor-business-docx-review/`.

Expected: PDF pages are A4 and all four diagrams render.

- [x] **Step 3: Update the TOC and visually inspect representative pages**

Render the cover, TOC, first body page, four figure pages, a table-heavy page, and the final page to PNG. Check page boundaries, fonts, figure sharpness, captions, tables, headings, and `— N —` footers.

- [x] **Step 4: Run final repository checks**

Run:

```bash
make test-business-docx
git diff --check
git status --short
```

Expected: tests pass; only intended source/template changes remain; `dist/docs/*.docx` stays ignored.

- [x] **Step 5: Request independent review and commit**

After review finds no Critical or Important issue:

```bash
git add Makefile docs/business/templates test/suites/docs tools/docs \
  docs/superpowers/plans/2026-07-22-business-docx.md
git commit -m "feat: build formal business proposal docx"
```
