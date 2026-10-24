"""Observed native XML and inert mutations only; no pkgbuild or Installer."""
import hashlib
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest

import package_info_contract as contract


class PackageInfoContractTests(unittest.TestCase):
    def setUp(self):
        self.data = (Path(__file__).parent / "testdata/pkgbuild-native-upgrade-PackageInfo.xml").read_bytes()

    def validate(self, data):
        contract.validate(data, "1.0", "1", "ai.sirsi.pantheon.metadata-fixture", package_version="1")

    def test_real_native_metadata_semantics_and_provenance(self):
        # Source fixture adds a final newline to the retained no-newline native
        # XML. Removing only that newline binds the exact observed artifact.
        self.assertEqual(hashlib.sha256(self.data.rstrip(b"\n")).hexdigest(), "ec7474e00ec3b038ba29cda56210662d31fdd08e773155edf3fbc21095ae423a")
        self.validate(self.data)

    def test_marker_mutations_fail(self):
        mutations = [
            self.data.replace(b"upgrade-bundle", b"renamed-marker"),
            self.data.replace(b"<upgrade-bundle>", b"<upgrade-bundle extra='unknown'>"),
            self.data.replace(b"<update-bundle/>", b'<update-bundle><bundle id="ai.sirsi.pantheon"/></update-bundle>'),
            self.data.replace(b"<atomic-update-bundle/>", b'<atomic-update-bundle><bundle id="ai.sirsi.pantheon"/></atomic-update-bundle>'),
            self.data.replace(b"<relocate/>", b'<relocate><bundle id="ai.sirsi.pantheon"/></relocate>'),
            self.data.replace(b"<upgrade-bundle>", b'<upgrade-bundle><bundle id="ai.sirsi.pantheon"/>'),
            self.data.replace(b'overwrite-permissions="true"', b'overwrite-permissions="false"'),
            self.data.replace(b'relocatable="false"', b'relocatable="true"'),
            self.data.replace(b'path="./Applications/Pantheon.app"', b'path="./Applications/Other.app"'),
            self.data.replace(b'CFBundleVersion="1"', b'CFBundleVersion="2"'),
            self.data.replace(b'CFBundleShortVersionString="1.0"', b'CFBundleShortVersionString="9.0"'),
            self.data.replace(b'<strict-identifier>', b'<strict-identifier><bundle id="other"/>'),
            self.data + b" " * contract.LIMIT,
        ]
        for index, data in enumerate(mutations):
            with self.subTest(index=index), self.assertRaises(ValueError):
                self.validate(data)

    def test_actual_pkg_helper_invocation_imports_without_bytecode_side_effects(self):
        scripts = Path(__file__).resolve().parent
        # Extract the official invocation, including its actual interpreter
        # flags. Only copied Python/XML fixtures run; no shell/pkgbuild/signing.
        line = next(line for line in (scripts / "build-pkg.sh").read_text().splitlines()
                    if line.startswith("python3 ") and "package_info_contract.py" in line)
        tokens = shlex.split(line)
        with tempfile.TemporaryDirectory(prefix="owned-package-info-bytecode-") as temp:
            root = Path(temp).resolve()
            owned_scripts = root / "scripts"
            owned_scripts.mkdir()
            for name in ("package_info_contract.py", "artifact_modes.py"):
                shutil.copyfile(scripts / name, owned_scripts / name)
            info = root / "PackageInfo"
            # Adapt only the observed probe's package identity/version and app
            # build to the official CLI's coherent canonical VERSION defaults.
            data = self.data.replace(b'identifier="ai.sirsi.pantheon.metadata-fixture"', b'identifier="ai.sirsi.pantheon"')
            data = data.replace(b'version="1"', b'version="1.0"', 1)
            info.write_bytes(data.replace(b'CFBundleVersion="1"', b'CFBundleVersion="1.0"'))
            replacements = {
                "${PROJECT_ROOT}/scripts/package_info_contract.py": str(owned_scripts / "package_info_contract.py"),
                "$EXPANDED_ROOT/PackageInfo": str(info), "$VERSION": "1.0",
            }
            argv = [sys.executable, *(replacements.get(token, token) for token in tokens[1:])]
            env = {"HOME": str(root), "TMPDIR": str(root), "PATH": os.defpath}
            result = subprocess.run(argv, env=env, cwd=root, capture_output=True, text=True, timeout=10, check=False)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("PackageInfo fixed atomic upgrade metadata accepted", result.stdout)
            self.assertEqual(list(root.rglob("__pycache__")), [])
            self.assertEqual(list(root.rglob("*.pyc")), [])
            self.assertEqual(tokens[:2], ["python3", "-B"])

    def test_all_official_release_python_helpers_disable_bytecode_explicitly(self):
        scripts = Path(__file__).resolve().parent
        calls = []
        for name in ("build-dmg.sh", "build-pkg.sh"):
            calls.extend(line for line in (scripts / name).read_text().splitlines()
                         if line.startswith("python3 ") and any(helper in line for helper in ("artifact_modes.py", "package_info_contract.py")))
        self.assertEqual(len(calls), 6)
        for line in calls:
            self.assertEqual(shlex.split(line)[:2], ["python3", "-B"], line)
        workflow = (scripts.parent / ".github/workflows/release.yml").read_text()
        self.assertIn("python3 -B scripts/publish-release-assets.py", workflow)


if __name__ == "__main__":
    unittest.main()
