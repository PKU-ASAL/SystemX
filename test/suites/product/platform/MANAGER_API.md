# Manager Cloud Suite

Scope: `platform` for API/storage/control semantics, and `topology` when a real
VM agent path is required.

This suite owns manager ingest/query, cloud signals, incidents, graph/evidence,
policy APIs, response APIs, and manager-facing agent health checks.

Entrypoints:

```bash
make -C test product-platform
make -C test product-platform-smoke
make -C test product-topology-smoke
make -C test product-platform-full
make -C test effectiveness-topology
```

`product-platform-smoke` is an explicit alias for the local contract/smoke
target.

`product-topology-smoke` uses a fake sensor and validates the VM mTLS/systemd/data
path only. Use `product-platform-full` or `effectiveness-topology` for real
Tetragon event/signal/incident coverage.

Endpoint resource conclusions are out of scope here; use `performance-endpoint` on
`vm-endpoint` for that.
