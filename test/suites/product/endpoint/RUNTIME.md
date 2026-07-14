# Agent Runtime Smoke

Agent runtime smoke coverage lives primarily in focused Go tests. Shell runners
are retained only when they exercise a real process, container, VM, or sensor
boundary that cannot be expressed clearly in-process.

It validates:

- daemon startup with fake sensor input;
- sensor restart and recovery health semantics;
- startup capability failures such as bundle checksum, BTF, and bpffs;
- sensor parse/drop health degradation;
- container and VM managed fake sensor startup/restart/recovery behavior.

Standalone tests query the local Unix socket. Cloud-visible health and session
state are tested only after enrollment through a product platform scenario.

Smoke script groups:

| Entry | Boundary | Purpose |
|---|---|---|---|
| `go test ./internal/agent/daemon` | in-process Agent | daemon, labels, parse/drop health, tamper Signal. |
| `go test ./internal/sensors/linux/tetragon` | sensor process | restart, recovery, capability, and parsing. |
| `capability.sh` | real Agent process | bundle, BTF, and bpffs startup rejection. |
| `e2e-systemd-vm.sh`, `e2e-namespace-self-container.sh` | Manager installer | managed installation and cloud visibility. |
| `e2e-real-tetragon-owned-*.sh` | real Tetragon | owned sensor path. |

Run the current public endpoint entrypoint with:

```bash
make -C test product-endpoint
```
