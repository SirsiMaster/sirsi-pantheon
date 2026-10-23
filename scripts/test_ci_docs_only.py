import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
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

    def test_real_git_rename_cannot_hide_deleted_code(self):
        # Isolate Git's environment too: this fixture must never inherit a
        # pre-push parent's GIT_DIR/GIT_WORK_TREE and modify that repository.
        env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)

            def git(*args):
                return subprocess.run(["git", *args], cwd=root, env=env,
                                      check=True, capture_output=True).stdout

            git("init", "-q")
            git("config", "user.email", "fixture@example.invalid")
            git("config", "user.name", "Docs classifier fixture")
            source = root / "internal/router/x.go"
            source.parent.mkdir(parents=True)
            source.write_text("package router\n")
            git("add", ".")
            git("commit", "-qm", "source")
            destination = root / "docs/x.go"
            destination.parent.mkdir()
            source.rename(destination)
            git("add", "-A")
            git("commit", "-qm", "move source into docs")
            # Demonstrate the old path-only classification really goes green.
            self.assertTrue(classifier.docs_only(git("diff", "-M", "--name-only", "-z", "HEAD~1", "HEAD")))
            for refs in (("HEAD~1", "HEAD"), ("HEAD~1...HEAD",)):
                changed = git("diff", "--no-renames", "--name-only", "-z", *refs)
                self.assertIn(b"internal/router/x.go\0", changed)
                self.assertFalse(classifier.docs_only(changed))


if __name__ == "__main__":
    unittest.main()
