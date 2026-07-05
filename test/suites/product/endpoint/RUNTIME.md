# Agent Runtime Smoke

This folder still contains focused agent runtime smoke scripts. They are useful
for fast local gates, but they do not replace `product-endpoint`, performance
benchmarks, or detection effectiveness tests.

It validates:

- daemon startup with fake sensor input;
- sensor restart and recovery health semantics;
- startup capability failures such as bundle checksum, BTF, and bpffs;
- sensor parse/drop health degradation;
- container and VM managed fake sensor startup/restart/recovery behavior.

This suite may start a memory manager and query it with `sysarmorctl manager`
when a runtime smoke needs manager-visible health or session state. It should
not own cloud analytics, incident semantics, storage projection, or VM real
Tetragon benchmarks.

Smoke script groups:

| Scripts | Smoke | Sensor | Purpose |
|---|---|---|---|
| `e2e-daemon*.sh` | yes | fake input | agent daemon startup and local health. |
| `e2e-sensor-*.sh` | yes | fake sensor process | restart/recovery health semantics. |
| `e2e-capability*.sh` | yes | broken/fake bundle | startup capability failure semantics. |
| `e2e-parse-health.sh`, `e2e-dropped-health.sh` | yes | malformed/fake input | health degradation on parse/drop errors. |
| `e2e-managed-*.sh` | yes | fake Tetragon bundle | managed sensor lifecycle. |
| `e2e-real-tetragon-owned-*.sh` | no | owned real Tetragon | real owned sensor path. |

Run the current public endpoint entrypoint with:

```bash
make -C test product-endpoint
```
