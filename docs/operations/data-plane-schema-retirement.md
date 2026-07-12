# Data-Plane Schema Retirement

## Current Window

Worker currently accepts:

```text
sysarmor.dataplane/v1  current
empty                   legacy v0
other                   permanent DLQ failure
```

Every accepted empty-version source message increments
`legacy_data_batches`. This compatibility exists only to drain batches produced
before explicit schema versioning.

## Removal Gate

Do not remove empty-version support until all conditions are true:

1. Every supported Agent and data-append producer emits
   `sysarmor.dataplane/v1`.
2. Record the current `legacy_data_batches` value and confirm it does not
   increase for longer than Kafka `retention.ms` plus one deployment safety
   interval.
3. Restart or roll Worker during that interval and confirm the value remains
   stable after restart using the platform metrics history.
4. Publish a release note announcing that empty schema becomes unsupported.
5. Verify the Kafka topic has no retained empty-version records.

Example observation:

```text
window = Kafka retention.ms + 24h safety interval
start  = legacy_data_batches at T0
end    = legacy_data_batches at T0 + window
ready  = end == start and all supported producers are v1
```

The counter is process-local and monotonic for a Worker lifetime. Operational
metrics storage must compare the time series across restarts rather than assume
an in-process value survives restart.

## Removal Release

In the removal release:

1. Change `schema.ValidateDataPlane("")` to return
   `UnsupportedVersionError`.
2. Change Worker tests so empty schema writes a DLQ envelope with
   `unsupported_schema_version` and commits only after DLQ success.
3. Remove `RecordLegacyDataBatch` and `legacy_data_batches` after its dashboard
   and alert are no longer needed.
4. Deploy consumers before any producer begins using a future v2 contract.

Rollback restores the preceding Worker version. It does not require changing
producers already emitting v1.
