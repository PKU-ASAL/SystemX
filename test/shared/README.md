# Shared Test Utilities

`test/shared/` contains mechanisms reused by more than one suite. Product
assertions stay in `test/suites/`; reusable inputs stay in `test/data/`.

| Area | Responsibility |
|---|---|
| `agent/` | Install, enroll, inspect, and apply policy in test environments |
| `harness/` | Container and VM lifecycle, waiting, ports, and cleanup |
| `recorder/` | Endpoint timeline, CPU/RSS, event, signal, and phase capture |
| `reports/` | Shared report generation and effectiveness assertions |
| `diagnostics/` | On-demand process, sensor, and runtime diagnostics |
| `vm/` | VM-specific binary and configuration synchronization |

Helpers provide mechanism, return failures explicitly, and do not encode one
scenario's expected security result. Endpoint recording samples Agent and
sensor processes on `node-a`; platform resource sampling belongs to
`performance-platform` and samples services on `mgr`.
