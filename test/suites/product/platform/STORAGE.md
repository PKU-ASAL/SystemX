# Storage Suite

Scope: `storage`

System under test:

- store backend;
- Postgres migrations;
- table projection paths;
- query and write contracts backed by persistent storage.

PostgreSQL owns control-plane state. Event, signal, evidence, and incident report projections belong to OpenSearch; storage tests must not expect incident reports to survive through PostgreSQL.

Out of scope:

- runtime agent behavior;
- endpoint collection effectiveness;
- cloud detection scoring.

Store status and query pagination scripts are native suite cases. Postgres projection checks remain Go test entrypoints.
