# SysArmor Configs

This directory contains the default v3 rule/policy content pack.

Current status:

- Endpoint rule execution still lives in `internal/endpoint/fastpath`.
- Cloud convergence still lives in `internal/analytics/ingest`.
- The manager can now store policies, assignments, and expose an effective policy API.
- Rule content files are metadata and enable/disable references for the current hardcoded rules.

Layout:

```text
configs/
  policies/
    default-edr-policy.json
  rules/
    endpoint/
      *.json
    cloud/
      *.json
```

Keep cross-component data contracts in `api/proto`; configs should reference those contracts rather than defining new schemas ad hoc.
