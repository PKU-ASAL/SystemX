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

    def test_agent_implementation_is_owned_by_agent_app(self):
        expected = (
            "apps/agent/internal/config",
            "apps/agent/internal/content",
            "apps/agent/internal/daemon",
            "apps/agent/internal/localstore",
            "apps/agent/internal/policy",
            "apps/agent/internal/tamper",
            "apps/agent/internal/telemetry",
            "apps/agent/internal/endpoint",
            "apps/agent/internal/sensors/fake",
            "apps/agent/internal/sensors/linux",
            "apps/agent/internal/sensors/runtime",
        )

        for path in expected:
            with self.subTest(path=path):
                self.assertTrue((self.repo / path).is_dir(), f"missing {path}")

    def test_legacy_agent_implementation_paths_are_absent(self):
        legacy = (
            "internal/agent",
            "internal/endpoint",
            "internal/sensors/fake",
            "internal/sensors/linux",
            "internal/sensors/runtime",
        )

        for path in legacy:
            with self.subTest(path=path):
                self.assertFalse((self.repo / path).exists(), f"legacy path remains: {path}")

    def test_agent_does_not_import_manager_implementation(self):
        agent = self.repo / "apps/agent"
        violations = []
        for source in agent.rglob("*.go"):
            if "github.com/sysarmor/sysarmor-next-project/apps/manager/" in source.read_text():
                violations.append(str(source.relative_to(self.repo)))
        self.assertEqual([], violations, f"agent imports manager implementation: {violations}")

    def test_manager_implementation_is_owned_by_manager_app(self):
        expected = (
            "apps/manager/internal/api",
            "apps/manager/internal/auth",
            "apps/manager/internal/gateway",
            "apps/manager/internal/store",
            "apps/manager/internal/analytics",
            "apps/manager/internal/ingest",
            "apps/manager/internal/platform",
            "apps/manager/internal/distribution",
            "apps/manager/integration",
        )

        for path in expected:
            with self.subTest(path=path):
                self.assertTrue((self.repo / path).is_dir(), f"missing {path}")

    def test_legacy_manager_implementation_paths_are_absent(self):
        legacy = (
            "internal/manager",
            "internal/gateway",
            "internal/store",
            "internal/analytics",
            "internal/workers/ingest",
            "internal/platform",
            "internal/distribution",
            "test/suites/functional/platform/agent_gateway_manager_test.go",
        )

        for path in legacy:
            with self.subTest(path=path):
                self.assertFalse((self.repo / path).exists(), f"legacy path remains: {path}")

    def test_manager_does_not_import_agent_implementation(self):
        manager = self.repo / "apps/manager"
        violations = []
        for source in manager.rglob("*.go"):
            if "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/" in source.read_text():
                violations.append(str(source.relative_to(self.repo)))
        self.assertEqual([], violations, f"manager imports agent implementation: {violations}")

    def test_console_is_owned_by_apps(self):
        self.assertTrue((self.repo / "apps/console").is_dir(), "missing apps/console")
        self.assertFalse((self.repo / "web/manager").exists(), "legacy path remains: web/manager")

    def test_legacy_product_roots_are_absent(self):
        for path in ("cmd", "api", "internal", "web"):
            with self.subTest(path=path):
                self.assertFalse((self.repo / path).exists(), f"legacy product root remains: {path}")


if __name__ == "__main__":
    unittest.main()
