# Agent Plane mTLS Sample

Generate sample material:

```bash
tools/pki/gen-agent-plane-mtls.sh deployments/pki/agent-plane-mtls/runtime default agent-prod-001 sysarmor-gateway.example.com
```

The canonical endpoint identity is the agent certificate URI SAN:

```text
spiffe://sysarmor.local/tenant/<tenant_id>/agent/<agent_id>
```

The gateway verifies the client certificate CA and rejects data/control frames whose tenant or agent ID does not match the certificate identity.

`runtime/` is intentionally ignored by git. Generate fresh material for local
compose or VM topology tests instead of committing private keys.
