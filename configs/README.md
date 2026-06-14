# SysArmor Configs

This directory is reserved for policy and rule bundles once the MVP moves from compiled Go heuristics to content-driven detection.

Current MVP status:

- Endpoint rules are implemented in `internal/endpoint/fastpath`.
- Cloud rules and convergence controls are implemented in `internal/analytics/ingest`.
- Test-specific expectations and control checks live under `test/scenarios`.
- Runtime collection policy for e2e lives in `test/env/resources/syscall-capture.yaml`.

Planned layout:

```text
configs/
  policies/
  rules/
    endpoint/
    cloud/
```

Keep cross-component data contracts in `api/proto`; configs should reference those contracts rather than defining new schemas ad hoc.
