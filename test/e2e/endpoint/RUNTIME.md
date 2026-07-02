# Agent Runtime Suite

This suite owns agent runtime smoke tests for local, container, and VM
topologies. Fake-sensor tests are preferred for fast local gates; container and
VM tests cover packaging, service lifecycle, and managed sensor behavior.

It validates:

- daemon startup with fake sensor input;
- sensor restart and recovery health semantics;
- startup capability failures such as bundle checksum, BTF, and bpffs;
- sensor parse/drop health degradation;
- data-plane append session cursor behavior.
- container and VM managed sensor startup/restart/recovery behavior.

This suite may start a memory manager and query it with `sysarmorctl manager`
when a runtime smoke needs manager-visible health or session state. It should
not own cloud analytics, incident semantics, storage projection, or VM real
Tetragon benchmarks.

Entrypoint:

```bash
make -C test test-agent-runtime
```

Focused targets:

```bash
make -C test e2e-agent-daemon
make -C test e2e-agent-sensor-restart
make -C test e2e-agent-sensor-recover
make -C test e2e-agent-capability
make -C test e2e-agent-capability-btf
make -C test e2e-agent-capability-bpffs
make -C test e2e-agent-parse-health
make -C test e2e-agent-dropped-health
make -C test e2e-agent-session
make -C test e2e-agent-daemon-container
make -C test e2e-agent-managed-container
make -C test e2e-agent-managed-recover-container
make -C test e2e-agent-managed-restart-container
make -C test e2e-agent-systemd-vm
make -C test e2e-agent-managed-vm
make -C test e2e-agent-managed-recover-vm
```
