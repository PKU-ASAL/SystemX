# Agent Test Suite Convergence Design

## Conclusion

Migrate the remaining legacy Agent test assets by preserving behavioral
coverage, not file identity. Replace duplicated configuration and lifecycle
code with four narrow shared helpers, parameterize equivalent scenarios, and
delete an old script only after every assertion has a passing replacement.

The test suite must use the same product path as a real deployment:

```text
install standalone Agent -> wait for Unix socket -> optionally enroll -> test -> inspect
```

No test-only compatibility parser, legacy schema converter, direct SQLite
access, or alternate managed startup path is permitted.

## Scope

This convergence covers the remaining scripts under `test/` that reference
legacy Agent configuration keys or filesystem paths. It includes product,
performance, topology, VM, container, capture, diagnosis, and recorder assets.

Generated VM deployment mirrors under
`test/environments/vm-topology/deploy/platform/` are not edited directly.
`test/shared/harness/start-vm.sh` regenerates that tree from the repository, so
the source test assets are the only ownership boundary.

## Coverage Contract

Before changing scripts, create a machine-readable inventory with one record
per legacy script:

```text
legacy script -> capabilities -> assertions -> replacement scenario -> status
```

Capabilities are grouped into four mutually exclusive areas:

1. Agent lifecycle: install, startup, systemd, restart, recovery, capability,
   and health behavior.
2. Data and detection: Event and Signal production, parsing, drops, policy
   effects, namespace scope, and local query behavior.
3. Endpoint-to-cloud: enrollment, export, Gateway ingestion, Manager query,
   disconnect, restart, and recovery behavior.
4. Operations: capture, diagnosis, recorder, profiling, and performance
   measurement.

An old script may be deleted only when all of its assertions are represented
by passing replacement scenarios. File count is not a coverage metric.

## Target Structure

```text
test/
├── shared/agent/
│   ├── install.sh
│   ├── enroll.sh
│   ├── policy.sh
│   └── inspect.sh
├── fixtures/
│   └── agent/
│       ├── configs/
│       └── policies/
├── contracts/
│   └── agent-test-coverage.tsv
└── suites/
    ├── product/
    ├── performance/
    └── diagnostics/
```

The helpers are deliberately procedural shell libraries, not a test
framework. They expose only operations repeated by at least two retained
scenarios.

## Shared Helper Boundaries

### `install.sh`

Owns installation, runtime layout, process lifecycle, and readiness. It:

- installs an Agent package or explicit test binaries;
- writes a strict current `agent.yaml` and complete bootstrap `policy.json`;
- starts through systemd, a container process, or a foreground test process;
- waits for `/run/sysarmor/agent/control.sock` or an explicitly isolated test
  socket;
- never creates cloud identity or credentials.

### `enroll.sh`

Owns the optional cloud transition. It creates or consumes an enrollment token
and invokes `sysarmorctl enroll` through the Agent Unix socket. Unenrollment is
performed through the same local API. Tests must not write Agent credentials or
certificate files themselves.

### `policy.sh`

Owns test EndpointPolicy fixtures and local policy application. Every
bootstrap document contains `collection`, `detection`, `telemetry`, and
`response`. The helper may compose a complete document from repository-owned
current-schema fragments, but it must reject legacy collection-only documents
and must not translate old field names.

### `inspect.sh`

Owns health waits, Event and Signal queries, bounded log capture, and diagnostic
artifact collection. It talks to the Agent through `sysarmorctl`; it never
opens the Agent SQLite database. Raw segment inspection is allowed only in the
dedicated local-store format tests.

## Fixture Rules

Runtime fixtures contain only values whose behavior is under test. Product
defaults provide storage limits, retry timing, batching, and compression unless
a performance or boundary test explicitly varies one of them.

All runtime fixtures use the current ownership model:

```yaml
local:
  state_path: /var/lib/sysarmor/agent

control:
  socket_path: /run/sysarmor/agent/control.sock

sensor:
  backend: tetragon

policy:
  path: /etc/sysarmor/agent/policy.json
```

They never contain `manager`, Agent identity, token, certificate paths,
`data_plane`, old telemetry names, or `sensor.policy_path`.

EndpointPolicy fixtures use the current four-section schema. Scenario-specific
collection selectors belong in `collection`; sensor process options remain in
runtime config; cloud identity and endpoints come only from enrollment.

## Scenario Convergence

Consolidation follows behavior, not environment name:

- capability, BTF, and bpffs cases share one parameterized capability scenario;
- sensor restart and sensor recovery share one lifecycle scenario with an
  expected transition table;
- managed container restart and recovery share one enrolled-container scenario;
- apt, staged, and benign topology cases share one table-driven scenario runner;
- VM and container variants remain separate only where the isolation boundary
  itself is asserted;
- capture, diagnosis, and recorder become operational commands over the common
  install and inspect helpers rather than independent Agent launchers.

Real Tetragon, fake sensor, namespace isolation, systemd installation, local
store, and endpoint-to-cloud tests remain distinct because they validate
different system boundaries.

## Migration Order

1. Add the coverage inventory and a repository contract test that rejects
   legacy paths and schema keys in source test assets.
2. Add current-schema fixtures and unit-test the four shared helpers with
   temporary directories and fake commands.
3. Migrate local fake-sensor and capability scenarios.
4. Migrate real Tetragon, namespace, restart, and recovery scenarios.
5. Migrate enrollment, Gateway, Manager, topology, and systemd scenarios.
6. Migrate performance, capture, diagnosis, and recorder tooling.
7. Delete fully covered scripts, regenerate the VM deployment mirror, and run
   the complete test matrix.

Each batch must leave its retained scenarios runnable. Production code must not
gain compatibility behavior to make a test pass.

## Failure And Diagnostics

Shared waits use explicit deadlines and print the last Agent health response,
systemd or process status, bounded Agent logs, and relevant Gateway or Manager
logs on timeout. Helpers return non-zero on malformed fixtures, missing tools,
failed enrollment, and failed readiness checks; they never silently continue.

Temporary state is isolated per scenario. Cleanup is idempotent and preserves
diagnostic artifacts after failures. Secrets and enrollment tokens are never
printed or stored in result artifacts.

## Test Entry Points

`test/Makefile` remains the public interface. It exposes capability-oriented
targets rather than individual legacy filenames:

```text
product-endpoint-standalone
product-endpoint
product-endpoint-namespace-container
product-topology
product-platform
performance-endpoint
performance-local-store
capture-endpoint
capture-topology
diag-endpoint
recorder-*
```

Each target invokes retained scenario runners through the shared helpers. No
Make target may reference a deleted script.

## Acceptance

The migration is complete only when:

- every legacy assertion maps to a passing replacement in the coverage
  inventory;
- all public `test/Makefile` targets resolve to existing current-schema assets;
- source tests contain no legacy config keys or filesystem paths;
- the generated VM deployment mirror contains no legacy config keys or paths
  after regeneration;
- standalone, namespace, real Tetragon, systemd, enrollment, cloud export,
  restart, recovery, topology, diagnostics, recorder, and performance coverage
  all pass;
- `go test ./...`, `go vet ./...`, Agent race tests, UI tests, and UI production
  build continue to pass;
- deleted-script coverage records identify their replacements and remain in the
  inventory as historical traceability.

The expected result is approximately 10 to 14 scenario scripts plus four small
shared helpers. This number is a design guardrail, not a reason to merge tests
with different behavioral boundaries.
