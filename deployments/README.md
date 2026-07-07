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
make deploy
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

## Agent Install And Enrollment

Manager owns agent enrollment creation. Operators create an enrollment, receive
a one-time token and an install script URL, then run the script on the endpoint.
The manager stores only the token hash.

```bash
sysarmorctl --manager-url http://127.0.0.1:19443 --json \
  manager artifacts upload \
  --file dist/sysarmor-agent-linux-amd64-v1.tar.gz \
  --name sysarmor-agent \
  --kind agent \
  --version v1 \
  --os linux \
  --arch amd64 \
  --status active

sysarmorctl --manager-url http://127.0.0.1:19443 --json \
  manager channels upsert \
  --channel stable \
  --artifact-id art_...

sysarmorctl --manager-url http://127.0.0.1:19443 --json \
  manager enrollments create \
  --agent-id node-a \
  --gateway-addr 127.0.0.1:19444 \
  --gateway-sni localhost \
  --ttl 24h \
  --channel stable
```

The generated `agent-install.sh` installs the agent into a stable agent home:

```text
/opt/sysarmor/agent/bin/sysarmor-agent
/opt/sysarmor/agent/bundles/tetragon
/opt/sysarmor/agent/sensors
/opt/sysarmor/agent/runtime
/opt/sysarmor/agent/cache
/etc/sysarmor/agent.yaml
/etc/sysarmor/policies/...
/etc/systemd/system/sysarmor-agent.service
/run/sysarmor/agent.sock
```

Agent artifacts are signed distribution tarballs. The top-level
`manifest.json` describes the entrypoint, systemd unit, sensor bundles, and
file checksums; `manifest.sig` signs that manifest. The manager verifies the
artifact at upload time when `SYSARMOR_ARTIFACT_PUBLIC_KEY` is configured, and
the bootstrap script verifies the same manifest before installing.

The default gateway path requires agent-plane mTLS. The bootstrap script
generates the endpoint private key locally, submits a CSR with the enrollment
token, and stores the manager-issued certificate under `/etc/sysarmor/pki`.
The gateway validates the certificate URI SAN against the reported
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
- OpenSearch: `29200`

Manager-hosted artifacts are stored under
`/var/lib/sysarmor/manager/artifacts`; the compose deployment persists this path
with the `manager-artifacts` volume. Override it with
`SYSARMOR_ARTIFACT_DIR` when running the manager directly.

Host ports can be overridden when a local machine already has a service bound:

```bash
SYSARMOR_OPENSEARCH_PORT=39200 make deploy
```

The platform services still talk to each other through compose service names
such as `opensearch:9200`; these overrides only affect host access.

`gateway --local-ingest` is intentionally not used by this compose file. It is a
development and smoke-test fixture, not the standard platform path.
