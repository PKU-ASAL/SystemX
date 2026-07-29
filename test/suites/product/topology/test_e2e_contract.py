import unittest
from pathlib import Path


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


if __name__ == "__main__":
    unittest.main()
