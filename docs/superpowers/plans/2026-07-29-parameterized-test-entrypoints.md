# Parameterized Test Entrypoints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace root-level long test targets with validated category targets and domain-specific parameters, then make all active test documentation and automation use the same interface.

**Architecture:** The root `Makefile` is the only public dispatcher. It validates `DOMAIN`, `SOURCE`, or `STAGE` and delegates to concrete internal targets in `test/Makefile`; internal targets remain stable implementation details. A Python contract invokes Make in dry-run mode so routing and invalid-input behavior are verified without starting VMs or services.

**Tech Stack:** GNU Make, Python `unittest`, GitHub Actions YAML, Markdown, Bash-based existing suites.

## Global Constraints

- Public categories are exactly Unit, Functional, Detection, Performance, Distribution, and Release.
- Functional and Performance select with `DOMAIN`; Distribution selects with `SOURCE`; Release selects with `STAGE`.
- Missing or unsupported selectors fail with exit code 2 and print valid usage.
- Delete root long targets without compatibility aliases.
- Keep `test/Makefile` concrete targets as internal execution details.
- Do not modify historical Markdown under `test/.results/` or `test/suites/distribution/published/results/`.

---

### Task 1: Public Make Dispatcher

**Files:**
- Create: `test/test_make_entrypoints_contract.py`
- Modify: `Makefile`
- Modify: `test/test_taxonomy_contract.py`

**Interfaces:**
- Consumes: concrete targets in `test/Makefile`.
- Produces: `test-functional DOMAIN=...`, `test-performance DOMAIN=...`, `test-distribution SOURCE=...`, and `test-release STAGE=...`.

- [ ] **Step 1: Write failing dispatcher contracts**

Create a `unittest` module that runs `make -n --no-print-directory` at the repository root and asserts these mappings:

```python
VALID_CASES = {
    ("test-functional", "DOMAIN=endpoint"): "functional-endpoint",
    ("test-functional", "DOMAIN=platform"): "functional-platform",
    ("test-functional", "DOMAIN=topology"): "functional-topology",
    ("test-functional", "DOMAIN=all"): "functional-core",
    ("test-performance", "DOMAIN=endpoint"): "performance-endpoint",
    ("test-performance", "DOMAIN=platform"): "performance-platform",
    ("test-performance", "DOMAIN=modules"): "performance-modules",
    ("test-distribution", "SOURCE=local"): "distribution-package",
    ("test-distribution", "SOURCE=published"): "distribution-published",
    ("test-release", "STAGE=pre-publish"): "release-candidate",
    ("test-release", "STAGE=post-publish"): "release-published",
}
```

Assert missing and invalid selectors return 2 and include the exact valid values. Assert `make -n test-functional-endpoint` and the six other removed long forms fail with “No rule to make target”.

- [ ] **Step 2: Verify RED**

Run: `python3 test/test_make_entrypoints_contract.py -v`

Expected: failures because parameterized targets do not exist and long targets still resolve.

- [ ] **Step 3: Implement minimal root dispatcher**

Replace long `.PHONY` names and recipes with four category recipes. Each recipe uses a `case` statement and delegates to one concrete internal target. Example pattern:

```make
test-functional:
	@case "$(DOMAIN)" in \
		endpoint) target=functional-endpoint ;; \
		platform) target=functional-platform ;; \
		topology) target=functional-topology ;; \
		all) target=functional-core ;; \
		*) echo "usage: make test-functional DOMAIN=endpoint|platform|topology|all" >&2; exit 2 ;; \
	esac; \
	$(MAKE) -C test "$$target" SYSARMOR_TETRAGON_ARCHIVE="$(TETRAGON_ARCHIVE)"
```

Use equivalent explicit mappings for Performance, Distribution, and Release. For `DOMAIN=all`, Performance delegates once with all three internal targets. Preserve current benchmark variables and `URL` propagation.

- [ ] **Step 4: Extend taxonomy contract**

Change the root public target list to the six category targets and explicitly reject the deleted long forms. Keep concrete internal taxonomy assertions unchanged.

- [ ] **Step 5: Verify GREEN**

Run:

```bash
python3 test/test_make_entrypoints_contract.py -v
python3 test/test_taxonomy_contract.py -v
make -n test-functional DOMAIN=endpoint
make -n test-release STAGE=pre-publish
```

Expected: all tests pass; dry-run output delegates to the expected internal targets.

- [ ] **Step 6: Commit**

```bash
git add Makefile test/test_make_entrypoints_contract.py test/test_taxonomy_contract.py
git commit -m "refactor(test): add parameterized test dispatcher"
```

### Task 2: Documentation and Automation

**Files:**
- Modify: `Makefile`
- Modify: `test/Makefile`
- Modify: `.github/workflows/release-build.yml`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `test/README.md`
- Modify: `test/data/README.md`
- Modify: `test/suites/distribution/published/README.md`
- Modify: `docs/development/testing.md`
- Modify: `docs/development/development.md`
- Modify: `docs/operations/deployment.md`
- Modify: `docs/operations/maintenance.md`
- Modify: `docs/quickstart.md`

**Interfaces:**
- Consumes: Task 1 public commands.
- Produces: one documented command vocabulary for developers and CI.

- [ ] **Step 1: Add failing documentation assertions**

Extend `test/test_make_entrypoints_contract.py` to scan active Markdown, root help, test help, and release workflow. Reject the seven removed root commands and require examples for all four selector families:

```text
make test-functional DOMAIN=endpoint
make test-performance DOMAIN=endpoint
make test-distribution SOURCE=local
make test-release STAGE=pre-publish
```

Exclude design/plan documents and historical result directories.

- [ ] **Step 2: Verify RED**

Run: `python3 test/test_make_entrypoints_contract.py -v`

Expected: failures listing current help and Markdown references to removed commands.

- [ ] **Step 3: Update public help and documentation**

Make root `help` explain selector values and representative examples. Make `test/Makefile help` identify its concrete targets as internal and direct users to root commands. Replace active documentation commands with public parameterized calls; retain `make -C test ...` only where the document explicitly explains internal implementation or low-level diagnostics.

- [ ] **Step 4: Update release automation**

Replace direct public-gate calls in `.github/workflows/release-build.yml` with root dispatcher calls, for example `make test-distribution SOURCE=local`. Do not duplicate suite script lists in workflow YAML.

- [ ] **Step 5: Verify GREEN**

Run:

```bash
python3 test/test_make_entrypoints_contract.py -v
make help
make test-help
rg -n 'make test-functional-(endpoint|platform|topology)|make test-distribution-(package|published)|make test-release-(candidate|published)' . \
  --glob '!docs/superpowers/**' \
  --glob '!test/.results/**' \
  --glob '!test/suites/distribution/published/results/**'
```

Expected: contracts pass; search returns no active references.

- [ ] **Step 6: Commit**

```bash
git add Makefile test/Makefile .github/workflows/release-build.yml README.md README.zh-CN.md test/README.md test/data/README.md test/suites/distribution/published/README.md docs
git commit -m "docs(test): standardize parameterized test commands"
```

### Task 3: Test Tree Cleanup and Final Acceptance

**Files:**
- Delete when empty: `test/bin/`
- Delete when empty: `test/test/.results/`
- Modify only if contract evidence requires it: files under `test/`

**Interfaces:**
- Consumes: completed dispatcher and documentation contracts.
- Produces: a clean test tree with no generated non-Markdown results.

- [ ] **Step 1: Audit cleanup candidates**

Run:

```bash
find test -type d -empty -print
find test/.results -type f ! -name '*.md' -print
git status --short
```

Delete only confirmed empty directories and generated non-Markdown files. Preserve all historical Markdown.

- [ ] **Step 2: Run focused contracts**

```bash
python3 test/test_make_entrypoints_contract.py -v
python3 test/test_taxonomy_contract.py -v
python3 test/suites/functional/endpoint/test_e2e_contract.py -v
python3 test/suites/functional/topology/test_e2e_contract.py -v
bash -n test/suites/functional/endpoint/*.sh test/suites/functional/topology/*.sh
```

Expected: all pass.

- [ ] **Step 3: Run implementation acceptance**

```bash
go test ./...
make test-distribution SOURCE=local
make -n test-functional DOMAIN=all
make -n test-performance DOMAIN=all
make -n test-release STAGE=pre-publish
```

Expected: Go and local Distribution tests pass; all dry-run dispatches resolve without validation errors.

- [ ] **Step 4: Final consistency checks**

```bash
git diff --check
git status --short
```

Expected: no whitespace errors and only intended changes before commit.

- [ ] **Step 5: Commit cleanup if tracked changes exist**

```bash
git add test
git commit -m "chore(test): remove obsolete test artifacts"
```
