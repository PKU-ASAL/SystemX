# Deployments

This directory contains the Agent installer and the production-shaped local
platform. Test-only provisioning belongs under `test/`.

## Standalone Agent

On a systemd-based x86_64 Linux endpoint:

```bash
make install-agent
sudo sysarmorctl agent health
```

Installation writes:

```text
/opt/sysarmor/agent/                 Agent and managed Tetragon assets
/etc/sysarmor/agent/agent.yaml      runtime configuration
/etc/sysarmor/agent/policy.json     endpoint policy
/var/lib/sysarmor/agent/            local identity and bounded state
/run/sysarmor/agent/control.sock    local control API
```

The Agent starts in standalone mode and makes no platform connection. See
[Agent Runtime](../docs/architecture/agent-runtime.md) for the state and policy
model.

## Local Platform

`make deploy` builds binaries and a signed Agent release, initializes local
PKI and bootstrap credentials, builds service images, and starts Compose:

```bash
make deploy
make status
make doctor
```

The standard path is:

```text
Agent -> Gateway -> Kafka -> Worker -> PostgreSQL / OpenSearch
Browser -> Manager Console BFF -> Manager
```

| Service | Host port | Purpose |
|---|---:|---|
| Manager Console | `4173` | Browser UI and authenticated BFF |
| Manager | `19443` | Operator HTTP API |
| Gateway | `19444` | Agent mTLS gRPC |
| Gateway health | `19445` | Health endpoint |
| PostgreSQL | `15432` | Control-plane state |
| Kafka | `19092` | Durable telemetry handoff |
| Redis | `16379` | Gateway hot state |
| OpenSearch | `29200` | Telemetry and report projections |

Host ports may be overridden, for example:

```bash
SYSARMOR_OPENSEARCH_PORT=39200 make deploy
```

Use `make down` to stop services. `make reset` is destructive: it recreates
data volumes and the platform while preserving generated PKI.

## Authentication

`make auth-init` creates one local bootstrap administrator and Manager JWT
keys under `deployments/pki/agent-plane-mtls/runtime/`. Existing credentials
are not overwritten.

The browser holds an encrypted Auth.js session. Only the server-side BFF signs
short-lived RS256 Manager JWTs. Manager does not trust browser identity headers
or expose an alternate unauthenticated operator mode.

## Agent Distribution And Enrollment

`make release` writes a signed package and index to `dist/release/`. In the
local platform, the `packages` service hosts immutable bytes while Manager owns
artifact metadata, channels, enrollment, and installer rendering.

The enrollment sequence is:

```text
upload artifact -> bind channel -> create one-time enrollment
-> endpoint downloads and verifies installer/package
-> endpoint generates key and CSR
-> Manager issues tenant/Agent-bound certificate
-> Agent enables Gateway upload and control
```

Manager stores only the enrollment-token hash. The private key remains on the
endpoint. The Gateway verifies the certificate URI against the tenant and
Agent ID in each frame.

Use the Manager Console Deploy page to select an artifact and channel and
create an enrollment. For a new endpoint, run the installer command displayed
by the Console. It contains the one-time enrollment URL and installs the
selected signed artifact.

Direct enrollment of an already installed standalone Agent requires an
authenticated operator API flow. The current Console does not expose a
standalone enrollment command, so this document does not present manual token
extraction as a supported workflow.

By default, enrollment uploads only data created after the enrollment
boundary. Add `--upload-history` only when local history should be uploaded.
Return to standalone mode with:

```bash
sudo sysarmorctl unenroll
```

## Installation Profiles

- `linux-systemd` installs and enables the systemd service for a host or VM.
- `linux-container` skips systemd, sets `sensor.scope` to `namespace/self`, and
  returns this entrypoint:

```bash
/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent/agent.yaml
```

Both profiles install the same signed Agent distribution. Profile choice
changes lifecycle and sensor scope, not Agent identity or data contracts.

## Deployment Assets

| Path | Purpose |
|---|---|
| `agent/` | Installer, defaults, policy, and systemd unit |
| `packages/` | Signed release builder |
| `gateway/`, `manager/`, `worker/` | Service images and example environment |
| `manager-ui/` | Console image and runtime entrypoint |
| `infra/` | Kafka, PostgreSQL, Redis, and OpenSearch images |
| `opensearch/` | Versioned mappings and alias initialization |
| `pki/` | Local Agent-plane mTLS examples |
| `sensors/` | Managed sensor bundle installer |

`gateway --local-ingest` is a smoke-test fixture and is not used by the
standard Compose deployment.
