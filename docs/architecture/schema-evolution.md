# Schema Evolution Rules

## Version Axes

SysArmor uses independent versions for independent meanings:

- `schema_version` identifies a serialized transport contract and determines
  whether a consumer may process a message.
- `analysis_version` identifies the analysis rules and semantics that generated
  an Incident Report and participates in report identity.

A detection-rule or correlation change updates `analysis_version`. An
incompatible message-contract change updates `schema_version`. Either version
may change without the other.

The current data-plane contract is `sysarmor.dataplane/v1`. New producers must
set it on every `DataBatch`.

## Protobuf

1. Existing field numbers and meanings are immutable.
2. New fields use new numbers.
3. Deprecated fields are marked deprecated before removal.
4. Removed fields reserve both number and name.
5. A field number or name is never reused.
6. Adding an optional field that old consumers can ignore does not require a
   new schema major version.
7. Removing a field, changing its type or meaning, or making optional data
   required creates a new schema major version.
8. Generated Go files are updated only through `make api`.

## Worker Compatibility

A Worker accepts the current schema and at most its immediate predecessor by an
explicit allowlist. Successful protobuf decoding is not a compatibility check.

During the first rollout, an empty `DataBatch.schema_version` represents legacy
v0 and is accepted alongside `sysarmor.dataplane/v1`. Legacy acceptance is
counted in `legacy_data_batches`. After producers have emitted v1 for longer
than Kafka retention and the metric is zero, empty-version support is removed.

An unsupported non-empty version is a permanent failure with code
`unsupported_schema_version`. The source Kafka offset is committed only after
the DLQ record is published successfully.

For a future v2 rollout:

1. Deploy consumers accepting v1 and v2.
2. Deploy producers emitting v2.
3. Wait through Kafka retention and verify v1 traffic is zero.
4. Remove v1 compatibility in a later consumer release.

## OpenSearch

OpenSearch schema versions are physical index versions, not message or analysis
versions. Applications use stable `*-read` and `*-write` aliases. Compatible
field additions may update the current template; incompatible mapping changes
create a new physical index and use reindex plus an atomic alias switch.

See `docs/operations/opensearch-schema-evolution.md` for the operational
procedure.
