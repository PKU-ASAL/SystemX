# SysArmor Tests

SysArmor separates three questions because one test cannot answer all of them:

| Suite | Question | Primary entrypoint |
|---|---|---|
| Product | Does the component or product path work? | `make -C test product-*` |
| Effectiveness | Do malicious and benign scenarios produce the expected security results? | `make -C test effectiveness-topology` |
| Performance | What does the endpoint or platform cost under a stated workload? | `make -C test performance-*` |

`make -C test help` is the authoritative command list.

## Environments

| Environment | Shape | Use |
|---|---|---|
| `container` | Local Docker Compose | Fast platform contracts and container product paths |
| `vm-endpoint` | One protected VM | Real Agent/sensor behavior and endpoint CPU/RSS |
| `vm-topology` | `mgr`, `node-a`, `attacker` VMs | Distribution, enrollment, mTLS, end-to-end detection, and platform cost |

VM tests require KVM/libvirt and Vagrant. They create privileged infrastructure
and may download large images and sensor bundles.

## Recommended Checks

```bash
make test
make -C test product-endpoint-standalone
make -C test product-platform
make -C test product-topology
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=quick
make -C test effectiveness-topology
```

Use `quick` only to verify benchmark wiring. Resource conclusions require a
fresh, comparable `medium` or `long` run. Detection conclusions require the
truth-labelled effectiveness suite; product smoke tests are not evidence of
recall or precision.

## Inputs And Outputs

Reusable policies, workloads, and scenarios live under `test/data/`. Generated
captures and reports live under `test/.results/` and are ignored by Git.

Every reported result must identify its environment, policy, workload,
scenario, duration, and run ID. Compare runs only when those inputs and VM
lifecycle are equivalent.

See [Test Details](DETAILS.md) for lifecycle, result files, phase semantics,
and pass/fail boundaries.
