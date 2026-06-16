# SysArmor Platform Deployment

This directory contains the local platform stack for v3 development and tests.

```bash
docker compose -f deployments/compose.platform.yaml up -d --build
```

Services:

- `postgres`: control/state/audit metadata store.
- `kafka`: durable raw telemetry ingest log.
- `redis`: Agent Gateway hot state.
- `opensearch`: searchable events/signals/evidence layer.
- `manager`: manager API and Agent Gateway.
- `worker`: Kafka ingest consumer for detection, incident projection, and OpenSearch indexing.

Ports:

- Manager HTTP: `19443`
- Manager gRPC: `19444`
- Postgres: `15432`
- Kafka: `19092`
- Redis: `16379`
- OpenSearch: `19200`
