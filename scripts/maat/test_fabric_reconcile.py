import importlib.util
import pathlib
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("census", pathlib.Path(__file__).with_name("fabric-reconcile.py"))
census = importlib.util.module_from_spec(spec)
spec.loader.exec_module(census)


class ReadOnlyCensusTests(unittest.TestCase):
    def test_porcelain_spaces_and_rename(self):
        self.assertEqual(census.status_entries(b" M file with spaces\0R  new\0old\0?? fresh\0"),
                         [(" M", "file with spaces"), ("R ", "new"), ("??", "fresh")])

    def test_dirty_and_clean_without_index_or_head_changes(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            def git(*args):
                subprocess.run(["git", "-C", str(root), *args], check=True,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            git("init", "-q")
            (root / "tracked").write_text("baseline\n")
            git("add", "tracked")
            git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
            index = (root / ".git/index").read_bytes()
            row = census.inspect(root, {"own-lane"}, root.parent)
            self.assertEqual(row["state"], "CLEAN")
            (root / "tracked").write_text("unsaved work\n")
            (root / "untracked file").write_text("not a secret fixture\n")
            row = census.inspect(root, {"own-lane"}, root.parent)
            self.assertEqual(row["state"], "DIRTY")
            self.assertEqual(row["status_entry_count"], 2)
            self.assertEqual((root / ".git/index").read_bytes(), index)
            self.assertFalse(row["head_changed_during_observation"])
            # A leftover sequencer is unresolved work even if status is clean.
            git("add", "tracked", "untracked file")
            git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                "-c", "commit.gpgsign=false", "commit", "-qm", "checkpoint fixture work")
            (root / ".git/sequencer").mkdir()
            (root / ".git/sequencer/todo").write_text("pick fixture historical operation\n")
            row = census.inspect(root, {"own-lane"}, root.parent)
            self.assertEqual(row["status_entry_count"], 0)
            self.assertEqual(row["unfinished_git_operations"], ["sequencer"])

    def test_empty_inventory_does_not_prove_completion(self):
        with tempfile.TemporaryDirectory() as d:
            report = census.census(pathlib.Path(d), {"agents": {}}, [])
            self.assertFalse(report["zero_uncommitted_proven"])


if __name__ == "__main__":
    unittest.main()
