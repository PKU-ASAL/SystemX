import re
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

    def test_active_documentation_does_not_recommend_removed_commands(self):
        removed_commands = re.compile(
            r"make test-(?:functional-(?:endpoint|platform|topology)"
            r"|distribution-(?:package|published)"
            r"|release-(?:candidate|published))\b"
        )

        paths = [self.repo / "README.md", self.repo / "README.zh-CN.md"]
        paths.extend((self.repo / "docs").rglob("*.md"))
        paths.extend((self.repo / "test").rglob("*.md"))
        for path in paths:
            relative = path.relative_to(self.repo).as_posix()
            if (
                relative.startswith("docs/superpowers/")
                or relative.startswith("test/.results/")
                or relative.startswith(
                    "test/suites/distribution/published/results/"
                )
            ):
                continue
            with self.subTest(path=relative):
                self.assertNotRegex(path.read_text(), removed_commands)

    def test_help_uses_parameterized_public_commands(self):
        root_help = self.run_make("help")
        test_help = self.run_make("test-help")
        expected = (
            "make test-functional DOMAIN=endpoint",
            "make test-performance DOMAIN=endpoint",
            "make test-distribution SOURCE=local",
            "make test-release STAGE=pre-publish",
        )

        self.assertEqual(root_help.returncode, 0, root_help.stderr)
        self.assertEqual(test_help.returncode, 0, test_help.stderr)
        for command in expected:
            with self.subTest(command=command):
                self.assertIn(command, root_help.stdout)
                self.assertIn(command, test_help.stdout)

    def test_release_workflow_uses_public_distribution_dispatcher(self):
        workflow = (
            self.repo / ".github/workflows/release-build.yml"
        ).read_text()

        self.assertIn("make test-distribution SOURCE=local", workflow)
        self.assertNotIn("make -C test distribution-package", workflow)


if __name__ == "__main__":
    unittest.main()
