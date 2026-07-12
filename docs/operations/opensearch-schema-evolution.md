# OpenSearch Schema Evolution

## Contract

Applications access OpenSearch through stable aliases:

| Data | Read alias | Write alias | Initial physical index |
| --- | --- | --- | --- |
| Events | `sysarmor-events-read` | `sysarmor-events-write` | `sysarmor-events-v1` |
| Signals | `sysarmor-signals-read` | `sysarmor-signals-write` | `sysarmor-signals-v1` |
| Incidents | `sysarmor-incidents-read` | `sysarmor-incidents-write` | `sysarmor-incidents-v1` |
| Evidence | `sysarmor-evidence-read` | `sysarmor-evidence-write` | `sysarmor-evidence-v1` |

The application never falls back to an unversioned index. A missing alias is an
explicit deployment error.

## Initial Provisioning

Create each physical index with its reviewed settings and mapping, then attach
both aliases atomically. For incidents:

```json
POST /_aliases
{
  "actions": [
    {"add": {"index": "sysarmor-incidents-v1", "alias": "sysarmor-incidents-read"}},
    {"add": {"index": "sysarmor-incidents-v1", "alias": "sysarmor-incidents-write", "is_write_index": true}}
  ]
}
```

Repeat with the corresponding names for events, signals, and evidence. Deploy
alias-aware application code only after all aliases exist.

## Incompatible Mapping Upgrade

For an incompatible incident mapping change from v1 to v2:

1. Create `sysarmor-incidents-v2` from the reviewed v2 template.
2. Reindex `sysarmor-incidents-v1` into `sysarmor-incidents-v2`.
3. Compare document counts and run representative tenant, timestamp, and exact
   ID queries against v2.
4. Pause or drain writers for the short cutover interval, or perform a final
   delta reindex when the deployment process provides one.
5. Atomically remove both aliases from v1 and add them to v2 with one
   `POST /_aliases` request.
6. Resume writers and verify Bulk projection and Manager reads.

Never change an existing field type in place.

## Rollback

Keep the previous physical index until the new mapping has passed its retention
and operational verification window. Roll back by atomically moving both aliases
to the previous index. Do not delete the failed index until its documents and
failure evidence have been inspected.

## Verification

Before and after a switch, verify:

```text
GET /_alias/sysarmor-*-read
GET /_alias/sysarmor-*-write
GET /_cat/count/sysarmor-incidents-v2?v
GET /sysarmor-incidents-read/_search
```

The read and write aliases for one data kind must resolve to the intended
physical version, and exactly one target of a write alias must have
`is_write_index=true`.
