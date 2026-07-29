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
            "product-platform": "functional-platform",
            "product-topology": "functional-topology",
            "effectiveness-topology": "detection-topology",
        }

        for legacy, replacement in aliases.items():
            with self.subTest(legacy=legacy):
                self.assertRegex(
                    self.test_makefile,
                    rf"(?m)^{re.escape(legacy)}\s*:\s*{re.escape(replacement)}\s*$",
                )

    def test_root_makefile_exposes_public_taxonomy(self):
        targets = [
            "test-functional-endpoint",
            "test-functional-platform",
            "test-functional-topology",
            "test-detection",
            "test-performance",
            "test-distribution-package",
            "test-distribution-published",
            "test-release-candidate",
            "test-release-published",
        ]

        for target in targets:
            with self.subTest(target=target):
                self.assertRegex(self.root_makefile, rf"(?m)^{re.escape(target)}\s*:")

    def test_release_workflow_uses_distribution_paths(self):
        self.assertNotIn("test/suites/product/", self.release_workflow)
        self.assertNotIn("test/release/", self.release_workflow)
        self.assertIn("make -C test distribution-package", self.release_workflow)


if __name__ == "__main__":
    unittest.main()
