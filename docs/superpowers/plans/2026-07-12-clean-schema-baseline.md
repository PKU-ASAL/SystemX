# Clean Schema Baseline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox syntax.

**Goal:** Replace unreleased compatibility history with one clean fresh-deployment schema baseline.

**Architecture:** Keep future migration and alias mechanisms, but reset their current state to PostgreSQL V1, a clean Incident v1 protobuf, mandatory DataBatch v1, and OpenSearch v1 aliases. Existing data volumes are explicitly destroyed through `make reset`.

**Tech Stack:** Go, Protobuf, PostgreSQL migrations, Kafka, OpenSearch, Docker Compose, Make.

## Global Constraints

- Fresh deployment only; no old data upgrade path.
- Preserve PKI during schema reset.
- Historical Superpowers decision records remain untouched.
- User-owned untracked UI documents remain untouched.

### Task 1: PostgreSQL Current V1

- [ ] Add failing tests requiring `PostgresVersion == 1`, one ordered migration, and absence of obsolete tables.
- [ ] Squash current schema into V1; delete V2/V3 and legacy cleanup SQL/tests.
- [ ] Run store migration/backend tests and commit `refactor: squash postgres control plane baseline`.

### Task 2: Current Protobuf And Kafka Contracts

- [ ] Add descriptor test for exact clean Incident field numbers and Worker test rejecting empty schema through DLQ.
- [ ] Rewrite Incident proto, regenerate code, remove lifecycle field accesses and legacy metric/code/docs.
- [ ] Run analytics, Worker, Store, Manager, and generated API tests; commit `refactor: reset telemetry schema baseline`.

### Task 3: Fresh OpenSearch And Development Reset

- [ ] Add shell assertions that initializer has no legacy-index branch and Make exposes explicit reset behavior.
- [ ] Remove old-index compatibility text/logic; add destructive `make reset` preserving PKI and using normal `up`/`status` flow.
- [ ] Validate Shell, Compose, initializer idempotence, and real alias lifecycle; commit `chore: add fresh platform schema reset`.

### Task 4: Final Verification

- [ ] Run `make api`, `go test ./...`, `go vet ./...`, Compose/Shell checks, OpenSearch lifecycle, and `git diff --check`.
- [ ] Scan active source/docs for deprecated Incident lifecycle, legacy schema metric, V2/V3 migration, unversioned-index compatibility, and obsolete cleanup artifacts.
- [ ] Perform constrained local code review and commit documentation cleanup if required.
