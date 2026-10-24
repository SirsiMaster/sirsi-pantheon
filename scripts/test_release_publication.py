"""Inert publication state fixtures. No gh, Apple tool, network, or app runs."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location(
    "release_publication", Path(__file__).with_name("publish-release-assets.py"))
publication = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(publication)
VERSION = "0.24.999"
HEAD = "a" * 40
NAMES = ["SirsiPantheon-" + VERSION + "-arm64." + ext for ext in ("dmg", "pkg")]
PAYLOADS = dict(zip(NAMES, (b"owned inert dmg", b"owned inert pkg")))


class FakeGH:
    """A deterministic in-process gh transport; all other commands are fake too."""
    def __init__(self, draft=None, assets=None):
        self.draft = draft
        self.assets = dict(assets or {})
        self.calls = []
        self.fail = None
        self.download_directories = []

    def __call__(self, argv):
        self.calls.append(list(argv))
        self.assert_no_overwrite(argv)
        out, err, code = b"", b"", 0
        if argv[0] == "git":
            head = "b" * 40 if self.fail == "tag" else HEAD
            out = ("f" * 40 + "\trefs/tags/v" + VERSION + "\n" + head +
                   "\trefs/tags/v" + VERSION + "^{}\n").encode()
        elif argv[0] == "/usr/bin/codesign":
            if not all(flag in argv for flag in ("--verify", "--strict", "-R")):
                raise AssertionError("signature verification flags missing")
            requirement = argv[argv.index("-R") + 1]
            if not all(text in requirement for text in (
                    "anchor apple generic", 'certificate leaf[subject.OU] = "9D382WV988"',
                    "certificate 1[field.1.2.840.113635.100.6.2.6] exists",
                    "certificate leaf[field.1.2.840.113635.100.6.1.13] exists")):
                raise AssertionError("Apple Developer ID Team9D requirements weakened")
            if self.fail == "signature":
                code = 1
        elif argv[0] == "/usr/sbin/pkgutil":
            team = "FOREIGNTEAM" if self.fail == "installer" else publication.TEAM
            out = ("Status: signed by a developer certificate issued by Apple\n"
                   "Developer ID Installer: Owned Fixture (" + team + ")\n").encode()
        elif argv[0] == "/usr/bin/xcrun":
            if self.fail == "staple":
                code = 1
        elif argv[:2] == ["gh", "api"]:
            if self.fail == "lookup":
                return subprocess.CompletedProcess(argv, 1, b"HTTP/2.0 503 Unavailable\r\n\r\n{}", b"")
            if self.draft is None:
                if self.fail == "stderr-404":
                    return subprocess.CompletedProcess(argv, 1, b"", b"gh: Not Found (HTTP 404)\n")
                return subprocess.CompletedProcess(argv, 1, b"HTTP/2.0 404 Not Found\r\n\r\n{}", b"")
            data = {"tag_name": "v" + VERSION, "draft": self.draft,
                    "assets": [{"name": name, "size": len(value), "state": "uploaded"}
                               for name, value in self.assets.items()]}
            if self.fail == "duplicate":
                data["assets"] = [data["assets"][0], data["assets"][0]]
            out = b"HTTP/2.0 200 OK\r\n\r\n" + json.dumps(data).encode()
        elif argv[:3] == ["gh", "release", "create"]:
            if "--draft" not in argv or self.draft is not None:
                raise AssertionError("must create only an absent draft")
            self.draft = True
        elif argv[:3] == ["gh", "release", "upload"]:
            if self.draft is not True:
                raise AssertionError("never upload to a public release")
            path = Path(argv[-1])
            if path.name in self.assets:
                raise AssertionError("never overwrite an existing asset")
            if self.fail == "partial-upload" and path.name.endswith(".pkg"):
                code = 1
            else:
                self.assets[path.name] = path.read_bytes()
        elif argv[:3] == ["gh", "release", "download"]:
            name = argv[argv.index("--pattern") + 1]
            directory = Path(argv[argv.index("--dir") + 1])
            self.download_directories.append(directory)
            data = self.assets[name]
            if self.fail == "readback":
                data = b"x" * len(data)
            (directory / name).write_bytes(data)
        elif argv[:3] == ["gh", "release", "edit"]:
            if argv[-1] != "--draft=false" or self.draft is not True or self.assets != PAYLOADS:
                raise AssertionError("publish requires the exact owned pair")
            downloads = [a[a.index("--pattern") + 1] for a in self.calls if a[:3] == ["gh", "release", "download"]]
            if not all(n in downloads for n in NAMES):
                raise AssertionError("publish occurred before paired byte readback")
            self.draft = False
            if self.fail == "publish-unknown":
                code = 1  # Remote accepted, but transport could not confirm it.
        else:
            raise AssertionError("unexpected process/command: " + repr(argv))
        return subprocess.CompletedProcess(argv, code, out, err)

    @staticmethod
    def assert_no_overwrite(argv):
        if "--clobber" in argv or argv[:3] == ["gh", "release", "delete-asset"]:
            raise AssertionError("destructive publication route")

    def mutations(self):
        return [a for a in self.calls if a[:2] == ["gh", "release"] and a[2] in ("create", "upload", "edit")]


class ReleasePublicationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="owned-publication-fixture-")
        self.addCleanup(self.tmp.cleanup)
        self.directory = Path(self.tmp.name)
        for name, data in PAYLOADS.items():
            (self.directory / name).write_bytes(data)
        # Even an accidental omitted fake-run seam cannot execute production
        # commands or reach a host/network during these fixtures.
        guard = mock.patch.object(publication.subprocess, "run", side_effect=AssertionError("real process forbidden"))
        guard.start()
        self.addCleanup(guard.stop)

    def publish(self, fake):
        record = {}
        publication.publish(VERSION, HEAD, self.directory, self.directory / "inert.app", record, run=fake)
        self.assertTrue(all(not d.exists() for d in fake.download_directories))
        return record

    def test_new_release_is_draft_until_paired_verified_readback(self):
        fake = FakeGH()
        result = self.publish(fake)
        self.assertFalse(fake.draft)
        self.assertEqual(result["prepublication_readback"], result["assets"])
        self.assertEqual(len(result["verification"]), 5)
        self.assertEqual([a[2] for a in fake.mutations()], ["create", "upload", "upload", "edit"])

    def test_partial_upload_failure_keeps_draft_and_retry_resumes_without_overwrite(self):
        fake = FakeGH()
        fake.fail = "partial-upload"
        with self.assertRaises(publication.Refusal):
            self.publish(fake)
        self.assertTrue(fake.draft)
        self.assertEqual(fake.assets, {NAMES[0]: PAYLOADS[NAMES[0]]})
        self.assertFalse(any(a[2] == "edit" for a in fake.mutations()))
        fake.calls.clear()
        fake.fail = None
        self.publish(fake)
        uploads = [a for a in fake.mutations() if a[2] == "upload"]
        self.assertEqual([Path(a[-1]).name for a in uploads], [NAMES[1]])

    def test_existing_complete_draft_is_verified_then_published_without_upload(self):
        fake = FakeGH(True, PAYLOADS)
        self.publish(fake)
        self.assertEqual([a[2] for a in fake.mutations()], ["edit"])

    def test_explicit_gh_stderr_404_is_absent_not_unknown_transport(self):
        fake = FakeGH()
        fake.fail = "stderr-404"
        self.publish(fake)
        self.assertFalse(fake.draft)

    def test_published_identical_retry_is_read_only(self):
        fake = FakeGH(False, PAYLOADS)
        result = self.publish(fake)
        self.assertEqual(result["status"], "verified_existing_published")
        self.assertFalse(result["remote_mutation"])
        self.assertEqual(fake.mutations(), [])

    def test_published_same_size_changed_bytes_refuse_without_mutation(self):
        changed = {**PAYLOADS, NAMES[0]: b"x" * len(PAYLOADS[NAMES[0]])}
        fake = FakeGH(False, changed)
        with self.assertRaisesRegex(publication.Refusal, "bytes differ"):
            self.publish(fake)
        self.assertEqual(fake.mutations(), [])
        self.assertEqual(fake.assets, changed)

    def test_partial_published_record_is_ambiguous_and_never_repaired_in_place(self):
        for assets in ({}, {NAMES[0]: PAYLOADS[NAMES[0]]}):
            with self.subTest(assets=list(assets)):
                fake = FakeGH(False, assets)
                with self.assertRaisesRegex(publication.Refusal, "complete pair"):
                    self.publish(fake)
                self.assertEqual(fake.mutations(), [])

    def test_draft_wrong_bytes_also_refuse_without_overwrite(self):
        fake = FakeGH(True, {NAMES[0]: b"x" * len(PAYLOADS[NAMES[0]])})
        with self.assertRaisesRegex(publication.Refusal, "bytes differ"):
            self.publish(fake)
        self.assertEqual(fake.mutations(), [])

    def test_duplicate_metadata_refuses_without_mutation(self):
        fake = FakeGH(False, PAYLOADS)
        fake.fail = "duplicate"
        with self.assertRaisesRegex(publication.Refusal, "duplicate"):
            self.publish(fake)
        self.assertEqual(fake.mutations(), [])

    def test_publish_transport_failure_retains_unknown_not_false_unpublished(self):
        fake = FakeGH()
        fake.fail = "publish-unknown"
        record = {}
        with self.assertRaises(publication.Refusal):
            publication.publish(VERSION, HEAD, self.directory, self.directory / "inert.app", record, run=fake)
        self.assertFalse(fake.draft)  # Fake server accepted the mutation.
        self.assertTrue(record["publish_attempted"])
        self.assertIsNone(record["published"])
        self.assertTrue(all(not d.exists() for d in fake.download_directories))

    def test_failed_readback_cannot_publish_and_temp_is_cleaned(self):
        fake = FakeGH()
        fake.fail = "readback"
        with self.assertRaisesRegex(publication.Refusal, "bytes differ"):
            self.publish(fake)
        self.assertTrue(fake.draft)
        self.assertFalse(any(a[2] == "edit" for a in fake.mutations()))
        self.assertTrue(all(not d.exists() for d in fake.download_directories))

    def test_signature_installer_staple_and_unknown_lookup_fail_before_mutation(self):
        for defect in ("signature", "installer", "staple", "lookup", "tag"):
            with self.subTest(defect=defect):
                fake = FakeGH()
                fake.fail = defect
                with self.assertRaises(publication.Refusal):
                    self.publish(fake)
                self.assertEqual(fake.mutations(), [])

    def test_missing_or_symlink_local_pair_refuses_before_creation(self):
        pkg = self.directory / NAMES[1]
        pkg.unlink()
        fake = FakeGH()
        with self.assertRaises(OSError):
            self.publish(fake)
        self.assertEqual(fake.mutations(), [])
        pkg.symlink_to(self.directory / NAMES[0])
        with self.assertRaises(OSError):
            self.publish(fake)
        self.assertEqual(fake.mutations(), [])


if __name__ == "__main__":
    unittest.main()
