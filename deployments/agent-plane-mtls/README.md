# Agent Plane mTLS Sample

Generate sample material:

```bash
tools/pki/gen-agent-plane-mtls.sh ./pki default agent-prod-001 sysarmor-manager.example.com
```

The canonical endpoint identity is the agent certificate URI SAN:

```text
spiffe://sysarmor.local/tenant/<tenant_id>/agent/<agent_id>
```

The manager verifies the client certificate CA and rejects data/control frames whose tenant or agent ID does not match the certificate identity.
