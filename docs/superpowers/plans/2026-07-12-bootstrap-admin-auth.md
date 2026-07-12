# Bootstrap Admin Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver one bootstrap-admin browser login with no auth database, an Auth.js cookie session, a JWT-signing Next.js BFF, and a Manager that trusts only that internal JWT.

**Architecture:** Auth.js validates deployment-mounted credentials and owns the browser session. Server-only BFF routes translate that session into a five-minute RS256 JWT for Manager; Manager validates one configured issuer and creates the existing request-scoped `Principal`. Agent mTLS remains unchanged and no user/session schema is added.

**Tech Stack:** Next.js 16, Auth.js, React 19, TypeScript, Vitest, Go 1.24, `golang-jwt/jwt/v5`, Docker Compose, pnpm.

## Global Constraints

- The only interactive identity is `bootstrap-admin`, tenant `default`, role `admin`.
- Do not create `users`, `identities`, `accounts`, or `sessions` tables.
- Browser code must never receive Manager JWTs, JWT private keys, Manager internal origin, or bootstrap credentials.
- Manager authorization consumes only `Principal`; `/healthz` stays anonymous and `/api/` stays fail closed.
- Agent authentication remains mTLS.
- New production functions stay below 50 lines and focused files stay below 500 lines.
- Every behavior change follows red-green-refactor and each task ends in an atomic Conventional Commit.

---

## File Structure

- `web/manager/auth.ts`: Auth.js configuration and session identity mapping.
- `web/manager/lib/auth/bootstrap-credentials.ts`: secret-file loading and constant-time credential validation.
- `web/manager/lib/auth/login-limiter.ts`: bounded single-process login attempt limiter.
- `web/manager/lib/auth/session-identity.ts`: fixed bootstrap identity and credential-version checks.
- `web/manager/lib/auth/manager-token.ts`: server-only RS256 Manager JWT signing.
- `web/manager/lib/api/manager-proxy.ts`: allowlisted BFF forwarding and error normalization.
- `web/manager/app/login/page.tsx`: bootstrap login form.
- `web/manager/app/api/manager/[...path]/route.ts`: authenticated BFF endpoint.
- `web/manager/proxy.ts`: route-level optimistic session protection; server components and BFF recheck authorization.
- `internal/manager/auth/config.go`: single static-key internal issuer configuration.
- `internal/manager/api/http_error.go`: Manager JSON error envelope.
- `tools/auth/init-bootstrap-admin.sh`: idempotent local Secret generation.
- `tools/doctor.sh`: deployment and authenticated service checks.

### Task 1: Bootstrap Credential Boundary

**Files:**
- Create: `web/manager/lib/auth/bootstrap-credentials.ts`
- Create: `web/manager/lib/auth/bootstrap-credentials.test.ts`
- Create: `web/manager/lib/auth/login-limiter.ts`
- Create: `web/manager/lib/auth/login-limiter.test.ts`

**Interfaces:**
- Produces: `loadBootstrapCredentials(env?): BootstrapCredentials`
- Produces: `verifyBootstrapCredentials(input, expected): boolean`
- Produces: `credentialVersion(credentials): string`
- Produces: `LoginLimiter.allow(key, now?): boolean`

- [ ] **Step 1: Write failing credential tests**

Test real temporary files for trimmed non-empty username/password, missing/empty/over-permissive files, equal credentials, wrong username/password, and deterministic version digest. Assert all invalid logins return only `false`.

- [ ] **Step 2: Verify RED**

Run: `cd web/manager && pnpm test lib/auth/bootstrap-credentials.test.ts`
Expected: FAIL because the module does not exist.

- [ ] **Step 3: Implement minimal credential loading**

Read `SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE` and `SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE`, require regular owner-readable files without group/other permission bits, reject empty values, and compare fixed-length SHA-256 digests with `timingSafeEqual`. Return a SHA-256 credential version without exposing the password.

- [ ] **Step 4: Write and verify RED for limiting**

Assert five attempts in one minute are allowed, the sixth is denied, a different key is independent, and expired windows are removed. Run the focused test and observe missing implementation failure.

- [ ] **Step 5: Implement and verify GREEN**

Implement a bounded Map-based limiter with fixed `limit=5`, `windowMs=60_000`, and maximum 1,024 keys. Run both auth tests; expected PASS.

- [ ] **Step 6: Commit**

```bash
git add web/manager/lib/auth
git commit -m "feat(ui): add bootstrap credential boundary"
```

### Task 2: Auth.js Session and Login UI

**Files:**
- Modify: `web/manager/package.json`
- Modify: `web/manager/pnpm-lock.yaml`
- Create: `web/manager/auth.ts`
- Create: `web/manager/auth.config.ts`
- Create: `web/manager/lib/auth/session-identity.ts`
- Create: `web/manager/lib/auth/session-identity.test.ts`
- Create: `web/manager/app/login/page.tsx`
- Create: `web/manager/app/login/actions.ts`
- Create: `web/manager/proxy.ts`
- Modify: `web/manager/app/page.tsx`
- Modify: `web/manager/types/next-auth.d.ts`

**Interfaces:**
- Consumes: Task 1 credential loader, verifier, version and limiter.
- Produces: Auth.js `auth`, `handlers`, `signIn`, `signOut`.
- Produces: Session fields `subject`, `tenantId`, `roles`, `credentialVersion`.

- [ ] **Step 1: Install Auth.js**

Run: `cd web/manager && pnpm add next-auth@beta`
Expected: package and lockfile update with no npm/yarn artifacts.

- [ ] **Step 2: Write failing identity tests**

Assert successful credentials map only to `{subject:"bootstrap-admin", tenantId:"default", roles:["admin"]}`, and a Session with an old credential version is rejected.

- [ ] **Step 3: Verify RED, implement, verify GREEN**

Run the focused Vitest file, observe missing module failure, implement pure identity helpers, rerun and expect PASS.

- [ ] **Step 4: Configure Auth.js**

Use Credentials Provider and JWT session strategy without Adapter. Load `AUTH_SECRET_FILE` server-side, set an 8-hour session maximum, copy only trusted identity fields in callbacks, and revalidate credential version whenever a protected session is consumed.

- [ ] **Step 5: Add login and logout workflow**

Build a compact username/password login page using existing UI components. Submit through a server action, return one generic invalid-credentials message, preserve CSRF protections, redirect authenticated users to `/`, and expose a logout command in the existing account menu.

- [ ] **Step 6: Protect pages twice**

Export `auth as proxy` for optimistic route protection and call `auth()` in the protected page/layout before rendering data. Never rely on proxy alone.

- [ ] **Step 7: Verify and commit**

Run: `cd web/manager && pnpm test && pnpm build`
Expected: all tests and production build PASS.

```bash
git add web/manager
git commit -m "feat(ui): add bootstrap admin session"
```

### Task 3: Server-only Manager JWT and BFF

**Files:**
- Create: `web/manager/lib/auth/manager-token.ts`
- Create: `web/manager/lib/auth/manager-token.test.ts`
- Create: `web/manager/lib/api/manager-proxy.ts`
- Create: `web/manager/lib/api/manager-proxy.test.ts`
- Create: `web/manager/app/api/manager/[...path]/route.ts`
- Modify: `web/manager/lib/api/data-source.ts`
- Modify: `web/manager/lib/api/manager-origin.ts`
- Modify: `web/manager/lib/api/client.test.ts`

**Interfaces:**
- Produces: `issueManagerToken(identity, config, now?): Promise<string>`.
- Produces: `proxyManagerRequest(request, path, dependencies): Promise<Response>`.
- Permits only Manager `/api/v1/*` operations explicitly used by UI service modules.

- [ ] **Step 1: Write JWT RED tests**

Generate an RSA test key and assert RS256, `sub`, `tenant_id`, `roles`, `iss=sysarmor-bff`, `aud=sysarmor-manager`, and `exp-iat <= 300`. Assert missing/invalid key material fails.

- [ ] **Step 2: Implement JWT signing and verify GREEN**

Use `jose` as the only new JWT dependency, load the private key from `SYSARMOR_BFF_JWT_PRIVATE_KEY_FILE`, and keep all imports server-only. Run the focused tests; expected PASS.

- [ ] **Step 3: Write BFF RED tests**

Assert unauthenticated requests produce `{error:{code:"unauthorized",message:"Unauthorized"}}`, allowed requests add a Bearer token, unsupported paths/methods return 404/405, query strings are preserved, and upstream network/non-JSON failures become stable JSON errors.

- [ ] **Step 4: Implement allowlisted proxy and route**

Resolve `MANAGER_API_ORIGIN` only on the server, construct upstream URLs from validated path segments, strip browser authorization/cookie headers, forward only needed content headers, and never accept a caller-provided origin.

- [ ] **Step 5: Switch UI API base**

Make browser clients use `/api/manager`; remove `NEXT_PUBLIC_MANAGER_API_BASE` and any public Manager origin configuration. Keep mock data mode explicit.

- [ ] **Step 6: Verify and commit**

Run `cd web/manager && pnpm test && pnpm build`; expected PASS.

```bash
git add web/manager
git commit -m "feat(ui): proxy manager API with internal JWT"
```

### Task 4: Single-Issuer Manager Authentication and JSON Errors

**Files:**
- Modify: `internal/manager/auth/config.go`
- Modify: `internal/manager/auth/config_test.go`
- Delete: `internal/manager/auth/oidc.go`
- Delete: `internal/manager/auth/oidc_test.go`
- Modify: `cmd/sysarmor-manager/main.go`
- Modify: `cmd/sysarmor-manager/main_test.go`
- Create: `internal/manager/api/http_error.go`
- Create: `internal/manager/api/http_error_test.go`
- Modify: `internal/manager/api/http_auth.go`
- Modify: all `internal/manager/api/http_*.go` callers of `http.Error`
- Delete: `cmd/sysarmorctl/auth_token.go`
- Delete: `cmd/sysarmorctl/auth_token_test.go`
- Modify: `cmd/sysarmorctl/main.go`

**Interfaces:**
- `auth.Config` contains only `PublicKeyFile`, `Issuer`, and `Audience`.
- `writeAPIError(w, status, code, message)` emits the sole Manager error envelope.

- [ ] **Step 1: Write verifier RED tests**

Assert valid static RS256 configuration works and missing key/issuer/audience fails. Remove tests that describe runtime OIDC discovery.

- [ ] **Step 2: Simplify verifier configuration**

Delete auth mode and OIDC discovery branches. Manager flags/env accept only BFF public key, issuer and audience. Run `go test ./internal/manager/auth ./cmd/sysarmor-manager`; expected PASS.

- [ ] **Step 3: Write JSON-error RED tests**

Assert missing bearer token, invalid token, forbidden role, bad input, method errors and internal failures return `application/json` with `{error:{code,message}}` and no plaintext body.

- [ ] **Step 4: Centralize Manager errors**

Implement `writeAPIError` and mechanically replace every `http.Error` in Manager API handlers with stable codes. Do not expose wrapped storage/search errors to clients; log internal details server-side where actionable.

- [ ] **Step 5: Remove production token minting**

Delete `sysarmorctl auth token`, its private-key flags/help and production issuer path. Keep JWT creation only in UI BFF and `_test.go` helpers.

- [ ] **Step 6: Verify and commit**

Run: `go test ./internal/manager/auth ./internal/manager/api ./cmd/sysarmor-manager ./cmd/sysarmorctl`
Expected: PASS.

```bash
git add internal/manager cmd/sysarmor-manager cmd/sysarmorctl
git commit -m "refactor(manager): trust only bff identity"
```

### Task 5: Deployment Secrets and Local Initialization

**Files:**
- Create: `tools/auth/init-bootstrap-admin.sh`
- Create: `tools/auth/init-bootstrap-admin_test.sh`
- Modify: `tools/pki/gen-manager-jwt.sh`
- Modify: `deployments/compose.platform.yaml`
- Modify: `deployments/manager/manager.env.example`
- Create: `deployments/manager-ui/Dockerfile`
- Create: `deployments/manager-ui/manager-ui.env.example`
- Modify: `Makefile`

**Interfaces:**
- `make auth-init` creates missing Secret files idempotently.
- Compose service `manager-ui` mounts bootstrap/session/private-key Secrets read-only and reaches Manager at `http://manager:9443`.

- [ ] **Step 1: Write RED shell tests**

Use a temporary output directory to assert first run creates mode `0600` username/password/Auth secret/BFF keys, emits the generated password once, and second run preserves every checksum without printing the password.

- [ ] **Step 2: Implement idempotent initialization**

Use `openssl rand` and existing PKI conventions. Reject partial credential state instead of silently regenerating. Run the shell test; expected PASS.

- [ ] **Step 3: Add UI production image and Compose service**

Build the existing Next.js app with pnpm, run as a non-root user, mount Secrets read-only, expose the UI port, set `MANAGER_API_ORIGIN=http://manager:9443`, and depend on Manager health.

- [ ] **Step 4: Wire Make targets**

Make `pki`/`deploy` initialize auth material without overwriting it. Ensure `reset` preserves PKI/auth Secrets while deleting data volumes, consistent with the clean-baseline contract.

- [ ] **Step 5: Verify and commit**

Run: `bash tools/auth/init-bootstrap-admin_test.sh && docker compose -f deployments/compose.platform.yaml config && make web-build`
Expected: PASS.

```bash
git add tools/auth tools/pki deployments Makefile
git commit -m "feat(deploy): provision bootstrap admin authentication"
```

### Task 6: Doctor, Documentation, and End-to-End Acceptance

**Files:**
- Create: `tools/doctor.sh`
- Create: `tools/doctor_test.sh`
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `deployments/README.md`
- Modify: `web/README.md`
- Modify: `docs/architecture/manager-ui-api-contract.md`

**Interfaces:**
- `make doctor` exits nonzero with a named failed check and never prints Secrets.

- [ ] **Step 1: Write doctor RED tests**

Inject command/HTTP fixtures and assert checks for Secret files, Compose services, Manager health, UI health/login reachability, unauthenticated BFF rejection, and authenticated Manager data access. Assert failure output names the component without leaking fixture Secrets.

- [ ] **Step 2: Implement doctor and verify GREEN**

Keep checks read-only. Use the configured username/password only inside a temporary cookie jar, verify login plus one BFF request, remove the jar on exit, and print a compact pass/fail summary.

- [ ] **Step 3: Reconcile documentation**

Document one fresh-deployment flow: `make auth-init`, `make deploy`, read the one-time bootstrap password, open UI, then `make doctor`. Remove `SYSARMOR_AUTH_MODE`, direct browser Manager access, manual daily JWT generation and OIDC-at-Manager instructions.

- [ ] **Step 4: Run full acceptance**

Run:

```bash
make api
go test ./...
go vet ./...
cd web/manager && pnpm test && pnpm build
docker compose -f deployments/compose.platform.yaml config
git diff --check
```

Expected: every command PASS with no warnings attributable to this change.

- [ ] **Step 5: Run destructive fresh-deploy acceptance**

Run `make reset`, wait for services, then run `make doctor`. This is authorized by the accepted clean-schema baseline; it deletes development data volumes but preserves PKI and auth Secret files. Expected: reset succeeds and doctor reports every check PASS.

- [ ] **Step 6: Commit**

```bash
git add tools/doctor.sh tools/doctor_test.sh Makefile README.md deployments/README.md web/README.md docs/architecture/manager-ui-api-contract.md
git commit -m "docs: define authenticated local deployment"
```

### Task 7: Final Review

**Files:**
- Review only all files changed by Tasks 1-6.

- [ ] **Step 1: Inspect scope and security invariants**

Confirm no auth tables, browser-visible JWT, public Manager origin, production token minting, OIDC Manager mode, plaintext Manager API errors, or Secret logging remains.

- [ ] **Step 2: Inspect size and clarity**

Run file/function size checks on changed code. Split any file over 500 lines or function over 50 lines by responsibility without altering behavior.

- [ ] **Step 3: Re-run full verification**

Repeat Task 6 non-destructive acceptance plus focused auth/BFF tests. Expected: PASS.

- [ ] **Step 4: Review Git history and worktree**

Confirm commits are atomic and the two pre-existing untracked 2026-07-08 UI documents remain untouched and uncommitted.
