"""Owned inert staging only: no signing, pkgbuild, install or real app changes."""
import hashlib
import importlib.util
import os
import plistlib
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("artifact_modes", Path(__file__).with_name("artifact_modes.py"))
MODES = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODES)


class ArtifactModesTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name).resolve()

    def bundle(self, path, umask):
        old = os.umask(umask)
        try:
            for relative in ("Contents/MacOS/sirsi", "Contents/MacOS/sirsi-menubar", "Contents/Info.plist", "Contents/PkgInfo", "Contents/Resources/StackLab/recipe.json", "Contents/Resources/Apollo/native-profile.json"):
                leaf = path / relative
                leaf.parent.mkdir(parents=True, exist_ok=True)
                leaf.write_bytes(("inert owned " + relative).encode())
        finally:
            os.umask(old)
        return path

    def hashes(self, root):
        return {str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest() for path in root.rglob("*") if path.is_file()}

    def assert_public(self, root, payload=False):
        # Evaluate OTHER bits as a non-owning ordinary UID would after root:wheel
        # installation. os.access as the current owner/root is NOT an oracle.
        for path in [root, *root.rglob("*")]:
            relative = str(path.relative_to(root)) if path != root else ""
            mode = stat.S_IMODE(path.lstat().st_mode)
            expected = MODES.expected_mode(relative, path.is_dir(), payload)
            self.assertEqual(mode, expected, relative)
            self.assertTrue(mode & stat.S_IROTH, relative)
            if path.is_dir() or expected == 0o755:
                self.assertTrue(mode & stat.S_IXOTH, relative)
            else:
                self.assertFalse(mode & 0o111, relative)

    def test_bundle_umask077_and022_same_public_modes_bytes(self):
        outputs = []
        for mask in (0o077, 0o022):
            with self.subTest(umask=oct(mask)):
                root = self.bundle(self.root / ("bundle-" + oct(mask)), mask)
                before = self.hashes(root)
                MODES.normalize(root)
                self.assert_public(root)
                self.assertEqual(self.hashes(root), before)
                MODES.normalize(root, check=True)
                outputs.append(before)
        self.assertEqual(outputs[0], outputs[1])

    def test_payload_umask077_and022_system_cli_ancestors_public(self):
        for mask in (0o077, 0o022):
            with self.subTest(umask=oct(mask)):
                root = self.root / ("payload-" + oct(mask))
                old = os.umask(mask)
                try:
                    app = self.bundle(root / MODES.APP_PREFIX, mask)
                    system = root / MODES.SYSTEM_CLI
                    system.parent.mkdir(parents=True)
                    system.write_bytes((app / "Contents/MacOS/sirsi").read_bytes())
                finally:
                    os.umask(old)
                before = self.hashes(root)
                MODES.normalize(root, payload=True)
                self.assert_public(root, payload=True)
                self.assertEqual(self.hashes(root), before)
                self.assertEqual(system.read_bytes(), (app / "Contents/MacOS/sirsi").read_bytes())
                MODES.normalize(root, payload=True, check=True)

    def test_owner_readable0700_bundle_is_not_public(self):
        root = self.bundle(self.root / "Pantheon.app", 0o077)
        self.assertTrue((root / "Contents/Info.plist").read_bytes())
        with self.assertRaisesRegex(ValueError, "public mode mismatch"):
            MODES.normalize(root, check=True)

    def test_refuse_symlink_and_special_before_any_chmod(self):
        for kind in ("leaf-link", "directory-link", "fifo", "extra-executable", "unexpected-root-file", "hardlink"):
            with self.subTest(kind=kind):
                root = self.bundle(self.root / kind, 0o077)
                target = self.root / (kind + "-sentinel")
                target.write_bytes(b"unchanged")
                os.chmod(target, 0o600)
                if kind == "leaf-link":
                    (root / "Contents/Resources/link").symlink_to(target)
                elif kind == "directory-link":
                    (root / "Contents/Resources/link").symlink_to(self.root, target_is_directory=True)
                elif kind == "fifo":
                    os.mkfifo(root / "Contents/Resources/fifo", 0o600)
                elif kind == "extra-executable":
                    path = root / "Contents/MacOS/unreviewed"
                    path.write_bytes(b"never execute")
                    os.chmod(path, 0o755)
                elif kind == "unexpected-root-file":
                    (root / "unexpected").write_bytes(b"refuse")
                else:
                    os.link(target, root / "Contents/Resources/hardlink")
                original_mode = root.stat().st_mode
                with self.assertRaises((OSError, ValueError)):
                    MODES.normalize(root)
                self.assertEqual(root.stat().st_mode, original_mode)
                self.assertEqual(target.read_bytes(), b"unchanged")
                self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o600)

    def test_root_and_parent_symlinks_refused(self):
        root = self.bundle(self.root / "real" / "Pantheon.app", 0o077)
        (self.root / "alias").symlink_to(root.parent, target_is_directory=True)
        (self.root / "bundle-alias").symlink_to(root, target_is_directory=True)
        for alias in (self.root / "alias/Pantheon.app", self.root / "bundle-alias"):
            with self.assertRaises(OSError):
                MODES.normalize(alias)

    def test_only_exact_executables_survive_resource_execute_bits(self):
        root = self.bundle(self.root / "Pantheon.app", 0o077)
        resource = root / "Contents/Resources/StackLab/recipe.json"
        os.chmod(resource, 0o777)
        MODES.normalize(root)
        self.assertEqual(stat.S_IMODE(resource.stat().st_mode), 0o644)
        actual = {str(path.relative_to(root)) for path in root.rglob("*") if path.is_file() and path.stat().st_mode & 0o111}
        self.assertEqual(actual, MODES.BUNDLE_EXECUTABLES)

    def test_signature_metadata_check_is_readonly(self):
        root = self.bundle(self.root / "Pantheon.app", 0o077)
        signature = root / "Contents/_CodeSignature/CodeResources"
        signature.parent.mkdir()
        signature.write_bytes(b"inert signature metadata, not signed")
        MODES.normalize(root)
        self.assert_public(root)
        before = self.hashes(root)
        MODES.normalize(root, check=True)
        self.assertEqual(self.hashes(root), before)

    def test_actual_build_entry_wiring_precedes_sign_and_pkgbuild(self):
        scripts = Path(__file__).resolve().parent
        dmg = (scripts / "build-dmg.sh").read_text()
        pkg = (scripts / "build-pkg.sh").read_text()
        self.assertLess(dmg.index('artifact_modes.py" --bundle "${BUNDLE_DIR}"'), dmg.index("# --- Code signing ---"))
        self.assertIn('artifact_modes.py" --bundle "${BUNDLE_DIR}" --check', dmg)
        self.assertLess(pkg.index('artifact_modes.py" --payload "$PAYLOAD_ROOT"'), pkg.index('COPYFILE_DISABLE=1 /usr/bin/pkgbuild'))
        self.assertIn('artifact_modes.py" --payload "$EXPANDED_ROOT/Payload" --check', pkg)

    def test_private_previous_bundle_uses_native_upgrade_not_merge(self):
        # Native Installer semantics are pkgbuild(1)'s contract, not a mock OS.
        scripts = Path(__file__).resolve().parent
        with (scripts / "pantheon-pkg-components.plist").open("rb") as stream:
            components = plistlib.load(stream)
        self.assertEqual(components, [{
            "RootRelativeBundlePath": "Applications/Pantheon.app",
            "BundleIsRelocatable": False,
            "BundleIsVersionChecked": True,
            "BundleHasStrictIdentifier": True,
            "BundleOverwriteAction": "upgrade",
        }])
        pkg = (scripts / "build-pkg.sh").read_text()
        self.assertIn('--component-plist "${PROJECT_ROOT}/scripts/pantheon-pkg-components.plist"', pkg)
        previous = self.bundle(self.root / "previous" / "Pantheon.app", 0o077)
        candidate = self.bundle(self.root / "candidate" / "Pantheon.app", 0o077)
        MODES.normalize(candidate)
        self.assertEqual(stat.S_IMODE(previous.stat().st_mode), 0o700)
        self.assert_public(candidate)
        self.assertNotEqual(stat.S_IMODE(previous.stat().st_mode), stat.S_IMODE(candidate.stat().st_mode))
        # No Installer or swap simulation is claimed as actual upgrade proof.

    def test_actual_pkg_entry_refuses_private_input_before_staging(self):
        scripts = Path(__file__).resolve().parent
        fence = self.root / "owned-build-fence.sh"
        # Test-only Bash DEBUG fence, not a production bypass. Even a mutant
        # omitting the input check stops BEFORE mkdir/staging/pkgbuild/signing.
        fence.write_text("""set -o functrace
trap 'case "$BASH_COMMAND" in
  *BUILD_DIR*mkdir*|*mkdir*BUILD_DIR*) echo OWNED_FIXTURE_STOP_BEFORE_STAGING >&2; exit 97;;
  */usr/bin/pkgbuild*|*/usr/bin/ditto*|*codesign*|*productsign*|*notarytool*|*stapler*) echo OWNED_FIXTURE_FORBIDDEN_TOOL >&2; exit 98;;
esac' DEBUG
""")
        for failure in ("private-root", "private-info", "valid-control"):
            with self.subTest(failure=failure):
                root = self.bundle(self.root / failure / "Pantheon.app", 0o077)
                MODES.normalize(root)
                if failure == "private-root":
                    os.chmod(root, 0o700)
                elif failure == "private-info":
                    os.chmod(root / "Contents/Info.plist", 0o600)
                before = self.hashes(root)
                before_modes = {str(path): path.stat().st_mode for path in [root, *root.rglob("*")]}
                env = {"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": str(self.root), "TMPDIR": str(self.root), "BASH_ENV": str(fence), "PYTHONDONTWRITEBYTECODE": "1"}
                result = subprocess.run(["/bin/bash", str(scripts / "build-pkg.sh"), "--development", "--version", "0.0.0-fixture", "--app", str(root)], env=env, capture_output=True, text=True, timeout=10, check=False)
                if failure == "valid-control":
                    self.assertEqual(result.returncode, 97, result.stderr)
                    self.assertIn("OWNED_FIXTURE_STOP_BEFORE_STAGING", result.stderr)
                else:
                    self.assertEqual(result.returncode, 1, result.stderr)
                    self.assertIn("artifact modes refused: public mode mismatch", result.stderr)
                    self.assertNotIn("OWNED_FIXTURE_STOP", result.stderr)
                self.assertEqual(self.hashes(root), before)
                self.assertEqual({str(path): path.stat().st_mode for path in [root, *root.rglob("*")]}, before_modes)


if __name__ == "__main__":
    unittest.main()
