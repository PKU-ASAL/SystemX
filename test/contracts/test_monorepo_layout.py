import unittest
from pathlib import Path


class MonorepoLayoutContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.repo = Path(__file__).resolve().parents[2]

    def test_executable_entrypoints_are_owned_by_apps(self):
        expected = (
            "apps/agent/cmd/sysarmor-agent",
            "apps/agent/cmd/sysarmor-content-sign",
            "apps/manager/cmd/sysarmor-manager",
            "apps/manager/cmd/sysarmor-gateway",
            "apps/manager/cmd/sysarmor-worker",
            "apps/cli/cmd/sysarmorctl",
        )

        for path in expected:
            with self.subTest(path=path):
                self.assertTrue((self.repo / path).is_dir(), f"missing {path}")

    def test_legacy_executable_entrypoints_are_absent(self):
        legacy = (
            "cmd/sysarmor-agent",
            "cmd/sysarmor-content-sign",
            "cmd/sysarmor-manager",
            "cmd/sysarmor-gateway",
            "cmd/sysarmor-worker",
            "cmd/sysarmorctl",
        )

        for path in legacy:
            with self.subTest(path=path):
                self.assertFalse((self.repo / path).exists(), f"legacy path remains: {path}")


if __name__ == "__main__":
    unittest.main()
