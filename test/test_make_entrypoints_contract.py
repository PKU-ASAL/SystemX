import subprocess
import unittest
from pathlib import Path


class TestMakeEntrypointsContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.repo = Path(__file__).resolve().parent.parent

    def run_make(self, *arguments, dry_run=False):
        command = ["make", "--no-print-directory"]
        if dry_run:
            command.append("-n")
        command.extend(arguments)
        return subprocess.run(
            command,
            cwd=self.repo,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_valid_selectors_delegate_to_internal_targets(self):
        cases = {
            ("test-functional", "DOMAIN=endpoint"): "functional-endpoint",
            ("test-functional", "DOMAIN=platform"): "functional-platform",
            ("test-functional", "DOMAIN=topology"): "functional-topology",
            ("test-functional", "DOMAIN=all"): "functional-core",
            ("test-performance", "DOMAIN=endpoint"): "performance-endpoint",
            ("test-performance", "DOMAIN=platform"): "performance-platform",
            ("test-performance", "DOMAIN=modules"): "performance-modules",
            ("test-distribution", "SOURCE=local"): "distribution-package",
            ("test-distribution", "SOURCE=published"): "distribution-published",
            ("test-release", "STAGE=pre-publish"): "release-candidate",
            ("test-release", "STAGE=post-publish"): "release-published",
        }

        for arguments, internal_target in cases.items():
            with self.subTest(arguments=arguments):
                result = self.run_make(*arguments, dry_run=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(internal_target, result.stdout)

    def test_performance_all_delegates_to_every_domain(self):
        result = self.run_make("test-performance", "DOMAIN=all", dry_run=True)

        self.assertEqual(result.returncode, 0, result.stderr)
        for internal_target in (
            "performance-endpoint",
            "performance-platform",
            "performance-modules",
        ):
            self.assertIn(internal_target, result.stdout)

    def test_missing_or_invalid_selectors_fail_with_usage(self):
        cases = (
            ("test-functional", "", "DOMAIN=endpoint|platform|topology|all"),
            ("test-functional", "DOMAIN=invalid", "DOMAIN=endpoint|platform|topology|all"),
            ("test-performance", "", "DOMAIN=endpoint|platform|modules|all"),
            ("test-performance", "DOMAIN=invalid", "DOMAIN=endpoint|platform|modules|all"),
            ("test-distribution", "", "SOURCE=local|published"),
            ("test-distribution", "SOURCE=invalid", "SOURCE=local|published"),
            ("test-release", "", "STAGE=pre-publish|post-publish"),
            ("test-release", "STAGE=invalid", "STAGE=pre-publish|post-publish"),
        )

        for target, selector, usage in cases:
            arguments = (target, selector) if selector else (target,)
            with self.subTest(arguments=arguments):
                result = self.run_make(*arguments)
                self.assertEqual(result.returncode, 2)
                self.assertIn(usage, result.stderr)

    def test_removed_long_targets_do_not_exist(self):
        targets = (
            "test-functional-endpoint",
            "test-functional-platform",
            "test-functional-topology",
            "test-distribution-package",
            "test-distribution-published",
            "test-release-candidate",
            "test-release-published",
        )

        for target in targets:
            with self.subTest(target=target):
                result = self.run_make(target, dry_run=True)
                self.assertEqual(result.returncode, 2)
                self.assertIn("No rule to make target", result.stderr)


if __name__ == "__main__":
    unittest.main()
