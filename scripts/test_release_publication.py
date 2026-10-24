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
EXPECTED_INLINE_REQUIREMENT = (
    '=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" '
    'and certificate 1[field.1.2.840.113635.100.6.2.6] exists '
    'and certificate leaf[field.1.2.840.113635.100.6.1.13] exists')


class FakeGH:
    """A deterministic in-process gh transport; all other commands are fake too."""
    def __init__(self, draft=None, assets=None):
        self.draft = draft
        self.assets = dict(assets or {})
        self.calls = []
        self.fail = None
        self.download_directories = []
        self.release_id = 314
        self.hide_draft = True  # Actual private-draft tag lookup may404.
        self.discovery = None
        self.discovery_pages = None

    def __call__(self, argv):
        self.calls.append(list(argv))
        self.assert_no_overwrite(argv)
        out, err, code = b"", b"", 0
        if argv[0] == "git":
            head = "b" * 40 if self.fail == "tag" else HEAD
            out = ("f" * 40 + "\trefs/tags/v" + VERSION + "\n" + head +
                   "\trefs/tags/v" + VERSION + "^{}\n").encode()
        elif argv[0] == "/usr/bin/codesign":
            expected = ["/usr/bin/codesign", "--verify"]
            if argv[-1].endswith(".app"):
                expected.append("--deep")
            expected += ["--strict", "-R", EXPECTED_INLINE_REQUIREMENT, argv[-1]]
            if argv != expected:
                raise AssertionError("codesign must receive exact inline requirement argv")
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
            endpoint = argv[-1]
            release_root = "repos/" + publication.REPO + "/releases"
            if endpoint.startswith(release_root + "?"):
                if self.fail == "discovery":
                    return subprocess.CompletedProcess(argv, 1, b"HTTP/2.0 403 Forbidden\r\n\r\n{}", b"")
                page = int(endpoint.rsplit("page=", 1)[1])
                data = (self.discovery_pages.get(page, []) if self.discovery_pages is not None else
                        self.discovery if self.discovery is not None else
                        [] if self.draft is None else [self.metadata()])
                return subprocess.CompletedProcess(argv, 0, b"HTTP/2.0 200 OK\r\n\r\n" + json.dumps(data).encode(), b"")
            tag_lookup = endpoint == release_root + "/tags/v" + VERSION
            id_lookup = endpoint == release_root + "/" + str(self.release_id)
            if not (tag_lookup or id_lookup):
                raise AssertionError("unexpected release metadata endpoint " + endpoint)
            if self.draft is None or (tag_lookup and self.hide_draft and self.draft):
                if self.fail == "stderr-404":
                    return subprocess.CompletedProcess(argv, 1, b"", b"gh: Not Found (HTTP 404)\n")
                return subprocess.CompletedProcess(argv, 1, b"HTTP/2.0 404 Not Found\r\n\r\n{}", b"")
            if id_lookup and self.fail == "id-missing":
                return subprocess.CompletedProcess(argv, 1, b"HTTP/2.0 404 Not Found\r\n\r\n{}", b"")
            data = self.metadata()
            if id_lookup and self.fail == "id-drift":
                data["id"] += 1
            if id_lookup and self.fail == "tag-drift":
                data["tag_name"] = "v0.0.0"
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

    def metadata(self):
        return {"id": self.release_id, "tag_name": "v" + VERSION, "draft": self.draft,
                "assets": [{"name": name, "size": len(value), "state": "uploaded"}
                           for name, value in self.assets.items()]}

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
        self.assertEqual([a for a in fake.calls if a[0] == "/usr/bin/codesign"], [
            ["/usr/bin/codesign", "--verify", "--deep", "--strict", "-R", EXPECTED_INLINE_REQUIREMENT,
             str(self.directory / "inert.app")],
            ["/usr/bin/codesign", "--verify", "--strict", "-R", EXPECTED_INLINE_REQUIREMENT,
             str(self.directory / NAMES[0])],
        ])
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

    def test_private_draft_404_resumes_same_exact_id_without_duplicate_creation(self):
        fake = FakeGH(True)
        result = self.publish(fake)
        self.assertEqual(result["release_id"], 314)
        self.assertEqual([a[2] for a in fake.mutations()], ["upload", "upload", "edit"])
        apis = [a[-1] for a in fake.calls if a[:2] == ["gh", "api"]]
        self.assertIn("repos/" + publication.REPO + "/releases?per_page=100&page=1", apis)
        self.assertGreaterEqual(apis.count("repos/" + publication.REPO + "/releases/314"), 4)
        self.assertFalse(fake.draft)

    def test_private_draft_identity_or_discovery_refusal_never_mutates(self):
        for defect in ("discovery", "id-missing", "id-drift", "tag-drift"):
            with self.subTest(defect=defect):
                fake = FakeGH(True)
                fake.fail = defect
                with self.assertRaises(publication.Refusal):
                    self.publish(fake)
                self.assertEqual(fake.mutations(), [])

    def test_duplicate_same_tag_private_records_refuse_before_creation_or_upload(self):
        fake = FakeGH(True)
        fake.discovery = [fake.metadata(), {**fake.metadata(), "id": 315}]
        with self.assertRaisesRegex(publication.Refusal, "duplicate exact-tag"):
            self.publish(fake)
        self.assertEqual(fake.mutations(), [])

    def test_malformed_or_incomplete_discovery_does_not_prove_absence(self):
        for records in ({"not": "a list"}, [{"id": True, "tag_name": "v" + VERSION, "draft": True}],
                        [{"id": 1, "tag_name": "unrelated", "draft": None}]):
            with self.subTest(records=records):
                fake = FakeGH()
                fake.discovery = records
                with self.assertRaises(publication.Refusal):
                    self.publish(fake)
                self.assertEqual(fake.mutations(), [])
        fake = FakeGH()
        fake.discovery = [{"id": i + 1, "tag_name": "unrelated", "draft": False} for i in range(100)]
        # Repeated page identities are ambiguous rather than absent.
        with self.assertRaisesRegex(publication.Refusal, "ambiguous"):
            self.publish(fake)
        self.assertEqual(fake.mutations(), [])

    def test_draft_exact_id_disappearing_after_readback_never_recreates(self):
        fake = FakeGH(True)
        original = fake.__call__
        count = 0

        def disappear(argv):
            nonlocal count
            if argv[:2] == ["gh", "api"] and argv[-1].endswith("/releases/314"):
                count += 1
                if count == 2:
                    fake.fail = "id-missing"
            return original(argv)

        with self.assertRaises(publication.Refusal):
            publication.publish(VERSION, HEAD, self.directory, self.directory / "inert.app", {}, run=disappear)
        self.assertEqual(fake.mutations(), [])

    def test_private_draft_on_second_page_and_complete_absence_bound(self):
        first = [{"id": i + 1000, "tag_name": "unrelated", "draft": False} for i in range(100)]
        fake = FakeGH(True, PAYLOADS)
        fake.discovery_pages = {1: first, 2: [fake.metadata()]}
        result = self.publish(fake)
        self.assertEqual(result["release_id"], 314)
        self.assertEqual([a[2] for a in fake.mutations()], ["edit"])
        self.assertTrue(any(a[-1].endswith("?per_page=100&page=2") for a in fake.calls))

        fake = FakeGH()
        fake.discovery_pages = {
            page: [{"id": page * 1000 + i, "tag_name": "unrelated", "draft": False} for i in range(100)]
            for page in range(1, publication.MAX_RELEASE_PAGES + 1)
        }
        with self.assertRaisesRegex(publication.Refusal, "incomplete at bound"):
            self.publish(fake)
        self.assertEqual(fake.mutations(), [])
        pages = [a for a in fake.calls if a[:2] == ["gh", "api"] and "?per_page=" in a[-1]]
        self.assertEqual(len(pages), publication.MAX_RELEASE_PAGES)

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

    def test_unobserved_public_release_stays_unknown_on_lookup_or_verifier_failure(self):
        for defect in ("lookup", "signature"):
            with self.subTest(defect=defect):
                fake = FakeGH(False, PAYLOADS)
                fake.fail = defect
                record = {}
                with self.assertRaises(publication.Refusal):
                    publication.publish(VERSION, HEAD, self.directory, self.directory / "inert.app", record, run=fake)
                self.assertIsNone(record["published"])
                self.assertFalse(record["publish_attempted"])
                self.assertEqual(fake.mutations(), [])

    def test_main_serializes_unobserved_failures_as_null_in_exclusive_receipt(self):
        original_publish = publication.publish
        for defect in ("lookup", "signature"):
            with self.subTest(defect=defect):
                fake = FakeGH(False, PAYLOADS)
                fake.fail = defect
                receipt = self.directory / (defect + ".json")

                def inert_publish(version, head, directory, app, record):
                    return original_publish(version, head, directory, app, record, run=fake)

                argv = ["publish-release-assets.py", "--version", VERSION, "--expected-commit", HEAD,
                        "--directory", str(self.directory), "--app", str(self.directory / "inert.app"),
                        "--receipt", str(receipt)]
                with mock.patch("sys.argv", argv), mock.patch.dict(publication.os.environ), \
                        mock.patch.object(publication, "publish", side_effect=inert_publish):
                    with self.assertRaises(publication.Refusal):
                        publication.main()
                retained = json.loads(receipt.read_text())
                self.assertIsNone(retained["published"])
                self.assertFalse(retained["publish_attempted"])
                self.assertEqual(retained["status"], "failed")
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
