# Production Identity And Index Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete production-ready JWT key sourcing, remove obsolete database authorization, provision OpenSearch aliases safely, and establish observable schema retirement.

**Architecture:** Manager selects exactly one JWT verifier source at startup: local RS256 PEM or generic OIDC Discovery/JWKS. OpenSearch desired state is owned by versioned mappings and one idempotent initializer that gates application startup. Authorization remains Principal-only; legacy schema support remains temporarily observable.

**Tech Stack:** Go, RS256 JWT, OIDC Discovery, JWKS, PostgreSQL migrations, OpenSearch, Docker Compose, Bash.

## Global Constraints

- SysArmor never stores users, passwords, sessions, refresh tokens, or identity-provider state.
- `SYSARMOR_AUTH_MODE` is required and is exactly `local` or `oidc`.
- There is no disabled authentication mode or bundled identity provider.
- OIDC requires `kid`, RS256, bounded HTTP, exact issuer, audience, expiry, tenant, and recognized roles.
- OpenSearch initialization is idempotent and refuses incompatible or legacy unversioned state.
- Empty data-plane schema remains accepted in this release.
- Do not modify the user-owned untracked Manager UI documents or VM deployment snapshot.

---

### Task 1: Generic OIDC/JWKS Verifier And Explicit Auth Modes

**Files:**
- Create: `internal/manager/auth/oidc.go`
- Create: `internal/manager/auth/oidc_test.go`
- Create: `internal/manager/auth/config.go`
- Create: `internal/manager/auth/config_test.go`
- Modify: `internal/manager/auth/jwt.go`
- Modify: `cmd/sysarmor-manager/main.go`
- Modify: deployment environment files and authentication documentation

**Interfaces:**
- Produces: `auth.Config{Mode, PublicKeyFile, IssuerURL, Issuer, Audience}`.
- Produces: `auth.NewVerifier(context.Context, Config) (*Verifier, error)`.
- Produces: a JWKS key provider used by the existing Principal middleware.

- [ ] Write in-memory OIDC tests for discovery, exact issuer, JWKS cache reuse, unknown-`kid` refresh, key rotation, outage, response-size limits, RS256 enforcement, and loopback HTTP allowance.
- [ ] Run `go test ./internal/manager/auth -run 'Test(OIDC|AuthConfig)' -count=1` and verify RED.
- [ ] Refactor `Verifier` to resolve RSA keys by token `kid` while retaining a single-key local provider; implement bounded Discovery/JWKS loading and refresh.
- [ ] Implement strict mode configuration and replace direct PEM construction in Manager main.
- [ ] Run Manager auth, API, command, and full focused tests; verify invalid or conflicting modes fail startup.
- [ ] Commit `feat: support oidc jwks authentication`.

### Task 2: Minimal Local JWT Issuer

**Files:**
- Modify: `cmd/sysarmorctl/main.go`
- Modify: `cmd/sysarmorctl/main_test.go`
- Modify: local deployment documentation

**Interfaces:**
- Produces: `sysarmorctl auth token --private-key ... --subject ... --tenant ... --roles ... --issuer ... --audience ... --ttl ...`.

- [ ] Write tests for valid RS256 JWT, missing fields, unrecognized roles, invalid key, non-positive TTL, and TTL above 24 hours.
- [ ] Run focused command tests and verify RED.
- [ ] Implement only token signing and stdout output; never generate or persist keys.
- [ ] Verify issued tokens with the local Manager verifier.
- [ ] Commit `feat: issue local development jwt`.

### Task 3: Remove OperatorRoleBinding

**Files:**
- Modify: `internal/store/store.go` and tests
- Modify: `internal/store/postgres/snapshot.go` and tests
- Modify: `internal/manager/api/http.go`, `http_identity.go`, and tests
- Modify: `cmd/sysarmorctl/main.go` and tests
- Modify: `internal/store/migrations/postgres.go` and migration tests
- Modify: active API and architecture documentation

**Interfaces:**
- Produces: ordered PostgreSQL migration v3 `drop_operator_role_bindings`.

- [ ] Write migration tests proving a fresh schema omits the table and an upgraded schema drops it transactionally and idempotently.
- [ ] Write authenticated route and CLI tests proving the removed surface is unavailable.
- [ ] Run focused tests and verify RED.
- [ ] Delete the API, CLI, Store state/type/methods, PostgreSQL projection, and obsolete tests; add migration v3.
- [ ] Scan production source and active docs for `OperatorRoleBinding` and `operator-role-bindings`.
- [ ] Commit `refactor: remove operator role bindings`.

### Task 4: OpenSearch Desired State And Alias Lifecycle

**Files:**
- Create: `deployments/opensearch/init.sh`
- Create: `deployments/opensearch/mappings/events-v1.json`
- Create: corresponding signal, incident, and evidence v1 mappings
- Create: `test/suites/product/platform/opensearch-alias-lifecycle.sh`
- Modify: `deployments/compose.platform.yaml`
- Modify: `test/environments/container/compose.yaml`
- Modify: `test/shared/harness/start-container.sh`
- Modify: `Makefile`
- Modify: OpenSearch operational documentation

**Interfaces:**
- Produces: idempotent `init.sh` configured by `SYSARMOR_OPENSEARCH_URL`, optional credentials, and bounded readiness settings.
- Produces: one-shot Compose service `opensearch-init` required by Manager and Worker.
- Produces: `make test-opensearch-lifecycle`.

- [ ] Write shell contract tests using a fake curl executable for fresh, idempotent, incompatible-alias, and legacy-index cases; verify RED.
- [ ] Add minimal reviewed mappings and initializer; run `bash -n` and fake-server tests.
- [ ] Gate Compose services on successful one-shot initialization and validate Compose configuration.
- [ ] Add real-container v1/v2 reindex, atomic cutover, new-write, rollback, and cleanup test.
- [ ] Run the real lifecycle test when Docker/OpenSearch is available.
- [ ] Commit `feat: provision opensearch aliases before startup`.

### Task 5: Legacy Schema Retirement Gate And Final Verification

**Files:**
- Modify: `internal/contracts/schema/schema_test.go`
- Modify: `internal/workers/ingest/worker_test.go`
- Modify: `docs/architecture/schema-evolution.md`
- Create: `docs/operations/data-plane-schema-retirement.md`
- Modify: release/deployment documentation

**Interfaces:**
- Preserves: empty schema acceptance and `legacy_data_batches` metric.
- Produces: exact operational retirement checklist and next-release test change.

- [ ] Add tests documenting current/legacy/unknown compatibility as a table and proving legacy counting is per consumed source message.
- [ ] Document Kafka-retention-plus-safety-window query, restart requirement, release note, rollback, and next-release code change.
- [ ] Run `make api`, `go test ./...`, `go vet ./...`, shell syntax checks, Compose config, and `git diff --check`.
- [ ] Scan for identity storage, authentication bypasses, obsolete role binding, and direct OpenSearch index access.
- [ ] Request final code review or perform the constrained local review fallback.
- [ ] Commit `docs: define data plane schema retirement gate`.
