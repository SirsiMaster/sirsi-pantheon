"""Inert contract fixtures for the future three-asset release path.

These tests may fail while guard publication is being implemented. They do not
invoke gh, signing, mounting, or any other host workload.
"""
import importlib.util
import hashlib
import json
import os
import plistlib
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest
from unittest import mock
import zipfile


SPEC = importlib.util.spec_from_file_location(
    "release_publication_fixture", Path(__file__).with_name("test_release_publication.py"))
fixture = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(fixture)
publication = fixture.publication
CUSTODY_SPEC = importlib.util.spec_from_file_location(
    "release_custody_fixture", Path(__file__).with_name("release-custody.py"))
custody = importlib.util.module_from_spec(CUSTODY_SPEC)
CUSTODY_SPEC.loader.exec_module(custody)
RUN_CUSTODY_SPEC = importlib.util.spec_from_file_location(
    "run_custody_fixture", Path(__file__).with_name("check-release-run-custody.py"))
run_custody = importlib.util.module_from_spec(RUN_CUSTODY_SPEC)
RUN_CUSTODY_SPEC.loader.exec_module(run_custody)
RESERVE_SPEC = importlib.util.spec_from_file_location(
    "reserve_release_fixture", Path(__file__).with_name("reserve-release-draft.py"))
reserve_release = importlib.util.module_from_spec(RESERVE_SPEC)
RESERVE_SPEC.loader.exec_module(reserve_release)
GUARD = "SirsiPantheon-" + fixture.VERSION + "-arm64-pretoolguard.dmg"
PAYLOADS = {**fixture.PAYLOADS, GUARD: b"inert guard image"}
MANIFEST = {
    "sirsi-display-power-guard": {"sha256": "a" * 64, "identifier": "test.guard"},
    "sirsi-pretoolguard-install": {"sha256": "b" * 64, "identifier": "test.install"},
}


class GuardFakeGH(fixture.FakeGH):
    """In-process reserved-release transport; no tag-addressed mutations."""

    def __init__(self, draft=True, assets=None, *, corrupt_guard=False,
                 fail_guard_upload=False, fail_download=False, fail_patch=False):
        super().__init__(draft, assets)
        self.corrupt_guard = corrupt_guard
        self.fail_guard_upload = fail_guard_upload
        self.fail_download = fail_download
        self.fail_patch = fail_patch
        self.asset_ids = {name: i + 501 for i, name in enumerate(self.assets)}
        self.downloads = []

    def metadata(self):
        value = super().metadata()
        for asset in value["assets"]:
            asset["id"] = self.asset_ids[asset["name"]]
            if self.fail == "asset-id-missing":
                del asset["id"]
        return value

    def download(self, asset_id, path, expected_bytes):
        self.downloads.append((asset_id, Path(path), expected_bytes))
        names = [name for name, known_id in self.asset_ids.items() if known_id == asset_id]
        if len(names) != 1 or names[0] not in self.assets:
            raise AssertionError("download must select an existing asset ID")
        if self.fail_download:
            raise publication.Refusal("inert download failure")
        data = self.assets[names[0]]
        if expected_bytes != len(data):
            raise AssertionError("readback must use verified expected asset length")
        if self.corrupt_guard and names[0] == GUARD:
            data = b"x" * len(data)
        Path(path).write_bytes(data)

    def __call__(self, argv):
        if argv[:3] == ["/usr/bin/codesign", "-d", "--verbose=4"]:
            self.calls.append(list(argv))
            return subprocess.CompletedProcess(argv, 0, b"Timestamp=fixture\n", b"")
        if argv[:2] == ["gh", "release"]:
            raise AssertionError("tag-addressed gh release operation forbidden")
        if argv[:2] == ["gh", "api"] and "--method" in argv:
            self.calls.append(list(argv))
            self.assert_no_overwrite(argv)
            method = argv[argv.index("--method") + 1]
            if method == "POST":
                path = Path(argv[argv.index("--input") + 1])
                expected = (f"https://uploads.github.com/repos/{publication.REPO}/releases/"
                            f"{self.release_id}/assets?name={path.name}")
                if argv[-1] != expected or self.draft is not True or path.name in self.assets:
                    raise AssertionError("upload must target reserved draft ID without overwrite")
                if path.name == GUARD and self.fail_guard_upload:
                    return subprocess.CompletedProcess(argv, 1, b"", b"inert upload failure")
                self.assets[path.name] = path.read_bytes()
                self.asset_ids[path.name] = max(self.asset_ids.values(), default=500) + 1
            elif method == "PATCH":
                expected = f"repos/{publication.REPO}/releases/{self.release_id}"
                read_ids = {asset_id for asset_id, _, _ in self.downloads}
                if (argv[-1] != expected or "draft=false" not in argv or self.draft is not True or
                        self.assets != PAYLOADS or read_ids != set(self.asset_ids.values())):
                    raise AssertionError("publish requires ID-pinned three-asset readback")
                if self.fail_patch:
                    return subprocess.CompletedProcess(argv, 1, b"", b"inert patch failure")
                self.draft = False
            else:
                raise AssertionError("unexpected mutation method")
            return subprocess.CompletedProcess(argv, 0, b"", b"")
        return super().__call__(argv)

    def mutations(self):
        return [a for a in self.calls if a[:2] == ["gh", "api"] and "--method" in a]

    def mutation_kinds(self):
        return ["upload" if a[a.index("--method") + 1] == "POST" else "edit"
                for a in self.mutations()]


class GuardPublicationTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="guard-publication-fixture-")
        self.addCleanup(tmp.cleanup)
        self.directory = Path(tmp.name)
        for name, data in PAYLOADS.items():
            (self.directory / name).write_bytes(data)
        process_guard = mock.patch.object(
            publication.subprocess, "run", side_effect=AssertionError("real process forbidden"))
        process_guard.start()
        self.addCleanup(process_guard.stop)
        verifier = mock.patch.object(publication, "verify_guard_image", create=True)
        self.verify_guard = verifier.start()
        self.addCleanup(verifier.stop)

    def publish(self, fake, manifest=MANIFEST):
        record = {}
        publication.publish(fixture.VERSION, fixture.HEAD, self.directory,
                            self.directory / "inert.app", record, run=fake,
                            include_guard=True, guard_manifest=manifest,
                            reserved_id=fake.release_id, download=fake.download)
        return record

    def assert_no_publish(self, fake):
        self.assertFalse(any("--method" in a and a[a.index("--method") + 1] == "PATCH"
                             for a in fake.mutations()))

    def test_three_assets_are_staged_and_read_back_before_publish(self):
        fake = GuardFakeGH()
        record = self.publish(fake)
        self.verify_guard.assert_called_once_with(self.directory / GUARD, MANIFEST, fake)
        self.assertEqual(fake.assets, PAYLOADS)
        self.assertEqual(set(record["prepublication_readback"]), set(PAYLOADS))
        self.assertEqual(fake.mutation_kinds(), ["upload", "upload", "upload", "edit"])
        self.assertEqual({item[0] for item in fake.downloads}, set(fake.asset_ids.values()))
        for asset_id, path, expected_bytes in fake.downloads:
            self.assertEqual(asset_id, fake.asset_ids[path.name])
            self.assertEqual(expected_bytes, len(PAYLOADS[path.name]))
        self.assertTrue(all(not a[:2] == ["gh", "release"] for a in fake.calls))
        self.assertFalse(fake.draft)

    def test_missing_local_guard_or_manifest_never_publishes(self):
        for missing in ("file", "manifest"):
            with self.subTest(missing=missing):
                fake = GuardFakeGH()
                if missing == "file":
                    (self.directory / GUARD).unlink()
                with self.assertRaises((publication.Refusal, FileNotFoundError)):
                    self.publish(fake, None if missing == "manifest" else MANIFEST)
                self.assert_no_publish(fake)
                if missing == "file":
                    (self.directory / GUARD).write_bytes(PAYLOADS[GUARD])

    def test_missing_guard_in_published_release_or_extra_asset_is_refused(self):
        for assets in (fixture.PAYLOADS, {**PAYLOADS, "unexpected.dmg": b"extra"}):
            with self.subTest(assets=list(assets)):
                fake = GuardFakeGH(False, assets)
                with self.assertRaises(publication.Refusal):
                    self.publish(fake)
                self.assertEqual(fake.mutations(), [])
                self.assert_no_publish(fake)

    def test_untrusted_guard_and_failed_staging_or_readback_never_publish(self):
        for failure in ("untrusted", "upload", "readback"):
            with self.subTest(failure=failure):
                fake = GuardFakeGH(fail_guard_upload=failure == "upload",
                                   corrupt_guard=failure == "readback")
                self.verify_guard.reset_mock(side_effect=True)
                if failure == "untrusted":
                    self.verify_guard.side_effect = publication.Refusal("untrusted guard payload")
                with self.assertRaises(publication.Refusal):
                    self.publish(fake)
                self.assert_no_publish(fake)
                if failure == "untrusted":
                    self.assertEqual(fake.mutations(), [])

    def test_reserved_id_pins_all_release_reads_without_discovery_or_creation(self):
        fake = GuardFakeGH(True)
        record = {}
        publication.publish(fixture.VERSION, fixture.HEAD, self.directory,
                            self.directory / "inert.app", record, run=fake,
                            include_guard=True, guard_manifest=MANIFEST, reserved_id=fake.release_id,
                            download=fake.download)
        self.assertEqual(record["release_id"], fake.release_id)
        self.assertEqual(fake.mutation_kinds(), ["upload", "upload", "upload", "edit"])
        endpoints = [a[-1] for a in fake.calls if a[:2] == ["gh", "api"] and "--method" not in a]
        self.assertTrue(endpoints)
        self.assertTrue(all(endpoint == "repos/" + publication.REPO + "/releases/314" for endpoint in endpoints))

    def test_reserved_id_missing_or_drifted_never_rediscovers_or_mutates(self):
        for failure in ("id-missing", "id-drift", "tag-drift"):
            with self.subTest(failure=failure):
                fake = GuardFakeGH(True)
                fake.fail = failure
                record = {}
                with self.assertRaises(publication.Refusal):
                    publication.publish(fixture.VERSION, fixture.HEAD, self.directory,
                                        self.directory / "inert.app", record, run=fake,
                                        include_guard=True, guard_manifest=MANIFEST,
                                        reserved_id=fake.release_id, download=fake.download)
                self.assertEqual(fake.mutations(), [])
                endpoints = [a[-1] for a in fake.calls if a[:2] == ["gh", "api"]]
                self.assertEqual(endpoints, ["repos/" + publication.REPO + "/releases/314"])
                self.assertFalse(record["publish_attempted"])

    def test_missing_reservation_fails_before_any_transport(self):
        for reservation in (None, 0, True, "314"):
            with self.subTest(reservation=reservation):
                fake = GuardFakeGH()
                record = {}
                with self.assertRaises(publication.Refusal):
                    publication.publish(fixture.VERSION, fixture.HEAD, self.directory,
                                        self.directory / "inert.app", record, run=fake,
                                        include_guard=True, guard_manifest=MANIFEST,
                                        reserved_id=reservation, download=fake.download)
                self.assertEqual(fake.calls, [])
                self.assertEqual(fake.downloads, [])
                self.assertFalse(record["publish_attempted"])

    def test_download_failure_or_missing_asset_id_prevents_patch(self):
        for failure in ("download", "asset-id-missing"):
            with self.subTest(failure=failure):
                fake = GuardFakeGH(True, PAYLOADS, fail_download=failure == "download")
                if failure == "asset-id-missing":
                    fake.fail = failure
                with self.assertRaises(publication.Refusal):
                    self.publish(fake)
                self.assert_no_publish(fake)
                self.assertEqual(fake.mutations(), [])

    def test_patch_transport_failure_keeps_publication_outcome_unknown(self):
        fake = GuardFakeGH(fail_patch=True)
        record = {}
        with self.assertRaises(publication.Refusal):
            publication.publish(fixture.VERSION, fixture.HEAD, self.directory,
                                self.directory / "inert.app", record, run=fake,
                                include_guard=True, guard_manifest=MANIFEST,
                                reserved_id=fake.release_id, download=fake.download)
        self.assertTrue(record["publish_attempted"])
        self.assertIsNone(record["published"])
        self.assertEqual(fake.mutation_kinds(), ["upload", "upload", "upload", "edit"])


class AssetDownloadFixtureTests(unittest.TestCase):
    """Exercise streaming with a synthetic process and selector, never gh."""

    def test_invalid_expected_size_refuses_before_starting_process(self):
        for size in (0, -1, True, "3", publication.MAX_ASSET_BYTES + 1):
            with self.subTest(size=size), tempfile.TemporaryDirectory() as temporary:
                path = Path(temporary) / "asset"
                with mock.patch.object(publication.subprocess, "Popen", side_effect=AssertionError("real process forbidden")) as popen:
                    with self.assertRaises(publication.Refusal):
                        publication.download_asset(501, path, size)
                    popen.assert_not_called()
                self.assertFalse(path.exists())

    def test_stream_exact_short_oversized_and_timeout_are_bounded(self):
        cases = (("exact", [b"ab", b"c", b""], True),
                 ("short", [b"ab", b""], False),
                 ("oversized", [b"abcd"], False),
                 ("timeout", [], False))
        for case, blocks, success in cases:
            with self.subTest(case=case), tempfile.TemporaryDirectory() as temporary:
                path = Path(temporary) / "asset"
                process = mock.MagicMock()
                process.stdout.fileno.return_value = 4321
                process.wait.return_value = 0
                process.poll.return_value = None
                selector = mock.MagicMock()
                selector.__enter__.return_value = selector
                selector.select.return_value = [] if case == "timeout" else [(object(), 1)]
                with mock.patch.object(publication.subprocess, "Popen", return_value=process) as popen, \
                        mock.patch.object(publication.selectors, "DefaultSelector", return_value=selector), \
                        mock.patch.object(publication.os, "read", side_effect=blocks) as read, \
                        mock.patch.object(publication.os, "fsync"):
                    if success:
                        publication.download_asset(501, path, 3)
                    else:
                        with self.assertRaises(publication.Refusal):
                            publication.download_asset(501, path, 3)
                    self.assertEqual(popen.call_args.args[0][-1],
                                     f"repos/{publication.REPO}/releases/assets/501")
                    self.assertTrue(all(call.args[1] <= 4 for call in read.call_args_list))
                    if success:
                        self.assertEqual([call.args[1] for call in read.call_args_list], [4, 2, 1])
                        self.assertEqual(path.read_bytes(), b"abc")
                        process.kill.assert_not_called()
                    else:
                        process.kill.assert_called_once()
                    process.stdout.close.assert_called_once()


class GuardImageVerificationTests(unittest.TestCase):
    def test_exact_payloads_and_detach_are_required(self):
        payloads = {"sirsi-display-power-guard": b"guard", "sirsi-pretoolguard-install": b"installer"}
        expected = {name: {"sha256": hashlib.sha256(data).hexdigest(), "identifier": name}
                    for name, data in payloads.items()}
        with tempfile.TemporaryDirectory() as temporary:
            mount = Path(temporary) / "mount"
            mount.mkdir()
            calls = []
            def run(argv):
                calls.append(argv)
                if argv[0].endswith("hdiutil") and argv[1] == "attach":
                    for name, data in payloads.items():
                        (mount / name).write_bytes(data)
                        (mount / name).chmod(0o755)
                elif argv[0].endswith("hdiutil") and argv[1] == "detach":
                    for name in payloads:
                        (mount / name).unlink()
                details = (plistlib.dumps({"system-entities": [{"dev-entry": "/dev/disk123"},
                           {"dev-entry": "/dev/disk123s1", "mount-point": str(mount)}]})
                           if argv[:2] == ["/usr/bin/hdiutil", "attach"] else
                           b"Timestamp=fixture\nCodeDirectory v=1 flags=0x10000(runtime)\n"
                           if argv[:2] == ["/usr/bin/codesign", "-d"] else b"")
                return subprocess.CompletedProcess(argv, 0, details, b"")
            with mock.patch.object(publication.tempfile, "mkdtemp", return_value=str(mount)):
                publication.verify_guard_image(Path("inert.dmg"), expected, run)
            self.assertFalse(mount.exists())
            self.assertEqual(calls[-1][:2], ["/usr/bin/hdiutil", "detach"])

    def test_invalid_manifest_refuses_before_attach(self):
        with self.assertRaises(publication.Refusal):
            publication.verify_guard_image(Path("inert.dmg"), {},
                                           lambda argv: self.fail("unexpected process"))

    def test_unpartitioned_hfs_volume_uses_its_exact_device_for_detach(self):
        payloads = {"sirsi-display-power-guard": b"guard", "sirsi-pretoolguard-install": b"installer"}
        expected = {name: {"sha256": hashlib.sha256(data).hexdigest(), "identifier": name}
                    for name, data in payloads.items()}
        with tempfile.TemporaryDirectory() as temporary:
            mount = Path(temporary) / "mount"
            mount.mkdir()
            calls = []
            def run(argv):
                calls.append(list(argv))
                if argv[:2] == ["/usr/bin/hdiutil", "attach"]:
                    for name, data in payloads.items():
                        (mount / name).write_bytes(data)
                        (mount / name).chmod(0o755)
                    return subprocess.CompletedProcess(argv, 0, plistlib.dumps({"system-entities": [
                        {"dev-entry": "/dev/disk123", "mount-point": str(mount)}]}), b"")
                if argv[:2] == ["/usr/bin/hdiutil", "detach"]:
                    for name in payloads:
                        (mount / name).unlink()
                details = (b"Timestamp=fixture\nCodeDirectory v=1 flags=0x10000(runtime)\n"
                           if argv[:2] == ["/usr/bin/codesign", "-d"] else b"")
                return subprocess.CompletedProcess(argv, 0, details, b"")
            with mock.patch.object(publication.tempfile, "mkdtemp", return_value=str(mount)):
                publication.verify_guard_image(Path("inert.dmg"), expected, run)
            self.assertEqual(calls[-1], ["/usr/bin/hdiutil", "detach", "/dev/disk123"])
            self.assertFalse(mount.exists())

    def test_synthesized_multi_root_is_retained_without_guessing_detach(self):
        with tempfile.TemporaryDirectory() as temporary:
            mount = Path(temporary) / "mount"
            mount.mkdir()
            calls = []
            def run(argv):
                calls.append(list(argv))
                self.assertEqual(argv[:2], ["/usr/bin/hdiutil", "attach"])
                topology = {"system-entities": [{"dev-entry": "/dev/disk123"},
                    {"dev-entry": "/dev/disk124s1", "mount-point": str(mount)}]}
                return subprocess.CompletedProcess(argv, 0, plistlib.dumps(topology), b"")
            with mock.patch.object(publication.tempfile, "mkdtemp", return_value=str(mount)):
                with self.assertRaisesRegex(publication.Refusal, "topology unknown"):
                    publication.verify_guard_image(Path("inert.dmg"), MANIFEST, run)
            self.assertEqual(len(calls), 1)
            self.assertTrue(mount.is_dir())

    def test_mount_without_matching_device_never_detaches(self):
        for kind in ('missing', 'mismatched'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as temporary:
                mount = Path(temporary) / 'mount'
                mount.mkdir()
                calls = []
                def run(argv):
                    calls.append(list(argv))
                    self.assertEqual(argv[:2], ['/usr/bin/hdiutil', 'attach'])
                    entity = {'mount-point': str(mount)}
                    if kind == 'mismatched':
                        entity['dev-entry'] = '/dev/disk124s1'
                    topology = {'system-entities': [{'dev-entry': '/dev/disk123'}, entity]}
                    return subprocess.CompletedProcess(argv, 0, plistlib.dumps(topology), b'')
                with mock.patch.object(publication.tempfile, 'mkdtemp', return_value=str(mount)):
                    with self.assertRaises(publication.Refusal):
                        publication.verify_guard_image(Path('inert.dmg'), MANIFEST, run)
                self.assertEqual(len(calls), 1, 'ambiguous association must not detach')
                self.assertTrue(mount.is_dir())

    def test_extra_mounted_volume_is_refused_and_known_device_detached(self):
        with tempfile.TemporaryDirectory() as temporary:
            mount = Path(temporary) / "mount"
            mount.mkdir()
            calls = []

            def run(argv):
                calls.append(list(argv))
                if argv[:2] == ["/usr/bin/hdiutil", "attach"]:
                    topology = {"system-entities": [
                        {"dev-entry": "/dev/disk123"},
                        {"dev-entry": "/dev/disk123s1", "mount-point": str(mount)},
                        {"dev-entry": "/dev/disk123s2", "mount-point": "/inert/extra"},
                    ]}
                    return subprocess.CompletedProcess(argv, 0, plistlib.dumps(topology), b"")
                self.assertEqual(argv, ["/usr/bin/hdiutil", "detach", "/dev/disk123"])
                return subprocess.CompletedProcess(argv, 0, b"", b"")

            with mock.patch.object(publication.tempfile, "mkdtemp", return_value=str(mount)), \
                    mock.patch.object(publication.subprocess, "run", side_effect=AssertionError("real process forbidden")):
                with self.assertRaisesRegex(publication.Refusal, "unexpected mounted volumes"):
                    publication.verify_guard_image(Path("inert.dmg"), MANIFEST, run)
            self.assertEqual(len(calls), 2)
            self.assertFalse(mount.exists())

    def test_extra_unmounted_device_is_refused_and_known_device_detached(self):
        with tempfile.TemporaryDirectory() as temporary:
            mount = Path(temporary) / "mount"
            mount.mkdir()
            calls = []
            def run(argv):
                calls.append(list(argv))
                if argv[:2] == ["/usr/bin/hdiutil", "attach"]:
                    topology = {"system-entities": [{"dev-entry": "/dev/disk123"},
                        {"dev-entry": "/dev/disk123s1", "mount-point": str(mount)},
                        {"dev-entry": "/dev/disk123s2"}]}
                    return subprocess.CompletedProcess(argv, 0, plistlib.dumps(topology), b"")
                self.assertEqual(argv, ["/usr/bin/hdiutil", "detach", "/dev/disk123"])
                return subprocess.CompletedProcess(argv, 0, b"", b"")
            with mock.patch.object(publication.tempfile, "mkdtemp", return_value=str(mount)):
                with self.assertRaisesRegex(publication.Refusal, "unexpected unmounted"):
                    publication.verify_guard_image(Path("inert.dmg"), MANIFEST, run)
            self.assertEqual(len(calls), 2)

    def test_unknown_attach_never_detaches_or_removes_unidentified_mount(self):
        for failure in ("timeout", "invalid_metadata", "nonzero"):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                mount = Path(temporary) / "mount"
                mount.mkdir()
                calls = []

                def run(argv):
                    calls.append(list(argv))
                    self.assertEqual(argv[:2], ["/usr/bin/hdiutil", "attach"])
                    if failure == "timeout":
                        raise subprocess.TimeoutExpired(argv, 1)
                    return subprocess.CompletedProcess(argv, 1 if failure == "nonzero" else 0,
                                                       b"invalid plist", b"")

                with mock.patch.object(publication.tempfile, "mkdtemp", return_value=str(mount)), \
                        mock.patch.object(publication.subprocess, "run", side_effect=AssertionError("real process forbidden")):
                    with self.assertRaises(publication.Refusal):
                        publication.verify_guard_image(Path("inert.dmg"), MANIFEST, run)
                self.assertEqual(len(calls), 1)
                self.assertTrue(mount.is_dir())

    def test_detach_failure_refuses_and_retains_mount_for_inspection(self):
        payloads = {"sirsi-display-power-guard": b"guard", "sirsi-pretoolguard-install": b"installer"}
        expected = {name: {"sha256": hashlib.sha256(data).hexdigest(), "identifier": name}
                    for name, data in payloads.items()}
        for failure in ("nonzero", "timeout"):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                mount = Path(temporary) / "mount"
                mount.mkdir()
                calls = []

                def run(argv):
                    calls.append(list(argv))
                    if argv[:2] == ["/usr/bin/hdiutil", "attach"]:
                        for name, data in payloads.items():
                            (mount / name).write_bytes(data)
                            (mount / name).chmod(0o755)
                        topology = {"system-entities": [{"dev-entry": "/dev/disk123"},
                            {"dev-entry": "/dev/disk123s1", "mount-point": str(mount)}]}
                        return subprocess.CompletedProcess(argv, 0, plistlib.dumps(topology), b"")
                    if argv[:2] == ["/usr/bin/hdiutil", "detach"]:
                        self.assertEqual(argv[-1], "/dev/disk123")
                        if failure == "timeout":
                            raise subprocess.TimeoutExpired(argv, 1)
                        return subprocess.CompletedProcess(argv, 1, b"", b"inert failure")
                    self.assertIn(argv[0], ("/usr/bin/codesign", "/usr/bin/python3"))
                    details = b"Timestamp=fixture\nCodeDirectory v=1 flags=0x10000(runtime)\n"
                    return subprocess.CompletedProcess(argv, 0, details, b"")

                with mock.patch.object(publication.tempfile, "mkdtemp", return_value=str(mount)), \
                        mock.patch.object(publication.subprocess, "run", side_effect=AssertionError("real process forbidden")):
                    with self.assertRaisesRegex(publication.Refusal, "detach"):
                        publication.verify_guard_image(Path("inert.dmg"), expected, run)
                self.assertEqual(calls[-1][:2], ["/usr/bin/hdiutil", "detach"])
                self.assertTrue(mount.is_dir())


class DraftReservationFixtureTests(unittest.TestCase):
    def invoke(self, *, pages=None, post_pages=None, heads=None, response=None, create_failure=False,
               persist=lambda state: None):
        calls, record = [], {}
        pages = pages or {}
        heads = list(heads or [fixture.HEAD, fixture.HEAD, fixture.HEAD])
        tag = "v" + fixture.VERSION
        identity = {"id": 314, "tag_name": tag, "draft": True}
        if post_pages is None:
            post_pages = {1: [identity]}
        created = False
        post_scan = -1

        def run(argv):
            nonlocal created, post_scan
            calls.append(list(argv))
            if argv[0] == "git":
                head = heads.pop(0)
                raw = ("f" * 40 + "\trefs/tags/" + tag + "\n" + head +
                       "\trefs/tags/" + tag + "^{}\n").encode()
                return subprocess.CompletedProcess(argv, 0, raw, b"")
            if argv[:2] == ["gh", "api"] and "--method" not in argv:
                page = int(argv[-1].rsplit("page=", 1)[1])
                if created and isinstance(post_pages, list):
                    if page == 1:
                        post_scan += 1
                    source = post_pages[min(post_scan, len(post_pages) - 1)]
                else:
                    source = post_pages if created else pages
                return subprocess.CompletedProcess(argv, 0, json.dumps(source.get(page, [])).encode(), b"")
            if argv[:2] == ["gh", "api"] and "--method" in argv:
                created = True
                raw = response if response is not None else b"HTTP/2 201 Created\r\n\r\n" + json.dumps(identity).encode()
                return subprocess.CompletedProcess(argv, 1 if create_failure else 0, raw, b"")
            raise AssertionError("unexpected command " + repr(argv))

        with mock.patch.object(reserve_release.subprocess, "run", side_effect=AssertionError("real process forbidden")):
            try:
                reserve_release.reserve(fixture.VERSION, fixture.HEAD, record, run=run, persist=persist)
                error = None
            except (RuntimeError, ValueError) as exc:
                error = exc
        return calls, record, error

    @staticmethod
    def creates(calls):
        return [call for call in calls if call[:2] == ["gh", "api"] and "--method" in call]

    def test_complete_pagination_rechecks_peeled_tag_and_records_exact_draft_identity(self):
        page = [{"id": i, "tag_name": "v0.0." + str(i), "draft": False} for i in range(1, 101)]
        identity = {"id": 314, "tag_name": "v" + fixture.VERSION, "draft": True}
        calls, record, error = self.invoke(pages={1: page, 2: []}, post_pages={1: page, 2: [identity]})
        self.assertIsNone(error)
        self.assertEqual(record, {"tag": "v" + fixture.VERSION, "commit": fixture.HEAD,
                                  "status": "reserved", "creation_attempted": True, "release_id": 314})
        self.assertEqual([call[0] for call in calls], ["git", "gh", "gh", "git", "gh", "gh", "gh", "git"])
        self.assertIn("page=2", calls[2][-1])
        self.assertIn("page=2", calls[6][-1])
        self.assertEqual(len(self.creates(calls)), 1)
        creation = self.creates(calls)[0]
        self.assertIn("tag_name=v" + fixture.VERSION, creation)
        self.assertIn("target_commitish=" + fixture.HEAD, creation)
        self.assertIn("draft=true", creation)

    def test_post_create_duplicate_or_changed_identity_refuses_without_reselection(self):
        tag = "v" + fixture.VERSION
        selected = {"id": 314, "tag_name": tag, "draft": True}
        for entries in ([selected, {"id": 315, "tag_name": tag, "draft": True}],
                        [{"id": 315, "tag_name": tag, "draft": True}],
                        [{"id": 314, "tag_name": tag, "draft": False}], []):
            with self.subTest(entries=entries):
                calls, record, error = self.invoke(post_pages={1: entries})
                self.assertIsNotNone(error)
                self.assertEqual(record["release_id"], 314)
                self.assertNotEqual(record["status"], "reserved")
                self.assertEqual(len(self.creates(calls)), 1)

    def test_post_create_page_shift_restarts_read_only_discovery_once(self):
        tag = "v" + fixture.VERSION
        selected = {"id": 314, "tag_name": tag, "draft": True}
        page = [selected] + [{"id": i, "tag_name": "v0.0." + str(i), "draft": False}
                             for i in range(1, 100)]
        shifted = {1: page, 2: [page[-1]]}
        stable = {1: page, 2: [{"id": 100, "tag_name": "v0.0.100", "draft": False}]}
        calls, record, error = self.invoke(post_pages=[shifted, stable])
        self.assertIsNone(error)
        self.assertEqual(record["release_id"], 314)
        self.assertEqual(record["status"], "reserved")
        self.assertEqual(len(self.creates(calls)), 1)

    def test_intent_and_returned_id_are_persisted_before_next_operation(self):
        states = []
        calls, record, error = self.invoke(persist=lambda state: states.append(dict(state)))
        self.assertIsNone(error)
        self.assertEqual(states[0]["status"], "create_outcome_unknown")
        self.assertTrue(states[0]["creation_attempted"])
        self.assertNotIn("release_id", states[0])
        self.assertEqual(states[1]["release_id"], 314)
        self.assertEqual(len(self.creates(calls)), 1)

    def test_persist_interruption_stops_before_following_transport(self):
        for interrupt_after in (1, 2):
            with self.subTest(interrupt_after=interrupt_after):
                states = []
                def persist(state):
                    states.append(dict(state))
                    if len(states) == interrupt_after:
                        raise RuntimeError("simulated persistence interruption")
                calls, record, error = self.invoke(persist=persist)
                self.assertIsNotNone(error)
                self.assertEqual(len(self.creates(calls)), interrupt_after - 1)
                if interrupt_after == 2:
                    self.assertEqual(states[-1]["release_id"], 314)
                    self.assertEqual(calls[-1], self.creates(calls)[0])

    def test_atomic_receipt_retains_last_persisted_id(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "receipt.json"
            path.touch(exist_ok=False)
            intent = {"status": "create_outcome_unknown", "creation_attempted": True}
            reserve_release.persist_receipt(str(path), intent)
            self.assertEqual(json.loads(path.read_text()), intent)
            identified = {**intent, "release_id": 314}
            reserve_release.persist_receipt(str(path), identified)
            self.assertEqual(json.loads(path.read_text()), identified)
            self.assertEqual(sorted(p.name for p in Path(directory).iterdir()), ["receipt.json"])

    def test_receipt_failure_before_replace_preserves_previous_intent(self):
        for method in ('replace', 'fsync'):
            with self.subTest(method=method), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / 'receipt.json'
                path.touch(exist_ok=False)
                intent = {'status': 'create_outcome_unknown', 'creation_attempted': True}
                reserve_release.persist_receipt(str(path), intent)
                with mock.patch.object(reserve_release.os, method, side_effect=OSError('inert failure')):
                    with self.assertRaises(OSError):
                        reserve_release.persist_receipt(str(path), {**intent, 'release_id': 314})
                self.assertEqual(json.loads(path.read_text()), intent)
                self.assertEqual(sorted(p.name for p in Path(directory).iterdir()), ['receipt.json'])

    def test_post_create_repeated_page_shift_or_conflicting_duplicate_fails_closed(self):
        tag = "v" + fixture.VERSION
        selected = {"id": 314, "tag_name": tag, "draft": True}
        page = [selected] + [{"id": i, "tag_name": "v0.0." + str(i), "draft": False}
                             for i in range(1, 100)]
        for second in ([page[-1]], [{**page[-1], "draft": True}]):
            calls, record, error = self.invoke(post_pages=[{1: page, 2: second}])
            self.assertIsNotNone(error)
            self.assertNotEqual(record["status"], "reserved")
            self.assertEqual(record["release_id"], 314)
            self.assertEqual(len(self.creates(calls)), 1)

    def test_observed_competing_draft_is_not_forgotten_on_page_shift(self):
        tag = "v" + fixture.VERSION
        selected = {"id": 314, "tag_name": tag, "draft": True}
        competitor = {"id": 315, "tag_name": tag, "draft": True}
        others = [{"id": i, "tag_name": "v0.0." + str(i), "draft": False}
                  for i in range(1, 99)]
        page = [selected, competitor] + others
        calls, record, error = self.invoke(post_pages=[{1: page, 2: [others[-1]]},
                                                       {1: [selected]}])
        self.assertIsNotNone(error)
        self.assertIn('observed conflicting release identity', str(error))
        self.assertNotEqual(record['status'], 'reserved')
        self.assertEqual(len(self.creates(calls)), 1)

    def test_wrong_peeled_commit_before_or_after_discovery_never_creates(self):
        for heads in (["b" * 40], [fixture.HEAD, "b" * 40]):
            with self.subTest(heads=heads):
                calls, record, error = self.invoke(heads=heads)
                self.assertIsNotNone(error)
                self.assertFalse(record["creation_attempted"])
                self.assertEqual(self.creates(calls), [])

    def test_existing_tag_or_duplicate_ids_on_later_page_refuses(self):
        first = [{"id": i, "tag_name": "v0.0." + str(i), "draft": False} for i in range(1, 101)]
        for item in ({"id": 101, "tag_name": "v" + fixture.VERSION, "draft": True},
                     {"id": 1, "tag_name": "v0.0.101", "draft": False}):
            with self.subTest(item=item):
                calls, record, error = self.invoke(pages={1: first, 2: [item]})
                self.assertIsNotNone(error)
                self.assertFalse(record["creation_attempted"])
                self.assertEqual(self.creates(calls), [])
                self.assertIn("page=2", calls[-1][-1])

    def test_creation_ambiguous_or_invalid_identity_never_retries(self):
        tag = "v" + fixture.VERSION
        invalid = ({"id": 314, "tag_name": "v0.0.0", "draft": True},
                   {"id": 314, "tag_name": tag, "draft": False},
                   {"id": 0, "tag_name": tag, "draft": True},
                   {"id": True, "tag_name": tag, "draft": True},
                   {"id": 314, "tag_name": tag, "draft": True, "extra": 1})
        responses = [b"HTTP/2 201 Created\r\n\r\n" + json.dumps(value).encode() for value in invalid]
        responses += [b"HTTP/2 500 Error\r\n\r\n{}", b"unobserved response"]
        for response in responses:
            with self.subTest(response=response):
                calls, record, error = self.invoke(response=response)
                self.assertIsNotNone(error)
                self.assertEqual(record["status"], "create_outcome_unknown")
                self.assertTrue(record["creation_attempted"])
                self.assertNotIn("release_id", record)
                self.assertEqual(len(self.creates(calls)), 1)
                self.assertEqual(calls[-1], self.creates(calls)[0])
        calls, record, error = self.invoke(create_failure=True)
        self.assertIsNotNone(error)
        self.assertTrue(record["creation_attempted"])
        self.assertEqual(len(self.creates(calls)), 1)


class ReleaseRunCustodyFixtureTests(unittest.TestCase):
    tag = "v" + fixture.VERSION
    run_id = "12345"

    def invoke(self, *, runs=None, gh_output=None, returncode=0):
        calls = []
        if runs is None:
            runs = {"total_count": 1, "workflow_runs": [
                {"id": int(self.run_id), "head_branch": self.tag,
                 "head_sha": fixture.HEAD, "event": "push"}]}

        def transport(argv, **kwargs):
            calls.append(list(argv))
            if argv[:2] == ["gh", "api"]:
                data = gh_output if gh_output is not None else json.dumps(runs).encode()
                return subprocess.CompletedProcess(argv, returncode, data, b"")
            raise AssertionError("unexpected process " + repr(argv))

        environment = {"GITHUB_REF_NAME": self.tag, "GITHUB_RUN_ID": self.run_id}
        with mock.patch.dict(run_custody.os.environ, environment), \
                mock.patch.object(run_custody.sys, "argv", ["check-release-run-custody.py", fixture.HEAD]), \
                mock.patch.object(run_custody.subprocess, "run", side_effect=transport), \
                mock.patch("builtins.print") as output:
            try:
                run_custody.main()
                failure = None
            except (SystemExit, subprocess.TimeoutExpired, ValueError, TypeError, AttributeError) as error:
                failure = error
        return calls, failure, output

    def test_exact_run_is_a_bounded_check_not_historical_custody_proof(self):
        calls, failure, output = self.invoke()
        self.assertIsNone(failure)
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0][:4], ["gh", "api", "--hostname", "github.com"])
        self.assertIn("branch=" + self.tag, calls[0][-1])
        self.assertIn("event=push", calls[0][-1])
        self.assertIn("per_page=100&page=1", calls[0][-1])
        output.assert_called_once_with("bounded single tag-run listing confirmed; historical custody is not inferred")

    def test_cross_run_or_mismatched_run_refuses(self):
        current = {"id": int(self.run_id), "head_branch": self.tag,
                   "head_sha": fixture.HEAD, "event": "push"}
        for other in ({**current, "id": 999}, {**current, "head_sha": "b" * 40},
                      {**current, "head_branch": "v0.0.0"}, {**current, "event": "workflow_dispatch"}):
            for rows in ([other], [current, other]):
                with self.subTest(rows=rows):
                    calls, failure, _ = self.invoke(runs={"total_count": len(rows), "workflow_runs": rows})
                    self.assertIsNotNone(failure)
                    self.assertEqual(len(calls), 1)

    def test_incomplete_or_oversized_lists_refuse(self):
        current = {"id": int(self.run_id), "head_branch": self.tag,
                   "head_sha": fixture.HEAD, "event": "push"}
        for response in ({"total_count": 2, "workflow_runs": [current]},
                         {"total_count": 101, "workflow_runs": [current] * 101},
                         {"workflow_runs": [current]}):
            with self.subTest(response=response.get("total_count")):
                calls, failure, _ = self.invoke(runs=response)
                self.assertIsNotNone(failure)
                self.assertEqual(len(calls), 1)
        calls, failure, _ = self.invoke(gh_output=b" " * (2 * 1024 * 1024 + 1))
        self.assertIsNotNone(failure)
        self.assertEqual(len(calls), 1)
        calls, failure, _ = self.invoke(returncode=1)
        self.assertIsNotNone(failure)
        self.assertEqual(len(calls), 1)


class ReleaseCustodyFixtureTests(unittest.TestCase):
    def test_check_receipt_requires_exact_previous_run_identity_and_mount_metadata(self):
        cases = ("valid", "missing_receipt", "wrong_run", "wrong_attempt",
                 "missing_tag", "wrong_tag", "missing_commit", "wrong_commit",
                 "missing_metadata", "empty_metadata", "duplicate_receipt")
        for case in cases:
            with self.subTest(case=case), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                prior = root / "prior"
                prior.mkdir()
                record = {"tag": "v" + fixture.VERSION, "commit": fixture.HEAD,
                          "guard_attachment_metadata": "/inert/attachment.plist"}
                if case == "missing_tag":
                    del record["tag"]
                elif case == "wrong_tag":
                    record["tag"] = "v0.24.998"
                elif case == "missing_commit":
                    del record["commit"]
                elif case == "wrong_commit":
                    record["commit"] = "b" * 40
                elif case == "missing_metadata":
                    del record["guard_attachment_metadata"]
                elif case == "empty_metadata":
                    record["guard_attachment_metadata"] = ""
                filename = ("pantheon-publication-999-2.json" if case == "wrong_run" else
                            "pantheon-publication-123-1.json" if case == "wrong_attempt" else
                            "pantheon-publication-123-2.json")
                if case != "missing_receipt":
                    (prior / filename).write_text(json.dumps(record))
                if case == "duplicate_receipt":
                    nested = prior / "nested"
                    nested.mkdir()
                    (nested / filename).write_text(json.dumps(record))
                args = ["release-custody.py", "check-receipt", str(prior),
                        fixture.VERSION, fixture.HEAD, "ignored"]
                with mock.patch.object(custody.sys, "argv", args), \
                        mock.patch.dict(custody.os.environ, {"GITHUB_RUN_ID": "123", "GITHUB_RUN_ATTEMPT": "3"}), \
                        mock.patch.object(custody.Path, "cwd", return_value=root), \
                        mock.patch.object(custody.subprocess, "run", side_effect=AssertionError("real process forbidden")) as process, \
                        mock.patch.object(custody, "copy_exclusive", side_effect=AssertionError("restore forbidden")) as copy, \
                        mock.patch.object(custody, "inspect_app_archive", side_effect=AssertionError("restore forbidden")) as inspect:
                    if case == "valid":
                        self.assertIsNone(custody.main())
                    else:
                        with self.assertRaises(SystemExit):
                            custody.main()
                    process.assert_not_called()
                    copy.assert_not_called()
                    inspect.assert_not_called()
                self.assertFalse((root / "bin").exists())
                self.assertFalse((root / "Pantheon.app").exists())

    def test_archive_rejects_traversal_and_escaping_symlinks(self):
        cases = (
            ("Pantheon.app/../../outside", b"inert", False),
            ("/Pantheon.app/absolute", b"inert", False),
            ("Pantheon.app/link", b"../../outside", True),
            ("Pantheon.app/link", b"/outside", True),
            ("__MACOSX/link", b"../Pantheon.app/file", True),
        )
        for name, data, symlink in cases:
            with self.subTest(name=name, data=data), tempfile.TemporaryDirectory() as temporary:
                archive = Path(temporary) / "inert.zip"
                with zipfile.ZipFile(archive, "w") as output:
                    entry = zipfile.ZipInfo(name)
                    if symlink:
                        entry.create_system = 3
                        entry.external_attr = (stat.S_IFLNK | 0o777) << 16
                    output.writestr(entry, data)
                with mock.patch.object(custody.subprocess, "run", side_effect=AssertionError("real process forbidden")):
                    with self.assertRaises(RuntimeError):
                        custody.inspect_app_archive(archive)

    def test_archive_rejects_symlink_chain_that_escapes_after_resolution(self):
        # Each target normalizes lexically under Pantheon.app, but following
        # deep/link before processing the two '..' components escapes it.
        links = (("Pantheon.app/deep/link", "../shallow"),
                 ("Pantheon.app/escape", "deep/link/../../outside"))
        with tempfile.TemporaryDirectory() as temporary:
            archive = Path(temporary) / "inert.zip"
            with zipfile.ZipFile(archive, "w") as output:
                for name, target in links:
                    entry = zipfile.ZipInfo(name)
                    entry.create_system = 3
                    entry.external_attr = (stat.S_IFLNK | 0o777) << 16
                    output.writestr(entry, target)
            with mock.patch.object(custody.subprocess, "run", side_effect=AssertionError("real process forbidden")):
                with self.assertRaises(RuntimeError):
                    custody.inspect_app_archive(archive)

    def test_archive_rejects_entry_under_symlink_ancestor(self):
        with tempfile.TemporaryDirectory() as temporary:
            archive = Path(temporary) / "inert.zip"
            with zipfile.ZipFile(archive, "w") as output:
                entry = zipfile.ZipInfo("Pantheon.app/alias")
                entry.create_system = 3
                entry.external_attr = (stat.S_IFLNK | 0o777) << 16
                output.writestr(entry, "Contents")
                output.writestr("Pantheon.app/alias/inert", b"inert")
            with mock.patch.object(custody.subprocess, "run", side_effect=AssertionError("real process forbidden")):
                with self.assertRaises(RuntimeError):
                    custody.inspect_app_archive(archive)

    def test_archive_rejects_case_insensitive_symlink_collisions_and_chains(self):
        cases = (
            (("Pantheon.app/Alias", "Contents", True),
             ("Pantheon.app/alias", "inert", False)),
            (("Pantheon.app/Deep/Link", "../shallow", True),
             ("Pantheon.app/escape", "deep/link/../../outside", True)),
            (("Pantheon.app/Alias", "Contents", True),
             ("Pantheon.app/alias/inert", "inert", False)),
        )
        for entries in cases:
            with self.subTest(entries=entries), tempfile.TemporaryDirectory() as temporary:
                archive = Path(temporary) / "inert.zip"
                with zipfile.ZipFile(archive, "w") as output:
                    for name, data, symlink in entries:
                        entry = zipfile.ZipInfo(name)
                        if symlink:
                            entry.create_system = 3
                            entry.external_attr = (stat.S_IFLNK | 0o777) << 16
                        output.writestr(entry, data)
                with mock.patch.object(custody.subprocess, "run", side_effect=AssertionError("real process forbidden")):
                    with self.assertRaises(RuntimeError):
                        custody.inspect_app_archive(archive)

    def test_restore_rejects_identity_or_digest_mismatch_before_copy_or_extraction(self):
        for failure in ("commit", "version", "missing_entry", "digest"):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary) / "work"
                root.mkdir()
                retained = Path(temporary) / "retained"
                retained.mkdir()
                names = list(PAYLOADS) + ["payload-manifest.json", "Pantheon.app.zip"]
                hashes = {}
                for name in names:
                    data = ("inert " + name).encode()
                    (retained / name).write_bytes(data)
                    hashes[name] = hashlib.sha256(data).hexdigest()
                record = {"commit": fixture.HEAD, "version": fixture.VERSION, "sha256": hashes}
                if failure == "commit":
                    record["commit"] = "b" * 40
                elif failure == "version":
                    record["version"] = "0.24.998"
                elif failure == "missing_entry":
                    del hashes[GUARD]
                else:
                    hashes[GUARD] = "0" * 64
                (retained / "custody.json").write_text(json.dumps(record))
                args = ["release-custody.py", "restore", str(retained), fixture.VERSION,
                        fixture.HEAD, str(retained / "payload-manifest.json")]
                with mock.patch.object(custody.sys, "argv", args), \
                        mock.patch.object(custody.Path, "cwd", return_value=root), \
                        mock.patch.object(custody.subprocess, "run", side_effect=AssertionError("real process forbidden")) as process:
                    with self.assertRaisesRegex(SystemExit, "custody (identity|digest) mismatch"):
                        custody.main()
                    process.assert_not_called()
                self.assertFalse((root / "bin").exists())
                self.assertFalse((root / "Pantheon.app").exists())


class GuardReleaseSourceContractTests(unittest.TestCase):
    """Read source only: these assertions never execute shell or YAML commands."""

    @classmethod
    def setUpClass(cls):
        root = Path(__file__).resolve().parent.parent
        cls.workflow = (root / ".github/workflows/release.yml").read_text()
        cls.build = (root / "scripts/build-release-pretoolguard.sh").read_text()
        cls.publisher = (root / "scripts/publish-release-assets.py").read_text()

    def test_guard_build_is_inside_admission_and_before_both_app_builders(self):
        step = self.workflow.split("      - name: Build + sign + notarize DMG and PKG", 1)[1]
        step = step.split("      - name: Stage draft", 1)[0]
        admitted = step.index('"$HOME/.local/bin/sirsi-maat-run"')
        shell = step.index("-- /bin/bash -euo pipefail -c '", admitted)
        guard = step.index("bash scripts/ci-private-sdk.sh bash scripts/build-release-pretoolguard.sh", shell)
        dmg = step.index("bash scripts/ci-private-sdk.sh scripts/build-dmg.sh", guard)
        pkg = step.index("bash scripts/ci-private-sdk.sh scripts/build-pkg.sh", dmg)
        self.assertLess(pkg, step.index("\n            '", pkg))
        self.assertLess(shell, guard)

    def test_workflow_requires_three_asset_guard_publication_with_custody(self):
        stage = self.workflow.split("      - name: Stage draft", 1)[1]
        stage = stage.split("      - name:", 1)[0]
        self.assertIn("python3 -B scripts/publish-release-assets.py", stage)
        self.assertIn("--include-guard", stage)
        self.assertIn('--guard-manifest "${PANTHEON_RELEASE_CUSTODY:?}/payload-manifest.json"', stage)
        self.assertIn('--directory "${PANTHEON_RELEASE_INPUTS:?}/bin"', stage)
        self.assertIn('--app "$PANTHEON_RELEASE_INPUTS/Pantheon.app"', stage)
        self.assertIn("-arm64.dmg", stage)
        self.assertIn("-arm64.pkg", stage)
        self.assertIn('names.append("SirsiPantheon-" + version + "-arm64-pretoolguard.dmg")', self.publisher)
        self.assertIn("readback(release, True)", self.publisher)
        self.assertIn("len(assets) != len(names)", self.publisher)

    def test_publish_and_retry_use_fresh_bound_supervisor(self):
        stage = self.workflow.split("      - name: Stage draft", 1)[1].split("      - name:", 1)[0]
        build = self.workflow.split("      - name: Build + sign + notarize DMG and PKG", 1)[1].split("      - name:", 1)[0]
        binding = 'cmp -s "$HOME/.local/bin/sirsi-maat-run" scripts/maat/admitted-run.py'
        for section in (build, stage):
            self.assertIn(binding, section)
            self.assertIn("installed Ma’at supervisor does not match release source", section)
            self.assertLess(section.index(binding), section.index('--register-executor'))
        self.assertNotIn("if: github.run_attempt", stage)
        self.assertIn('--work "pantheon-publish-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"', stage)
        self.assertIn('--timeout 600 -- python3 -B scripts/publish-release-assets.py', stage)

    def test_first_attempt_builds_and_retry_restores_same_run_custody(self):
        def step(name):
            return self.workflow.split("      - name: " + name, 1)[1].split("\n      - name:", 1)[0]

        for name in ("Import Developer ID signing identities",
                     "Resolve enrolled or imported Developer ID identity",
                     "Preflight imported Developer ID Installer identity",
                     "Build + sign + notarize DMG and PKG",
                     "Preserve exact first-attempt publication inputs",
                     "Upload exact first-attempt publication inputs"):
            with self.subTest(step=name):
                self.assertIn("if: github.run_attempt == 1", step(name))
        upload = step("Upload exact first-attempt publication inputs")
        download = step("Restore exact prior-attempt publication inputs")
        previous = step("Identify previous publication attempt")
        receipt = step("Retrieve previous mount custody receipt")
        verify = step("Verify restored custody without rebuilding or signing")
        restore = step("Restore and verify retained inputs for publication")
        self.assertIn("if: github.run_attempt != 1", download)
        for section in (previous, receipt, verify):
            self.assertIn("if: github.run_attempt != 1", section)
        self.assertNotIn("if: github.run_attempt", restore)
        self.assertIn("actions/upload-artifact@v4", upload)
        self.assertIn("actions/download-artifact@v4", download)
        for section in (upload, download):
            self.assertIn("name: pantheon-release-custody-${{ github.run_id }}", section)
        self.assertIn("scripts/release-custody.py create", step("Preserve exact first-attempt publication inputs"))
        self.assertIn('PANTHEON_RELEASE_CUSTODY=$CUSTODY', step("Preserve exact first-attempt publication inputs"))
        self.assertIn("GITHUB_RUN_ATTEMPT - 1", previous)
        self.assertIn("actions/download-artifact@v4", receipt)
        self.assertIn("pantheon-publication-${{ github.run_id }}-${{ steps.previous_attempt.outputs.number }}", receipt)
        self.assertIn('scripts/release-custody.py check-receipt "$PRIOR"', verify)
        self.assertIn('set -euo pipefail', verify)
        self.assertLess(verify.index('check-receipt "$PRIOR"'), verify.index('PANTHEON_RELEASE_CUSTODY=$CUSTODY'))
        self.assertIn('PANTHEON_RELEASE_CUSTODY=$CUSTODY', verify)
        self.assertIn('CUSTODY="${PANTHEON_RELEASE_CUSTODY:?}"', restore)
        self.assertIn('mktemp -d "$RUNNER_TEMP/pantheon-release-inputs-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}.XXXXXXXX"', restore)
        self.assertIn("/bin/bash -euo pipefail -c 'cd \"$1\"; shift; exec python3 -B \"$@\"'", restore)
        self.assertIn('"$GITHUB_WORKSPACE/scripts/release-custody.py" restore', restore)
        self.assertIn('"$CUSTODY/payload-manifest.json"', restore)
        self.assertIn('PANTHEON_RELEASE_INPUTS=$RESTORED', restore)
        for section, work in ((step("Preserve exact first-attempt publication inputs"), 'pantheon-custody-create-'),
                              (restore, 'pantheon-custody-restore-')):
            self.assertIn('cmp -s "$HOME/.local/bin/sirsi-maat-run" scripts/maat/admitted-run.py', section)
            self.assertIn('--register-executor', section)
            self.assertIn(work, section)
            self.assertIn('--cpu-cores 1 --timeout 600 --', section)
        self.assertLess(restore.index('restore "$CUSTODY"'), restore.index('PANTHEON_RELEASE_INPUTS=$RESTORED'))
        for forbidden in ("scripts/build-release-pretoolguard.sh", "scripts/build-dmg.sh",
                          "scripts/build-pkg.sh", "scripts/sign-pretoolguard-artifact.sh"):
            self.assertNotIn(forbidden, download + receipt + verify + restore)
        self.assertLess(self.workflow.index("scripts/release-custody.py check-receipt"),
                        self.workflow.index('scripts/release-custody.py" restore'))
        self.assertLess(self.workflow.index('scripts/release-custody.py" restore'),
                        self.workflow.index("scripts/publish-release-assets.py"))

    def test_source_repo_signer_and_qualified_host_identity_are_bound(self):
        build_step = self.workflow.split("      - name: Build + sign + notarize DMG and PKG", 1)[1]
        build_step = build_step.split("      - name: Stage draft", 1)[0]
        signer = 'export PANTHEON_SIGN_CLIENT="$GITHUB_WORKSPACE/tools/signing/sirsi-sign"'
        self.assertIn(signer, build_step)
        self.assertLess(build_step.index(signer), build_step.index("scripts/build-release-pretoolguard.sh"))
        self.assertIn("SIRSI_SIGN_HOSTS: sirsi-m1", build_step)
        self.assertIn("StrictHostKeyChecking=yes", self.build)
        self.assertIn("BatchMode=yes", self.build)
        self.assertIn("IOPlatformUUID", self.build)
        self.assertIn('"$remote_host" == "$local_host"', self.build)
        self.assertLess(self.build.index('"$remote_host" == "$local_host"'),
                        self.build.index("scripts/sign-pretoolguard-artifact.sh"))

    def test_private_attempt_and_exclusive_custody_output(self):
        self.assertIn('mktemp -d "$RUNNER_TEMP/pantheon-guard-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}.XXXXXXXX"', self.workflow)
        self.assertIn("$(/usr/bin/stat -f '%u:%Lp' \"$attempt\") == \"$(id -u):700\"", self.build)
        self.assertIn('! -L "$attempt"', self.build)
        self.assertIn('mkdir -m 700 "$attempt/build" "$attempt/guard" "$attempt/installer" "$attempt/package"', self.build)
        self.assertIn('"$attempt/payload-manifest.json"', self.build)
        self.assertIn("open(target, 'xb')", self.build)
        self.assertIn("open(manifest, 'x')", self.build)


if __name__ == "__main__":
    unittest.main()
