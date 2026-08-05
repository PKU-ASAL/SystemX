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

    def test_shared_capabilities_are_owned_by_packages(self):
        expected = (
            "packages/contracts/proto",
            "packages/contracts/schema",
            "packages/contracts/controlmodel",
            "packages/contracts/health",
            "packages/eventmodel",
            "packages/policy",
            "packages/response",
            "packages/sensor-sdk/contract",
            "packages/tlsconfig",
        )

        for path in expected:
            with self.subTest(path=path):
                self.assertTrue((self.repo / path).is_dir(), f"missing {path}")

    def test_legacy_shared_capability_paths_are_absent(self):
        legacy = (
            "api/proto",
            "internal/contracts/schema",
            "internal/controlmodel",
            "internal/agent/health",
            "internal/eventmodel",
            "internal/policy",
            "internal/response",
            "internal/sensors/contract",
            "internal/tlsconfig",
        )

        for path in legacy:
            with self.subTest(path=path):
                self.assertFalse((self.repo / path).exists(), f"legacy path remains: {path}")

    def test_packages_do_not_import_apps(self):
        packages = self.repo / "packages"
        if not packages.exists():
            self.fail("missing packages directory")

        violations = []
        for source in packages.rglob("*.go"):
            if "github.com/sysarmor/sysarmor-next-project/apps/" in source.read_text():
                violations.append(str(source.relative_to(self.repo)))
        self.assertEqual([], violations, f"packages import apps: {violations}")


if __name__ == "__main__":
    unittest.main()
