# SysArmor Deployments

This directory contains production-shaped deployment assets. Test fixtures may
reuse these assets, but topology-specific setup belongs under `test/`.

## Layout

- `agent/`: endpoint agent install script and systemd unit.
- `gateway/`: agent-facing gRPC gateway image and example environment.
- `manager/`: operator-facing HTTP API image and example environment.
- `worker/`: telemetry ingest worker image and example environment.
- `infra/`: local Postgres, Kafka, Redis, and OpenSearch images.
- `sensors/`: sensor bundles and installers used by the agent.
- `pki/`: sample agent-plane mTLS material and examples.

## Local Platform

Generate local agent-plane mTLS material first:

```bash
SYSARMOR_GATEWAY_IPS=127.0.0.1 \
  tools/pki/gen-agent-plane-mtls.sh deployments/pki/agent-plane-mtls/runtime default agent-prod-001 localhost
```

Start the standard local platform:

```bash
docker compose -f deployments/compose.platform.yaml up -d --build
```

For the VM topology harness, use the VM override so manager/gateway are exposed
on their in-VM standard ports:

```bash
docker compose \
  -f deployments/compose.platform.yaml \
  -f deployments/compose.vm-topology.yaml \
  up -d --build
```

Runtime shape:

```text
agent -> gateway:9444 -> kafka -> worker -> postgres/opensearch
operator -> manager:9443 -> postgres/opensearch
gateway -> postgres/redis/kafka
```

The default gateway path requires agent-plane mTLS. Test or production agents
must use a client certificate whose URI SAN matches the reported
`tenant_id/agent_id`.

Services:

- `manager`: operator-facing control, audit, policy, and query API.
- `gateway`: agent-facing data/control gRPC endpoint.
- `worker`: Kafka ingest consumer for detection, incident projection, and indexing.
- `postgres`: control, state, and audit store.
- `kafka`: durable telemetry ingest log.
- `redis`: gateway hot state.
- `opensearch`: searchable events, signals, and evidence layer.

Ports:

- Manager HTTP: `19443`
- Gateway gRPC: `19444`
- Gateway health HTTP: `19445`
- Postgres: `15432`
- Kafka: `19092`
- Redis: `16379`
- OpenSearch: `19200`

`gateway --local-ingest` is intentionally not used by this compose file. It is a
development and smoke-test fixture, not the standard platform path.
