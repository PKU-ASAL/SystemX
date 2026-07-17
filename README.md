# SysArmor

[English](README.md) | [简体中文](README.zh-CN.md)

SysArmor is an endpoint security and detection platform for Linux. It combines
a standalone-first Agent with an optional management plane for centralized
enrollment, telemetry processing, investigation, and response.

The project is under active development. It is suitable for development,
evaluation, and testing; interfaces and deployment procedures may change
before a stable release.

## Core Capabilities

- **Standalone endpoint operation:** the Agent starts locally without a
  Manager dependency and owns sensor lifecycle, collection, detection, and
  bounded local storage.
- **Explicit enrollment:** an endpoint connects to the management plane only
  after enrollment, using tenant- and Agent-bound mTLS identity.
- **Endpoint detection:** events are normalized and evaluated locally, with
  signals available through the Agent control socket.
- **Central analysis:** Gateway, Worker, and Manager services support durable
  ingestion, correlation, incidents, evidence, policy, and response workflows.
- **Reproducible validation:** container and VM suites cover product behavior,
  detection effectiveness, and endpoint or platform performance.

## Architecture

```text
Linux host
  -> Agent observes and analyzes host activity
  -> Events and detections remain available locally
  -> Enrolled endpoints send selected data to the management platform
  -> Security operators investigate through the web console
```

The Agent remains useful in standalone mode. Enrollment adds centralized
upload and control without creating a second endpoint data path. Inside the
management platform, Gateway receives Agent data, Kafka carries it reliably,
Worker performs further analysis, and PostgreSQL and OpenSearch store
management data and security data respectively.

## Prerequisites

The current development workflow targets systemd-based x86_64 Linux hosts.

- Go 1.26 or newer
- `make`, `curl`, and root access for Agent installation
- Docker with Docker Compose and `openssl` for the local platform
- KVM/libvirt and Vagrant only for VM-based test suites

Regenerating API bindings with `make api` additionally requires `protoc`,
`protoc-gen-go`, and `protoc-gen-go-grpc` on `PATH` or under
`$(go env GOPATH)/bin`.

Some build and installation commands download Go modules, OS packages, or the
Tetragon sensor bundle.

## Quick Start

### Standalone Agent

Build and install the Agent, CLI, default policy, and managed Tetragon bundle:

```bash
make install-agent
sudo sysarmorctl agent health
```

The Agent stores local state under `/var/lib/sysarmor/agent` and exposes its
control API at `/run/sysarmor/agent/control.sock`.

Remove the installation while retaining configuration and local data:

```bash
make uninstall-agent
```

Use `make uninstall-agent PURGE=1` only when configuration and local data
should also be removed.

### Local Platform

Build the binaries and release package, initialize local credentials, and
start the platform:

```bash
make deploy
make status
make doctor
```

Stop the platform with `make down`. See [deployment documentation](deployments/README.md)
for service layout, enrollment, mTLS, configuration, and operational commands.

## Development

Common repository commands:

```bash
make build-binary  # build Agent, Gateway, Manager, Worker, and sysarmorctl
make test          # run Go tests
make api           # regenerate protobuf bindings
make release       # build the signed Agent release package and index
```

The generated binaries are written to `dist/bin/`; release artifacts are
written to `dist/release/`. Both directories are reproducible and ignored by
Git.

For product, effectiveness, and performance suites:

```bash
make -C test help
make -C test product-endpoint-standalone
make -C test product-topology
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=quick
```

VM suites create privileged local infrastructure and may download large
artifacts. Review the test documentation before running them.

## Documentation

- [Repository layout](docs/architecture/repo-layout.md)
- [Agent runtime](docs/architecture/agent-runtime.md)
- [Platform runtime](docs/architecture/platform-runtime.md)
- [Deployment and enrollment](deployments/README.md)
- [Test guide](test/README.md)
- [Detailed test environments and reports](test/DETAILS.md)
- [Telemetry semantics](docs/architecture/telemetry-semantics.md)
- [Schema evolution](docs/architecture/schema-evolution.md)
- [Manager UI API contract](docs/architecture/manager-ui-api-contract.md)

The protobuf definitions under `api/proto/` are the source of truth for wire
contracts.

## Contributing

The project does not yet publish a formal contribution guide. Before starting
a substantial change, coordinate the scope with the maintainers and keep
changes focused, tested, and documented.

## License

This repository does not currently include a license file. No open-source
license grant should be assumed until one is published.
