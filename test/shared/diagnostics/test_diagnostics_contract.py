import unittest
from pathlib import Path


class DiagnosticsContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.diagnostics_dir = Path(__file__).resolve().parent

    def test_capture_recreates_work_directory_before_writing_config(self):
        script = (self.diagnostics_dir / "capture-vm.sh").read_text()

        remove = script.index(r'rm -rf \"$WORK\"')
        recreate = script.find(r'install -d -m 0755 \"$WORK\"', remove)
        write = script.index(r'cat > \"$WORK/agent.yaml\"', remove)

        self.assertNotEqual(recreate, -1)
        self.assertLess(recreate, write)

    def test_tetragon_diagnostics_uses_standard_agent_config(self):
        script = (self.diagnostics_dir / "diagnose-tetragon-vm.sh").read_text()

        self.assertIn(
            'AGENT_CONFIG="${SYSARMOR_AGENT_CONFIG:-/etc/sysarmor/agent/agent.yaml}"',
            script,
        )
        self.assertNotIn("/etc/sysarmor/agent.yaml", script)


if __name__ == "__main__":
    unittest.main()
