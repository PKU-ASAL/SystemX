# Clean Schema Baseline Design

## 1. Conclusion

SysArmor Next performs one pre-release baseline reset. The repository supports
fresh deployments only and no longer upgrades PostgreSQL, Kafka messages, or
OpenSearch data created by earlier development revisions.

After this reset, the repository contains one current PostgreSQL baseline, one
current protobuf contract per package, mandatory data-plane schema identity,
and versioned OpenSearch indices behind aliases. Historical compatibility,
cleanup migrations, deprecated Incident lifecycle fields, and legacy data
readers are deleted.

This reset is exceptional. Once committed, protobuf field numbers, schema V1,
and published migrations are frozen under the normal evolution rules.

## 2. Deployment Contract

The only supported transition from an earlier development revision is:

```text
stop services
delete development data volumes
start current services
create current PostgreSQL baseline
create current OpenSearch v1 indices and aliases
produce only current Kafka messages
```

Existing PostgreSQL rows, OpenSearch documents, Kafka records, Redis state, and
control-plane state are not migrated. PKI is preserved by default because it is
not schema state.

## 3. PostgreSQL Baseline

The ordered migration framework remains for future releases, but `Ordered()`
contains exactly one migration:

```text
V1 current_control_plane_baseline
```

`PostgresVersion` is `1`. V1 directly creates only current control-plane tables
and indexes. It never creates and later drops an obsolete object.

The baseline excludes:

- `operator_role_bindings`;
- `incidents`;
- `incident_events`;
- OpenSearch evidence projections;
- any table that belongs to report/search storage.

Migration V2 `incident_reports_to_opensearch`, migration V3
`drop_operator_role_bindings`, legacy Incident cleanup SQL, upgrade detection,
and upgrade-only tests are deleted.

The migration executor, advisory lock, transactional version recording, and
schema inspection remain. Future released changes start at V2 and must never
rewrite V1.

## 4. Protobuf Baseline

### 4.1 Incident

`sysarmor.incident.v1.Incident` is rewritten as the current immutable report
contract:

```proto
message Incident {
  string id = 1;
  string summary = 2;
  uint32 severity = 3;
  repeated string mitre = 4;
  repeated string lineage_ids = 5;
  repeated string terminals = 6;
  EvidenceSubgraph evidence = 7;
  ConvergeTrace converge = 8;
  repeated sysarmor.signal.v1.Signal contributing_signals = 9;
  map<string, string> labels = 10;
  string tenant_id = 11;
  string correlation_key = 12;
  string analysis_version = 13;
  string first_observed_at = 14;
  string last_observed_at = 15;
}
```

The reset deletes `status`, `status_reason`, `status_actor`, their deprecated
annotations, and all reserved names/numbers inherited only from unreleased
development history. Generated files are regenerated through `make api`.

Production and tests delete all lifecycle field writes, merges, assertions, and
fallbacks. Incident remains an immutable/rebuildable report; future human
workflow belongs to a separate IncidentCase model.

### 4.2 DataBatch

Every new `DataBatch` must set:

```text
schema_version = sysarmor.dataplane/v1
```

Empty and unknown versions are both unsupported permanent failures. Worker
writes the source message to DLQ with `unsupported_schema_version` and commits
only after DLQ success.

The legacy-empty branch, `legacy_data_batches` metric, Store method, retirement
document, dashboards/tests, and operational gate are deleted.

## 5. OpenSearch Baseline

The only supported physical indices are:

```text
sysarmor-events-v1
sysarmor-signals-v1
sysarmor-incidents-v1
sysarmor-evidence-v1
```

Applications use only the corresponding `*-read` and `*-write` aliases.
Initializer mappings are the source of truth.

The initializer no longer checks or explains how to migrate old unversioned
indices because old volumes are unsupported. It remains idempotent and refuses
aliases pointing to an unexpected physical index.

The real v1/v2 alias lifecycle test remains because future alias-based mapping
evolution is part of the current architecture, not legacy compatibility.

Manager and Worker delete any fallback that reads Incident identity from labels
or old indices. New Incident documents always use formal identity fields.

## 6. Development Reset

`make reset` is the explicit destructive development reset command. It:

1. runs `docker compose down --volumes --remove-orphans` for the platform
   Compose project;
2. preserves the repository PKI directory;
3. starts a fresh platform using the normal `make up` flow;
4. relies on PostgreSQL V1 and `opensearch-init` to establish schemas;
5. reports the service status after startup.

The Make help text labels it destructive. It does not silently run during
ordinary `make up`, tests, or deployment.

No `reset-pki` target is added in this change; PKI cleanup is a separate concern
and not required by the schema reset.

## 7. Documentation Cleanup

Active architecture and operations documentation describes only the clean
baseline. Delete documents whose sole purpose is:

- migrating old Incident PostgreSQL projections;
- retiring empty schema compatibility;
- cleaning legacy Incident tables;
- migrating unversioned OpenSearch development indices.

Historical Superpowers specifications and plans remain as decision records.
They may mention removed history and are not runtime contracts.

## 8. Failure Handling

- Existing development volumes: operator must run `make reset`; no automatic
  migration is attempted.
- Empty/unknown `DataBatch.schema_version`: permanent DLQ failure.
- Missing or incorrect OpenSearch alias: initialization or application failure;
  no fallback index creation.
- Missing Principal or invalid JWT: unchanged fail-closed behavior.
- `make reset`: destructive behavior is explicit in target name and help text.

## 9. Verification

The reset is complete when:

1. `PostgresVersion == 1` and `Ordered()` contains only current baseline V1.
2. Current PostgreSQL schema contains no obsolete report or role-binding table.
3. Incident protobuf exactly matches the clean 15-field contract.
4. Production and active tests contain no deprecated Incident lifecycle access.
5. Empty and unknown DataBatch versions both follow permanent DLQ handling.
6. `legacy_data_batches` and its compatibility code/documentation are absent.
7. OpenSearch initializer and application contain no old unversioned index
   compatibility branch.
8. `make reset` removes data volumes, preserves PKI, starts the normal platform,
   and reports status.
9. A fresh platform contains only current PostgreSQL schema and OpenSearch v1
   physical indices/aliases.
10. `make api`, all Go tests, `go vet`, shell syntax checks, Compose config,
    OpenSearch lifecycle test, repository scans, and `git diff --check` pass.
