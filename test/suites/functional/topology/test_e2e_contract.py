import hashlib
import json
import sqlite3
import unittest
from pathlib import Path


def rc5_fixture_dir():
    return (
        Path(__file__).resolve().parents[3]
        / "fixtures/agent/upgrades/v0.1.0-rc.5"
    )


class TopologyE2EContractTest(unittest.TestCase):
    def test_vm_topology_authenticates_manager_operations(self):
        script = (Path(__file__).resolve().parent / "e2e-systemd-vm.sh").read_text()

        self.assertIn(
            'tools/auth/issue-manager-jwt.sh" "$PKI_DIR/manager-jwt-private.pem" sysarmor-bff sysarmor-manager',
            script,
        )
        self.assertIn("Authorization: Bearer $MANAGER_JWT", script)
        self.assertIn(
            'MANAGER_CTL="SYSARMOR_MANAGER_JWT=\'$MANAGER_JWT\' /tmp/sysarmorctl"',
            script,
        )

    def test_vm_topology_asserts_issued_enrollment(self):
        script = (Path(__file__).resolve().parent / "e2e-systemd-vm.sh").read_text()

        self.assertIn("enrollments list --tenant-id default --status issued", script)
        self.assertNotIn("enrollments list --tenant-id default --status active", script)

    def test_container_harness_initializes_content_signing_key(self):
        harness = (
            Path(__file__).resolve().parents[3] / "shared/harness/start-container.sh"
        ).read_text()

        self.assertIn(
            'openssl genpkey -algorithm ED25519 -out "$PKI_DIR/content-signing-key.pem"',
            harness,
        )

    def test_agent_packages_use_test_content_signing_key(self):
        suite_dir = Path(__file__).resolve().parent

        for name in ("e2e-systemd-vm.sh", "scenario-container.sh"):
            with self.subTest(script=name):
                script = (suite_dir / name).read_text()
                self.assertIn(
                    '--content-signing-key "$PKI_DIR/content-signing-key.pem"', script
                )
                self.assertIn("--content-key-id topology-test", script)

    def test_vm_topology_closes_managed_policy_rollout(self):
        script = (Path(__file__).resolve().parent / "e2e-systemd-vm.sh").read_text()

        self.assertIn("manager policies assign", script)
        self.assertIn("--downlink", script)
        self.assertIn("topology-rollout-policy", script)
        self.assertIn("/api/v1/policy-rollouts", script)
        self.assertIn("rollout-pending.json", script)
        self.assertIn("rollout-applied.json", script)
        self.assertIn("rollout-after-restart.json", script)

    def test_vm_topology_waits_for_sensor_policy_before_workload(self):
        script = (Path(__file__).resolve().parent / "e2e-systemd-vm.sh").read_text()

        self.assertIn('"policy_loaded":true', script)
        self.assertIn("sudo /bin/true", script)
        self.assertIn("SYSARMOR_TOPOLOGY_WAIT_SECONDS:-120", script)

    def test_vm_topology_closes_online_unenrollment_and_restart(self):
        script = (Path(__file__).resolve().parent / "e2e-systemd-vm.sh").read_text()

        self.assertIn("sysarmorctl --json unenroll --timeout 60s", script)
        self.assertIn('"status":"applied"', script)
        self.assertIn("manager.tls_insecure", script)
        self.assertIn('"unenrollment_status":"endpoint_completed"', script)
        self.assertIn("e2e-agent-systemd-vm.enrollment-after-unenroll.json", script)
        self.assertIn('"policyId":"standalone-default"', script)
        self.assertIn("managed enrollment credentials removed", script)
        self.assertIn("standalone policy after unenrollment restart", script)

    def test_rc5_managed_fixture_is_authentic_schema_v1(self):
        root = rc5_fixture_dir()
        manifest_path = root / "manifest.json"
        self.assertTrue(manifest_path.exists(), f"missing rc.5 manifest: {manifest_path}")

        manifest = json.loads(manifest_path.read_text())
        fixture = root / manifest["fixture_file"]
        self.assertTrue(fixture.exists(), f"missing rc.5 fixture: {fixture}")
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
        new_fields = {
            "enrollment_id",
            "certificate_serial",
            "manager_url",
            "unenrollment_protocol",
        }
        self.assertTrue(new_fields.isdisjoint(columns))

if __name__ == "__main__":
    unittest.main()
