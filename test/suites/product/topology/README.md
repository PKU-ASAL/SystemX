# Product Topology

`product-topology` validates distribution and enrollment across three VMs:

```text
mgr:      Manager, Gateway, Worker, and platform stores
node-a:   protected host receiving the Agent
attacker: C2 and scenario support
```

Run:

```bash
make -C test product-topology
```

Manager registers a signed Agent artifact, binds it to a channel, and creates a
one-time enrollment. `node-a` downloads and verifies the installer and package,
installs the systemd Agent, generates its key and CSR, obtains an mTLS
certificate, and connects to Gateway. The test then verifies Manager-visible
health/session state and Agent restart.

This proves:

```text
signed artifact -> channel -> enrollment -> verified install
-> certificate issuance -> systemd Agent -> authenticated platform connection
```

It does not prove kernel telemetry quality, attack recall/precision, or
platform resource cost. Use `effectiveness-topology` for detection and
`performance-platform` for platform resources.

Generated deployment caches live under `test/environments/vm-topology/deploy/`;
test results live under `test/.results/`.
