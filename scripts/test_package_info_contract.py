"""Observed native XML and inert mutations only; no pkgbuild or Installer."""
import hashlib
from pathlib import Path
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


if __name__ == "__main__":
    unittest.main()
