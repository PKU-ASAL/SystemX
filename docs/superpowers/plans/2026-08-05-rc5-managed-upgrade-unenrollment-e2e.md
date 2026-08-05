# rc.5 Managed State Upgrade Unenrollment E2E Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a deterministic VM topology test proving that a `v0.1.0-rc.5` schema v1 managed Agent state migrates to the current schema and completes a fail-closed legacy mTLS unenrollment.

**Architecture:** Keep production code unchanged. Add a versioned, secret-free SQL fixture with a manifest contract, then run a focused legacy scenario from the existing topology suite using runtime-issued credentials and a test-only PostgreSQL projection update that reproduces a pre-protocol certificate record.

**Tech Stack:** Python 3 `unittest` and `sqlite3`, Bash, Vagrant/libvirt, systemd, PostgreSQL 16, Go package regression tests.

## Global Constraints

- Compatibility baseline is tag `v0.1.0-rc.5`, commit `454b69d6c01f778add5836e0af1c9ba3299fd5b1`.
- Do not commit certificates, private keys, enrollment tokens, completion tokens, WAL files, or machine-specific data.
- Do not add production APIs or modify `EnrollmentCoordinator`, Store, Gateway, or protocol behavior.
- The test verifies managed persisted-state upgrade, not a full old-platform rolling binary upgrade.
- Legacy certificate projection writes must match exactly one tenant and serial or fail.
- The Manager terminal state must be `unknown_legacy`, never `endpoint_completed`.
- The Agent may enter standalone only after the real Gateway mTLS revocation succeeds.

---

### Task 1: Lock the fixture and topology scenario contract

**Files:**
- Modify: `test/suites/functional/topology/test_e2e_contract.py`
- Test: `test/suites/functional/topology/test_e2e_contract.py`

**Interfaces:**
- Consumes: the design contract and existing `e2e-systemd-vm.sh` source.
- Produces: static assertions for fixture provenance, schema v1 shape, scenario integration, migration, legacy revocation, Manager projection, credential cleanup, and restart recovery.

- [ ] **Step 1: Write the failing fixture contract**

Add imports for `hashlib`, `json`, and `sqlite3`, then add a helper and test:

```python
def fixture_dir():
    return (
        Path(__file__).resolve().parents[3]
        / "fixtures/agent/upgrades/v0.1.0-rc.5"
    )


def test_rc5_managed_fixture_is_authentic_schema_v1(self):
    root = fixture_dir()
    manifest = json.loads((root / "manifest.json").read_text())
    fixture = root / manifest["fixture_file"]
    raw = fixture.read_bytes()

    self.assertEqual(manifest["format"], "sysarmor.agent-upgrade-fixture/v1")
    self.assertEqual(manifest["source_tag"], "v0.1.0-rc.5")
    self.assertEqual(
        manifest["source_commit"],
        "454b69d6c01f778add5836e0af1c9ba3299fd5b1",
    )
    self.assertEqual(manifest["schema_version"], 1)
    self.assertEqual(hashlib.sha256(raw).hexdigest(), manifest["sha256"])

    db = sqlite3.connect(":memory:")
    self.addCleanup(db.close)
    db.executescript(raw.decode())
    self.assertEqual(db.execute("SELECT version FROM schema_meta").fetchone(), (1,))
    self.assertEqual(
        db.execute("SELECT state FROM enrollment WHERE singleton=1").fetchone(),
        ("managed",),
    )
    columns = {row[1] for row in db.execute("PRAGMA table_info(enrollment)")}
    self.assertTrue(
        {"enrollment_id", "certificate_serial", "manager_url", "unenrollment_protocol"}.isdisjoint(columns)
    )
```

- [ ] **Step 2: Write the failing topology integration contract**

Add a test that reads both scripts and asserts these stable behavior markers:

```python
def test_vm_topology_verifies_rc5_managed_state_upgrade_unenrollment(self):
    suite = Path(__file__).resolve().parent
    main = (suite / "e2e-systemd-vm.sh").read_text()
    legacy = (suite / "legacy-managed-upgrade-unenrollment.sh").read_text()

    self.assertIn("legacy-managed-upgrade-unenrollment.sh", main)
    for marker in (
        "v0.1.0-rc.5",
        "legacy_mtls",
        "unknown_legacy",
        "managed enrollment credentials removed",
        "standalone policy after legacy unenrollment restart",
    ):
        self.assertIn(marker, legacy)
```

- [ ] **Step 3: Run the contract test and verify RED**

Run:

```bash
python3 -m unittest test.suites.functional.topology.test_e2e_contract -v
```

Expected: the new tests fail because the fixture directory and legacy scenario script do not exist.

---

### Task 2: Add the secret-free rc.5 schema v1 fixture

**Files:**
- Create: `test/fixtures/agent/upgrades/v0.1.0-rc.5/agent-managed-v1.sql`
- Create: `test/fixtures/agent/upgrades/v0.1.0-rc.5/manifest.json`
- Create: `test/fixtures/agent/upgrades/v0.1.0-rc.5/README.md`
- Test: `test/suites/functional/topology/test_e2e_contract.py`

**Interfaces:**
- Consumes: the exact `baselineSchema` from `v0.1.0-rc.5:internal/agent/localstore/schema.go`.
- Produces: a deterministic SQL database seed with `schema_meta.version=1`, managed enrollment, old single policy, and no credential bytes.

- [ ] **Step 1: Add the canonical SQL dump**

Copy the complete rc.5 baseline table and index definitions, then append deterministic seed rows:

```sql
INSERT INTO schema_meta(version) VALUES (1);
INSERT INTO device_identity(singleton,device_id,host_id,created_at_ns)
VALUES (1,'rc5-device-fixture','rc5-host-fixture',1);
INSERT INTO enrollment(singleton,state,tenant_id,agent_id,gateway_address,tls_ca_path,
  tls_cert_path,tls_key_path,tls_server_name,upload_history,managed_from_seq,updated_at_ns)
VALUES (1,'managed','default','rc5-agent-fixture','10.66.0.10:9444',
  '/fixture/ca.pem','/fixture/agent.pem','/fixture/agent-key.pem',
  'sysarmor-gateway.local',0,1,1);
INSERT INTO policy(kind,version,document_json,digest,updated_at_ns)
VALUES ('endpoint',1,
  '{"policy_id":"rc5-managed-policy","version":1,"mode":"observe","collection":{"behaviors":["process.exec"],"observe_only":true}}',
  'rc5-managed-policy-fixture',1);
```

- [ ] **Step 2: Add provenance documentation and manifest**

Document that the fixture contains the rc.5 baseline schema plus deterministic non-secret managed rows, that it is mutated only after import into a VM copy, and that changing it requires updating the digest. Compute the digest with:

```bash
sha256sum test/fixtures/agent/upgrades/v0.1.0-rc.5/agent-managed-v1.sql
```

Write the emitted lowercase digest into `manifest.json` together with the exact format, source tag, source commit, schema version, and fixture filename from the global constraints.

- [ ] **Step 3: Run the fixture contract**

Run:

```bash
python3 -m unittest test.suites.functional.topology.test_e2e_contract.TopologyE2EContractTest.test_rc5_managed_fixture_is_authentic_schema_v1 -v
```

Expected: PASS. The topology integration contract remains RED because the scenario script is still missing.

- [ ] **Step 4: Commit the fixture contract**

```bash
git add test/fixtures/agent/upgrades/v0.1.0-rc.5 test/suites/functional/topology/test_e2e_contract.py
git commit -m "test(agent): add rc5 managed state fixture"
```

---

### Task 3: Execute the legacy state upgrade through the real topology

**Files:**
- Create: `test/suites/functional/topology/legacy-managed-upgrade-unenrollment.sh`
- Modify: `test/suites/functional/topology/e2e-systemd-vm.sh`
- Test: `test/suites/functional/topology/test_e2e_contract.py`

**Interfaces:**
- Consumes: `MANAGER_JWT`, `MANAGER_CTL`, `RESULTS`, `VM_ENV`, `ROOT`, and the published `topology-test` channel from the parent script.
- Produces: result files prefixed `e2e-agent-systemd-vm.legacy-` and five boolean/string summary fields consumed by the parent summary generator.

- [ ] **Step 1: Implement strict scenario input and enrollment setup**

The child script uses `set -euo pipefail`, validates inherited values, creates Agent ID `vm-legacy-rc5`, obtains an install URL through `manager enrollments create --channel topology-test`, and installs through the existing signed artifact flow. It stops the service immediately after enrollment is issued.

- [ ] **Step 2: Import and bind the rc.5 fixture copy**

Upload `agent-managed-v1.sql` to node-a. Use VM Python `sqlite3` to create `/var/lib/sysarmor/agent/agent.db` with mode `0600`, then update only schema v1 columns with the runtime Agent ID, Gateway address, TLS credential paths, server name, and timestamp. Preserve the installed bootstrap policy, configuration, and credential directory.

- [ ] **Step 3: Mark the Manager certificate projection as legacy**

Read the issued enrollment ID and certificate serial from Manager enrollment JSON. Pass the numeric serial through a validated
`psql` variable and execute:

```bash
docker exec sysarmor-postgres psql -U sysarmor -d sysarmor \
  --set=serial="$CERT_SERIAL" --tuples-only --no-align --command \
  "UPDATE agent_certificates
   SET data = data - 'unenrollment_protocol'
   WHERE tenant_id = 'default' AND serial_number = :'serial'
   RETURNING serial_number;"
```

The SQL operation is:

```text
UPDATE agent_certificates
SET data = data - 'unenrollment_protocol'
WHERE tenant_id = 'default' AND serial_number = the validated psql serial variable
RETURNING serial_number;
```

Safely quote the runtime numeric serial and require exactly one returned row. Do not change structured identity columns.

- [ ] **Step 4: Start the current Agent and assert migration before unenrollment**

Start systemd and wait for managed health. Query the Agent database read-only and require:

```text
schema_meta.version = 5
enrollment.state = managed
enrollment.unenrollment_protocol = legacy_mtls
policy_activation.source = managed
```

- [ ] **Step 5: Execute and assert real legacy unenrollment**

Run `sysarmorctl --json unenroll --timeout 60s`, require `status=applied`, then assert Manager enrollment projection contains `unenrollment_status=unknown_legacy` and does not contain `endpoint_completed`. Require current local policy `standalone-default` and fail if any managed CA, certificate, or key remains.

- [ ] **Step 6: Restart and assert durable standalone recovery**

Restart `sysarmor-agent`, wait for `policy current` to return `standalone-default`, and write a compact JSON result containing:

```json
{
  "legacy_fixture_source": "v0.1.0-rc.5",
  "legacy_schema_migrated": true,
  "legacy_mtls_unenrollment_applied": true,
  "manager_legacy_status_unknown": true,
  "legacy_standalone_after_restart": true
}
```

- [ ] **Step 7: Wire the child scenario into the parent summary**

Invoke the child script after the existing completion_v1 restart assertion. Load its JSON result in the existing summary Python block and copy the five fields into `e2e-agent-systemd-vm.summary.json`.

- [ ] **Step 8: Run the static topology contract and verify GREEN**

Run:

```bash
python3 -m unittest test.suites.functional.topology.test_e2e_contract -v
```

Expected: all topology contract tests PASS.

- [ ] **Step 9: Commit the topology scenario**

```bash
git add test/suites/functional/topology/e2e-systemd-vm.sh \
  test/suites/functional/topology/legacy-managed-upgrade-unenrollment.sh
git commit -m "test(e2e): verify rc5 managed state unenrollment"
```

---

### Task 4: Run regression and deployment-shaped acceptance

**Files:**
- Verify only; no expected source changes.

**Interfaces:**
- Consumes: all artifacts from Tasks 1-3.
- Produces: evidence that static, package-level, and VM topology behavior are green.

- [ ] **Step 1: Run focused package regression**

```bash
go test ./internal/agent/localstore ./internal/agent/daemon ./internal/gateway ./internal/store ./internal/store/postgres -count=1
```

Expected: PASS.

- [ ] **Step 2: Run repository regression**

```bash
go test ./... -count=1
go vet ./...
```

Expected: PASS with no vet diagnostics.

- [ ] **Step 3: Run real VM topology**

```bash
make test-functional DOMAIN=topology
```

Expected: the normal completion_v1 scenario and the rc.5 managed-state legacy scenario both pass, and the summary contains all five legacy success fields.

- [ ] **Step 4: Check final diff hygiene**

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only pre-existing untracked `bin/` and `test/environments/vm-topology/deploy/` generation outputs remain.
