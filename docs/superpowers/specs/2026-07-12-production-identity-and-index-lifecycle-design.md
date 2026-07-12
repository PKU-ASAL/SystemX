# Production Identity And Index Lifecycle Design

## 1. Conclusion

SysArmor verifies trusted JWTs but does not manage users. Manager exposes one
Bearer-JWT authentication contract and supports two mutually exclusive key
sources:

- `local`: a configured RS256 public key for development, CI, demos, and closed
  deployments;
- `oidc`: standard OIDC Discovery and JWKS for enterprise production.

Both sources create the same request-scoped Principal. SysArmor does not store
users, passwords, sessions, refresh tokens, or identity-provider state, and the
default deployment does not require Keycloak, Dex, or another identity service.

OpenSearch indices and aliases are provisioned idempotently before Manager and
Worker start. The obsolete PostgreSQL-backed OperatorRoleBinding model is
removed. Legacy empty data-plane schema compatibility remains for this release,
with an explicit, observable removal gate. Container integration tests verify
alias migration and rollback against real OpenSearch.

## 2. Scope

This change implements:

1. idempotent OpenSearch physical-index and alias provisioning;
2. complete removal of OperatorRoleBinding API, CLI, Store, PostgreSQL
   projection, and active schema objects;
3. a measurable release gate for removing empty `DataBatch.schema_version`;
4. real-container OpenSearch alias cutover and rollback tests;
5. generic OIDC Discovery/JWKS verification with `kid` rotation;
6. a minimal local JWT issuing command for tests and development.

It does not implement user registration, passwords, login UI, refresh tokens,
MFA, sessions, identity-provider administration, automatic mapping inference,
or a bundled identity provider.

## 3. Authentication Boundary

### 3.1 Modes

Manager requires `SYSARMOR_AUTH_MODE` with exactly one of:

```text
local
oidc
```

There is no `disabled`, anonymous, static-token, or debug-bypass mode.

Local mode requires:

```text
SYSARMOR_AUTH_MODE=local
SYSARMOR_JWT_PUBLIC_KEY_FILE=/path/to/public.pem
SYSARMOR_JWT_ISSUER=sysarmor-local
SYSARMOR_JWT_AUDIENCE=sysarmor-manager
```

OIDC mode requires:

```text
SYSARMOR_AUTH_MODE=oidc
SYSARMOR_OIDC_ISSUER_URL=https://identity.example.com/realms/security
SYSARMOR_JWT_AUDIENCE=sysarmor-manager
```

OIDC mode derives the expected issuer from the configured issuer URL and
discovers `jwks_uri` through `/.well-known/openid-configuration`. Local-only and
OIDC-only configuration must not be mixed. Missing, unknown, or conflicting
configuration fails Manager startup.

Both modes require RS256 and the existing claims `sub`, `tenant_id`, `roles`,
`iss`, `aud`, and `exp`. Both create the existing Principal and use the same
tenant and role authorization middleware.

### 3.2 OIDC Discovery And JWKS

The verifier:

- retrieves Discovery over HTTP(S), requiring HTTPS except for loopback test
  issuers;
- requires the discovered issuer to exactly equal the configured issuer after
  removing a trailing slash;
- requires an absolute `jwks_uri`, with HTTPS except for loopback tests;
- accepts only RSA signing keys with a non-empty `kid` and `use` absent or
  `sig`;
- selects a key by the JWT `kid` and rejects tokens without `kid` in OIDC mode;
- caches keys for a bounded five-minute interval;
- performs one rate-limited JWKS refresh when a `kid` is unknown, enabling
  rotation without restart;
- keeps the last successfully loaded key set when a background refresh fails;
- fails closed when no valid key is available;
- uses HTTP clients with explicit timeouts and bounded response bodies.

Discovery is loaded at startup so invalid configuration fails early. Tests use
an in-memory OIDC server and dynamically generated RSA keys; no external network
or real identity provider is required.

### 3.3 Local Token Tool

`sysarmorctl auth token` signs a development JWT with an RSA private key:

```text
sysarmorctl auth token \
  --private-key deployments/pki/agent-plane-mtls/runtime/manager-jwt-private.pem \
  --subject local-admin \
  --tenant default \
  --roles admin \
  --issuer sysarmor-local \
  --audience sysarmor-manager \
  --ttl 8h
```

The command writes only the JWT to stdout. It validates the private key,
subject, tenant, recognized roles, issuer, audience, and a positive TTL with an
upper bound of 24 hours. It never generates, stores, logs, or transmits private
keys. The existing PKI script remains responsible for local key generation.

## 4. OperatorRoleBinding Removal

OperatorRoleBinding no longer affects authorization and is removed completely:

- delete `/api/v1/operator-role-bindings`;
- delete the `sysarmorctl operator-role-bindings` command;
- delete Store types, state fields, methods, normalization, export/import, and
  tests;
- delete ManagerStore methods and handlers;
- delete PostgreSQL snapshot projection and associated reads/writes;
- add the next ordered migration that drops the obsolete role-binding table if
  it exists;
- remove active API and architecture documentation.

Dropping this table is safe because the data no longer grants permissions and
has no future owner. The migration remains transactional and idempotent. VM
deployment snapshots are not hand-edited as part of the main source change.

## 5. OpenSearch Provisioning

### 5.1 Desired State

An idempotent script owns the initial v1 physical indices and aliases:

```text
sysarmor-events-v1       <- sysarmor-events-read, sysarmor-events-write
sysarmor-signals-v1      <- sysarmor-signals-read, sysarmor-signals-write
sysarmor-incidents-v1    <- sysarmor-incidents-read, sysarmor-incidents-write
sysarmor-evidence-v1     <- sysarmor-evidence-read, sysarmor-evidence-write
```

Exactly one target of each write alias has `is_write_index=true`. The script:

- waits for OpenSearch readiness with a bounded timeout;
- creates missing v1 indices with committed mappings and settings;
- creates missing aliases atomically;
- succeeds without changes when desired state already exists;
- refuses to repoint an existing alias or overwrite an incompatible physical
  index;
- detects an old unversioned index and exits with an actionable migration error
  rather than silently creating parallel empty storage;
- accepts URL and optional basic-auth credentials through environment variables;
- never prints credentials.

Compose uses a one-shot `opensearch-init` service. Manager and Worker depend on
its successful completion, so application services never race alias creation.
Local and product container harnesses use the same script, not duplicate curl
logic.

### 5.2 Mapping Ownership

Versioned JSON mapping files live with the provisioning script and are reviewed
source artifacts. Initial mappings define only fields already queried or sorted
by the application. Dynamic mapping remains enabled for report content, while
identity, tenant, timestamp, and filter fields have explicit stable types.

## 6. Legacy Data-Plane Removal Gate

Empty `DataBatch.schema_version` remains accepted for this release. Removal is
allowed only when all conditions hold:

1. all supported producers emit `sysarmor.dataplane/v1`;
2. `legacy_data_batches` has not increased for longer than configured Kafka
   retention plus one deployment safety interval;
3. the observation interval includes at least one Worker restart or rollout;
4. a release note announces that empty schema becomes unsupported;
5. the following release changes the compatibility allowlist and tests empty
   schema as permanent DLQ failure.

The metric is monotonic across a Worker process lifetime and is documented with
an alert/query example. This change adds tests and an operational checklist but
does not remove empty-version compatibility early.

## 7. Real OpenSearch Integration Tests

A container test starts the repository OpenSearch image, provisions v1, indexes
documents through write aliases, and queries them through read aliases. It then:

1. creates v2 with an intentionally compatible test mapping;
2. reindexes v1 into v2;
3. validates document counts and representative queries;
4. atomically switches both aliases to v2;
5. verifies new writes and reads use v2;
6. atomically rolls both aliases back to v1;
7. verifies reads and writes return to v1.

The test must clean up its uniquely prefixed indices and network/container state.
It is exposed through a focused Make target and may be skipped by ordinary unit
tests when Docker is unavailable; CI and the product integration suite run it
explicitly.

## 8. Failure Handling

- Invalid or conflicting auth configuration: Manager startup failure.
- Discovery or initial JWKS unavailable: OIDC-mode startup failure.
- Unknown `kid`: one bounded refresh, then `401` if still absent.
- Expired, wrong-issuer, wrong-audience, non-RS256, or malformed JWT: `401`.
- OpenSearch initialization timeout or incompatible existing state: init service
  failure; Manager and Worker do not start.
- Alias switch validation failure: leave current aliases unchanged.
- Operator role-binding route after removal: authenticated `404`.
- Empty legacy schema during this release: accepted and counted.

## 9. Verification

The implementation is complete when:

1. local and OIDC modes both produce the same Principal authorization behavior;
2. OIDC tests cover discovery, cache reuse, unknown-kid refresh, rotation,
   invalid issuer/audience/algorithm, outage, and response-size bounds;
3. no user, password, session, refresh-token, static-token, or auth-bypass model
   exists in SysArmor;
4. no OperatorRoleBinding symbol, API route, CLI command, active table, or
   documentation contract remains;
5. a fresh Compose deployment creates physical indices and aliases before
   Manager and Worker start;
6. repeated initialization is a no-op and incompatible state fails explicitly;
7. the real OpenSearch cutover and rollback test passes;
8. legacy schema removal conditions are documented and tested without removing
   current compatibility;
9. API generation, all Go tests, `go vet`, shell syntax checks, Compose config,
   and `git diff --check` pass.

## 10. Rollout

1. Deploy the OperatorRoleBinding cleanup migration.
2. Migrate existing unversioned OpenSearch data to v1 and create aliases using
   the documented migration procedure.
3. Deploy idempotent initialization and alias-aware services.
4. Continue local-key mode for existing closed deployments, now with explicit
   `SYSARMOR_AUTH_MODE=local`.
5. Enable OIDC mode only where an enterprise identity provider already exists.
6. Observe legacy schema metrics through Kafka retention before scheduling its
   removal release.
