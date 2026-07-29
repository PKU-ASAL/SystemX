import unittest
from pathlib import Path


class EndpointE2EContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.script = (Path(__file__).parent / "e2e-real-tetragon-owned-vm.sh").read_text()

    def test_installer_receives_uploaded_ctl_binary(self):
        self.assertIn("SYSARMOR_CTL_BIN=/tmp/sysarmorctl.upload", self.script)
        self.assertIn(
            "SYSARMOR_COLLECTION_POLICY=/tmp/sysarmor-deployments.upload/agent/policy.json",
            self.script,
        )

    def test_product_content_is_uploaded_from_deployment_source(self):
        self.assertIn(
            'vagrant upload "$REPO/deployments/agent/content"', self.script
        )

    def test_standalone_config_uses_labels_without_cloud_identity(self):
        self.assertIn("  label.scenario: $SCENARIO", self.script)
        self.assertIn("  path: /etc/sysarmor/agent/policy.json", self.script)
        self.assertNotIn("  transport: local", self.script)


if __name__ == "__main__":
    unittest.main()
