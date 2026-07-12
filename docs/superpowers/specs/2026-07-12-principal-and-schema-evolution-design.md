# Principal Boundary And Schema Evolution Design

## 1. Conclusion

SysArmor Manager must have one identity path: a verified RS256 JWT creates a
request-scoped `Principal`, and authorization reads only that Principal. Static
operator tokens, identity headers, implicit local authorization, and production
authentication bypasses are removed.

Transport schema version and analysis version are separate contracts:

- `schema_version` identifies the serialized message contract accepted by a
  consumer.
- `analysis_version` identifies the rules and semantics that produced an
  Incident Report.

OpenSearch mappings evolve through versioned physical indices and stable read
and write aliases. Protobuf and Worker compatibility rules are explicit and
tested.

## 2. Scope

This change covers:

- Manager HTTP authentication and authorization helpers;
- Manager API tests that currently use a static operator token;
- the Kafka-ingested `DataBatch` protobuf contract and Worker validation;
- protobuf field evolution rules;
- OpenSearch index naming and mapping migration rules;
- architecture and operational documentation.

This change does not add an identity provider, JWKS discovery, a new Kafka
envelope, automated OpenSearch reindex orchestration, or Incident case
management.

## 3. Principal-Only Authentication

### 3.1 Production request flow

The only production identity flow is:

```text
Authorization: Bearer <RS256 JWT>
  -> JWT verifier
  -> Principal{Subject, TenantID, Roles}
  -> tenant binding
  -> API authorization
```

`/healthz` remains anonymous. Every `/api/` request must contain a verified
Principal. Missing identity returns `401`; a valid identity without the required
role or with a mismatched tenant returns `403`.

The Manager must not create identity from:

- `X-SysArmor-Operator-Token`;
- a static bearer token;
- `X-SysArmor-Actor` or `X-SysArmor-Role`;
- request body actor or role fields;
- a local or development bypass flag.

Audit actor and role values are derived from Principal. Request actor and role
fields may remain temporarily for wire compatibility, but they are not trusted
for authorization or audit attribution.

### 3.2 Server construction

`Server` has no operator-token field. Static-token constructors are deleted.
There are two explicit uses:

- production constructs a server with its required dependencies and exposes it
  only through JWT-authenticated middleware;
- unit tests call the internal Handler with a Principal already present in the
  request context.

The bare Handler is fail-closed for protected endpoints. Constructing a Server
without JWT middleware must never make a protected operation anonymous or
implicitly privileged.

### 3.3 Test identity

Authorization and business Handler tests use a helper defined only in a
`*_test.go` file. It attaches a `managerauth.Principal` to a request context.
The helper is not compiled into production binaries and creates no HTTP header,
route, configuration switch, or runtime bypass.

JWT tests continue to sign real RS256 tokens and cover signature, algorithm,
issuer, audience, expiry, required claims, and middleware integration. At least
one protected API integration test must traverse the complete JWT-to-Principal
path.

## 4. Message Schema Evolution

### 4.1 Two independent version axes

`DataBatch.schema_version` identifies the data-plane transport contract. Its
initial value is `sysarmor.dataplane/v1`.

`Incident.analysis_version` continues to identify analysis semantics. Its
current value is `incident.v1` and remains part of deterministic Incident Report
identity.

Changing detection rules, correlation logic, scoring, or report meaning changes
`analysis_version`, not `schema_version`. Changing an incompatible serialized
message contract changes `schema_version`, not `analysis_version`. A release may
change either, both, or neither.

No additional Kafka envelope is introduced. The version belongs to the top-level
message actually consumed from Kafka, currently `DataBatch`.

### 4.2 Compatibility policy

The Worker maintains a centralized allowlist containing the current schema and
at most its immediate predecessor. For the first rollout:

- current: `sysarmor.dataplane/v1`;
- legacy predecessor: an empty version from already queued messages.

An empty version is accepted for one release window and is treated as legacy
`v0`. Newly produced batches must always set `sysarmor.dataplane/v1`. A metric
must expose accepted legacy batches so removal is observable.

An unsupported non-empty version is a permanent processing failure. The source
message is committed only after the DLQ record is written successfully, using
the existing permanent-failure policy. The failure code identifies an
unsupported schema version.

The next incompatible contract, `sysarmor.dataplane/v2`, may accept `v1` as its
predecessor. It must not continue accepting empty legacy messages. Compatibility
is explicit; successful protobuf unmarshalling alone does not prove semantic
compatibility.

### 4.3 Protobuf rules

- Existing field numbers and meanings are immutable.
- New fields use new field numbers.
- Removed fields reserve both their numbers and names.
- Deprecated fields are marked deprecated before removal.
- Field numbers and names are never reused.
- Adding an optional, backward-compatible field does not by itself require a
  schema major-version change.
- Removing a field, changing its meaning or type, or making previously optional
  information required does require a new schema major version.
- Generated protobuf files are changed only through the repository API
  generation command.

## 5. OpenSearch Mapping Evolution

Applications use stable aliases and do not depend on physical index names:

```text
sysarmor-events-read       -> sysarmor-events-v1
sysarmor-events-write      -> sysarmor-events-v1
sysarmor-signals-read      -> sysarmor-signals-v1
sysarmor-signals-write     -> sysarmor-signals-v1
sysarmor-incidents-read    -> sysarmor-incidents-v1
sysarmor-incidents-write   -> sysarmor-incidents-v1
sysarmor-evidence-read     -> sysarmor-evidence-v1
sysarmor-evidence-write    -> sysarmor-evidence-v1
```

Compatible mapping additions may update the current index template. An
incompatible mapping change creates the next physical index, reindexes existing
documents, validates counts and representative queries, and atomically switches
both aliases. Field types are never changed in place.

The application configuration names aliases. Bulk projection uses write aliases;
history and Manager queries use read aliases. Alias creation and migration are
deployment responsibilities and must be documented as deterministic operations;
this change does not build a general migration framework.

## 6. Failure Handling

- Missing Principal: `401 Unauthorized`.
- Insufficient role or tenant mismatch: `403 Forbidden`.
- Identity headers without a verified JWT: ignored and the request remains
  unauthenticated.
- Empty legacy schema during its compatibility window: accepted and counted.
- Unsupported schema: permanent failure and DLQ before offset commit.
- OpenSearch alias missing or pointing to an invalid mapping: projection/search
  fails explicitly; the application does not fall back to an unversioned index.

## 7. Verification

The implementation is complete when tests prove:

1. No static operator-token symbol, constructor, header, environment variable,
   or documentation contract remains.
2. Protected handlers reject requests without Principal even when called
   without JWT middleware.
3. Actor and role headers cannot grant privileges or change audit identity.
4. Test-only Principal injection exercises viewer, operator, admin, and tenant
   isolation behavior.
5. Real RS256 JWT integration reaches a protected API and invalid JWTs do not.
6. New `DataBatch` producers populate `sysarmor.dataplane/v1`.
7. Worker accepts current and empty legacy schema, counts legacy use, and rejects
   unsupported versions as permanent failures.
8. OpenSearch reads and writes use stable aliases rather than physical indices.
9. API generation, all Go tests, `go vet`, and `git diff --check` pass.

## 8. Rollout

Deploy consumers before producers. The first consumer accepts both empty legacy
batches and `sysarmor.dataplane/v1`; the following producer release always emits
`v1`. After Kafka retention and the agreed release window have elapsed and the
legacy metric is zero, remove empty-version compatibility.

Create the initial versioned OpenSearch indices and aliases before deploying
code that requires aliases. Existing unversioned indices are migrated explicitly;
the application must not silently create parallel empty indices.
