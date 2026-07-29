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

    def test_only_additional_test_content_is_applied(self):
        self.assertIn('vagrant upload "$REPO/test/data/content"', self.script)
        self.assertNotIn("ioc-c2-port-feed.json", self.script)

    def test_standalone_config_uses_labels_without_cloud_identity(self):
        self.assertIn("  label.scenario: $SCENARIO", self.script)
        self.assertIn("  path: /etc/sysarmor/agent/policy.json", self.script)
        self.assertNotIn("  transport: local", self.script)

    def test_control_commands_use_runtime_identity(self):
        self.assertIn(".agentId // .agent_id", self.script)
        self.assertIn(".tenantId // .tenant_id", self.script)
        self.assertNotIn("--agent-id vm-owned-tetragon", self.script)

    def test_current_policy_uses_installed_policy_id(self):
        self.assertIn('\'"policyId":"standalone-default"\'', self.script)
        self.assertNotIn("default-edr-policy", self.script)

    def test_single_node_environment_starts_c2_fixture(self):
        self.assertIn("ip address replace 10.66.0.99/32 dev lo", self.script)
        self.assertIn("systemd-run --unit sysarmor-test-c2-http", self.script)
        self.assertIn("systemctl stop sysarmor-test-c2-http", self.script)

    def test_attack_asserts_scenario_signals(self):
        self.assertIn('\'"name":"suspicious_exec_connect"\'', self.script)
        self.assertNotIn('\'"name":"payload_lifecycle"\'', self.script)

    def test_event_labels_are_checked_structurally(self):
        self.assertIn("select(.event.labels.scenario == $scenario)", self.script)
        self.assertNotIn('grep -Fq "\\\"labels\\\":{\\\"scenario\\\"', self.script)

    def test_attack_capture_uses_pre_attack_stream_cursors(self):
        self.assertIn("streams.eventNewestSequence", self.script)
        self.assertIn("streams.signalNewestSequence", self.script)
        self.assertIn('--include-recent --after-seq \'$EVENT_CURSOR\'', self.script)
        self.assertIn('--include-recent --after-seq \'$SIGNAL_CURSOR\'', self.script)


if __name__ == "__main__":
    unittest.main()
