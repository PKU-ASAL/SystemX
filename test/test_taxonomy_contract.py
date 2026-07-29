import re
import unittest
from pathlib import Path


class TestTaxonomyContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.repo = Path(__file__).resolve().parent.parent
        cls.test_root = cls.repo / "test"
        cls.test_makefile = (cls.test_root / "Makefile").read_text()
        cls.root_makefile = (cls.repo / "Makefile").read_text()
        cls.release_workflow = (
            cls.repo / ".github/workflows/release-build.yml"
        ).read_text()

    def test_suite_directories_follow_unified_taxonomy(self):
        expected = [
            "suites/functional/endpoint",
            "suites/functional/platform",
            "suites/functional/topology",
            "suites/detection/topology",
            "suites/distribution/package",
            "suites/distribution/published",
        ]

        for relative in expected:
            with self.subTest(relative=relative):
                self.assertTrue((self.test_root / relative).is_dir(), relative)

    def test_test_makefile_exposes_new_taxonomy(self):
        targets = [
            "functional-endpoint",
            "functional-platform",
            "functional-topology",
            "detection-topology",
            "distribution-package",
            "distribution-published",
            "release-candidate",
            "release-published",
        ]

        for target in targets:
            with self.subTest(target=target):
                self.assertRegex(
                    self.test_makefile,
                    rf"(?m)^{re.escape(target)}(?:\s*:[^\n]*)$",
                )

    def test_legacy_targets_delegate_to_new_taxonomy(self):
        aliases = {
            "product-endpoint": "functional-endpoint",
            "product-endpoint-standalone": "functional-endpoint-local distribution-package",
            "product-endpoint-release-container": "distribution-published",
            "product-endpoint-namespace-container": "functional-endpoint-container",
            "product-platform": "functional-platform",
            "product-platform-smoke": "functional-platform",
            "product-platform-full": "functional-platform-full",
            "product-topology": "functional-topology",
            "effectiveness-topology": "detection-topology",
            "effectiveness-report": "detection-report",
            "test-all": "test-unit functional-core",
        }

        for legacy, replacement in aliases.items():
            with self.subTest(legacy=legacy):
                self.assertRegex(
                    self.test_makefile,
                    rf"(?m)^{re.escape(legacy)}\s*:\s*{re.escape(replacement)}\s*$",
                )

    def test_root_makefile_exposes_public_taxonomy(self):
        targets = [
            "test-functional",
            "test-detection",
            "test-performance",
            "test-distribution",
            "test-release",
        ]

        for target in targets:
            with self.subTest(target=target):
                self.assertRegex(self.root_makefile, rf"(?m)^{re.escape(target)}\s*:")

        removed = [
            "test-functional-endpoint",
            "test-functional-platform",
            "test-functional-topology",
            "test-distribution-package",
            "test-distribution-published",
            "test-release-candidate",
            "test-release-published",
        ]
        for target in removed:
            with self.subTest(removed=target):
                self.assertNotRegex(
                    self.root_makefile,
                    rf"(?m)^\.PHONY:.*\b{re.escape(target)}\b|^{re.escape(target)}\s*:",
                )

    def test_release_workflow_uses_distribution_paths(self):
        self.assertNotIn("test/suites/product/", self.release_workflow)
        self.assertNotIn("test/release/", self.release_workflow)
        self.assertIn("make -C test distribution-package", self.release_workflow)

    def test_detection_reports_use_current_taxonomy(self):
        reports = self.test_root / "shared/reports"

        self.assertTrue((reports / "detection_report.py").is_file())
        self.assertTrue((reports / "assert_detection.py").is_file())
        self.assertFalse((reports / "effectiveness_report.py").exists())
        self.assertFalse((reports / "assert_effectiveness.py").exists())

    def test_active_files_do_not_use_legacy_taxonomy(self):
        forbidden = (
            "suites/product/",
            "suites/effectiveness/",
            "effectiveness_report.py",
            "assert_effectiveness.py",
            "SYSARMOR_EFFECTIVENESS_MIN_SCORE",
            "Product 测试",
            "Effectiveness 测试",
        )
        roots = [self.repo / "docs", self.repo / "test"]
        excluded = {
            self.repo / "test/Makefile",
            Path(__file__).resolve(),
        }

        for root in roots:
            for path in root.rglob("*"):
                if not path.is_file() or path in excluded:
                    continue
                relative = path.relative_to(self.repo).as_posix()
                if (
                    relative.startswith("docs/superpowers/")
                    or relative.startswith("test/.results/")
                    or "/results/" in relative
                ):
                    continue
                try:
                    document = path.read_text()
                except UnicodeDecodeError:
                    continue
                for legacy in forbidden:
                    with self.subTest(path=relative, legacy=legacy):
                        self.assertNotIn(legacy, document)


if __name__ == "__main__":
    unittest.main()
