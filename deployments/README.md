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

New packages install and start in standalone mode. The Agent immediately
collects into its bounded local store and makes no Manager or Gateway
connection. The default state is under `/var/lib/sysarmor/agent`; inspect it
through the local Unix socket:

```bash
sudo deployments/agent/install-agent.sh
sudo sysarmorctl agent health
```

Enrollment is an explicit later operation. Manager creates a one-time token
and stores only its hash. Run the returned command on the endpoint; the Agent
generates its private key locally, obtains and validates the certificate, then
atomically changes its SQLite enrollment state to managed:

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

sudo sysarmorctl \
  --manager-url http://127.0.0.1:19443 \
  enroll \
  --token '<one-time-token>' \
  --tenant default \
  --agent-id node-a \
  --gateway 127.0.0.1:19444 \
  --gateway-server-name localhost
```

Registration uploads only batches created from the enrollment boundary.
Add `--upload-history` only when pre-enrollment telemetry must be uploaded.
Return to fully local operation without stopping the sensor:

```bash
sudo sysarmorctl unenroll
```

The generated `agent-install.sh` installs the agent into a stable agent home:

```text
/opt/sysarmor/agent/bin/sysarmor-agent
/opt/sysarmor/agent/bundles/tetragon
/opt/sysarmor/agent/sensors
/opt/sysarmor/agent/runtime
/opt/sysarmor/agent/cache
/etc/sysarmor/agent/agent.yaml
/etc/sysarmor/agent/policy.json
/etc/systemd/system/sysarmor-agent.service
/run/sysarmor/agent/control.sock
```

Enrollment install profiles:

- `linux-systemd` is the default Linux bare-metal/VM profile. It writes the
  systemd unit and runs `systemctl enable --now sysarmor-agent`.
- `linux-container` is the Linux in-container profile. It skips systemd,
  writes `sensor.scope.type=namespace` and `sensor.scope.selector=self`, and
  prints the entrypoint command:

```bash
/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent/agent.yaml
```

Agent artifacts are signed distribution tarballs. The top-level
`manifest.json` describes the entrypoint, systemd unit, sensor bundles, and
file checksums; `manifest.sig` signs that manifest. The manager verifies the
artifact at upload time when `SYSARMOR_ARTIFACT_PUBLIC_KEY` is configured, and
the bootstrap script verifies the same manifest before installing.

Local deploy also builds an agent release:

```bash
make release
make deploy
```

`make release` writes the signed agent package and package index under
`dist/release/`. The compose stack serves that directory from the
`sysarmor-packages` container, and the manager imports
`SYSARMOR_AGENT_PACKAGE_INDEX_URL=http://packages/index.json` on startup. This
keeps the production shape clear: the packages service hosts immutable bytes,
while the manager owns artifact metadata, channel selection, enrollment, and
install script rendering. A production deployment can replace `packages` with
S3, MinIO, OSS, GCS, or a CDN as long as it exposes the same package index
schema.

The manager uses two URLs for this path:

- `SYSARMOR_AGENT_PACKAGE_INDEX_URL`: manager-side package index discovery URL.
  In compose this is the internal service URL `http://packages/index.json`.
- `SYSARMOR_AGENT_PACKAGE_DOWNLOAD_BASE_URL`: agent-side download base URL for
  non-container profiles. In local compose this defaults to
  `http://127.0.0.1:18080`. Container profile enrollments keep the internal
  `http://packages/...` URL because the agent container joins the compose
  network.

The default gateway path requires agent-plane mTLS. The Agent enrollment RPC
generates the endpoint private key locally, submits a CSR with the enrollment
token, and stores the manager-issued certificate under its state directory.
The gateway validates the certificate URI SAN against the reported
`tenant_id/agent_id`.

Services:

- `manager`: operator-facing control, audit, policy, and query API.
- `packages`: static file service for signed agent packages and package index.
- `gateway`: agent-facing data/control gRPC endpoint.
- `worker`: Kafka ingest consumer for detection, incident projection, and indexing.
- `postgres`: control, state, and audit store.
- `kafka`: durable telemetry ingest log.
- `redis`: gateway hot state.
- `opensearch`: searchable events, signals, and evidence layer.
- `opensearch-init`: one-shot versioned-index and alias initialization that must
  complete before Manager and Worker start.

Manager authentication is always enabled. Manager trusts only short-lived JWTs
signed by the Manager UI BFF:

```text
SYSARMOR_JWT_PUBLIC_KEY_FILE=/etc/sysarmor/pki/manager-jwt-public.pem
SYSARMOR_JWT_ISSUER=sysarmor-bff
SYSARMOR_JWT_AUDIENCE=sysarmor-manager
```

Initialize the one bootstrap admin and deploy the platform:

```bash
make auth-init
make deploy
make doctor
```

The initial username and password are stored as mode `0600` files under
`deployments/pki/agent-plane-mtls/runtime/`. Initialization never overwrites
them. The UI is available at `http://127.0.0.1:4173`; its BFF is the only
browser path to Manager APIs. Future OIDC providers attach to Auth.js and keep
the same BFF-to-Manager contract; Manager does not implement an OIDC mode.
When publishing the UI through a reverse proxy, restrict the accepted `Host`
header to the configured SysArmor UI hostname.

Ports:

- Manager HTTP: `19443`
- Manager UI: `4173`
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
