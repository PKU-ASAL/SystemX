# Manager Cloud Suite

Scope: `platform` for API/storage/control semantics, and `topology` when a real
VM agent path is required.

This suite owns manager ingest/query, cloud signals, incidents, graph/evidence,
policy APIs, response APIs, and manager-facing agent health checks.

Entrypoints:

```bash
make -C test product-platform
make -C test product-topology SCENARIO=apt-fileless-c2
```

Endpoint resource conclusions are out of scope here; use `performance-endpoint` on
`vm-endpoint` for that.
