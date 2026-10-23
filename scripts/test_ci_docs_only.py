import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("ci_docs_only", Path(__file__).with_name("ci-docs-only.py"))
classifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(classifier)


class DocsOnlyTests(unittest.TestCase):
    def test_pure_docs(self):
        self.assertTrue(classifier.docs_only(b"README.md\0docs/design.txt\0.thoth/memory.yaml\0"))

    def test_mixed_code_is_not_a_test_exemption(self):
        self.assertFalse(classifier.docs_only(b"README.md\0internal/router/serve.go\0"))

    def test_deleted_code_and_workflow_still_require_real_checks(self):
        for path in (b"cmd/sirsi/main.go", b".github/workflows/ci.yml", b"go.mod", b"scripts/build.sh"):
            with self.subTest(path=path):
                self.assertFalse(classifier.docs_only(path+b"\0docs/plan.md\0"))

    def test_empty_or_malformed_list_is_not_an_exemption(self):
        self.assertFalse(classifier.docs_only(b""))
        self.assertFalse(classifier.docs_only(b"\0"))
        self.assertFalse(classifier.docs_only(b"docs/a.md\0\0"))

    def test_filename_newlines_do_not_hide_code(self):
        self.assertFalse(classifier.docs_only(b"README.md\0source\nREADME.go\0"))


if __name__ == "__main__":
    unittest.main()
