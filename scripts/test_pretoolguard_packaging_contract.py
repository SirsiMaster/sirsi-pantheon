"""Inert contracts and mocked helper checks: no builds, signing, mounts, or network.

Shell checks inspect text, not execution or artifacts. Helper checks replace
subprocess.run in-process; they never execute codesign. Neither proves a release.
"""

from pathlib import Path
import plistlib
import runpy
import subprocess
import os
import tempfile
import unittest
from unittest import mock


SCRIPTS = Path(__file__).resolve().parent


class PretoolguardPackagingContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = (SCRIPTS / "build-pretoolguard-artifact.sh").read_text()
        cls.sign = (SCRIPTS / "sign-pretoolguard-artifact.sh").read_text()
        cls.package = (SCRIPTS / "package-pretoolguard-artifact.sh").read_text()

    def ordered(self, source, *parts):
        position = 0
        for part in parts:
            found = source.find(part, position)
            self.assertNotEqual(found, -1, f"missing or out of order: {part!r}")
            position = found + len(part)

    def test_builder_emits_two_distinct_arm64_artifacts_and_digests(self):
        self.ordered(
            self.build,
            '[[ "$actual" == "$expected" ]]',
            'git status --porcelain --untracked-files=normal',
            '$(uname -s) == Darwin && $(uname -m) == arm64',
            "'INCOMPLETE: do not use; inspect retained evidence'",
            'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOWORK=off',
            'go build -trimpath -o "$dir/sirsi-display-power-guard" ./cmd/sirsi-pretoolguard',
            'CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 GOWORK=off',
            'go build -trimpath -o "$dir/sirsi-pretoolguard-install" ./cmd/sirsi-pretoolguard-install',
            'shasum -a 256 "$dir/sirsi-display-power-guard" > "$dir/unsigned.sha256"',
            'shasum -a 256 "$dir/sirsi-pretoolguard-install" > "$dir/installer-unsigned.sha256"',
            'mv "$dir/INCOMPLETE" "$dir/ACCEPTED"',
        )

    def test_signer_exact_names_m1_route_and_private_digest_rechecks(self):
        self.ordered(
            self.sign,
            '[[ "$name" == sirsi-display-power-guard || "$name" == sirsi-pretoolguard-install ]] ||',
            '[[ ! -e "$dir/signed-$name"',
            '== "$expected" ]] || { echo \'unsigned digest mismatch\' >&2; exit 1; }',
            '"$PANTHEON_SIGN_CLIENT" == "$root/tools/signing/sirsi-sign"',
            '"$SIRSI_SIGN_HOSTS" == sirsi-m1',
            'job=$(mktemp -d "$dir/signing.XXXXXXXX")',
            "'INCOMPLETE: do not use; signing outcome may be ambiguous'",
            '/bin/cp -p "$file" "$job/$name"',
            'file="$job/$name"',
            '== "$expected" ]] || { echo \'private signing input digest mismatch\' >&2; exit 1; }',
            '"$PANTHEON_SIGN_CLIENT" "$file" bin',
            'signed="$dir/signed-$name"',
            '"$signed.receipt.txt"',
        )

    def test_signer_verifies_identity_and_normalized_payload(self):
        self.ordered(
            self.sign,
            'certificate leaf[subject.OU] = \\"9D382WV988\\"',
            '/usr/bin/codesign --verify --strict -R "$requirement" "$signed"',
            '/usr/bin/python3 -B "$root/scripts/verify-pretoolguard-entitlements.py" "$signed"',
            "'^Timestamp='",
            "'^CodeDirectory .*flags=.*\\(.*runtime.*\\)'",
            '/bin/cp -p "$file" "$norm/input"',
            '/bin/cp -p "$signed" "$norm/output"',
            "'signing input changed during signing' >&2; exit 1;",
            'if /usr/bin/codesign -d "$norm/input"',
            '/usr/bin/codesign --remove-signature "$norm/input"',
            '/usr/bin/codesign --remove-signature "$norm/output"',
            '/usr/bin/cmp -s "$norm/input" "$norm/output" ||',
            '/usr/bin/shasum -a 256 "$signed" > "$dir/signed.sha256"',
            'mv "$dir/INCOMPLETE" "$dir/ACCEPTED"',
        )

    def test_packaging_checks_both_staged_and_mounted_binaries(self):
        self.ordered(
            self.package,
            'if [ "$#" -ne 7 ]; then',
            'binary=$1\nexpected=$2\nidentifier=$3\ninstaller=$4\ninstaller_expected=$5\ninstaller_identifier=$6\noutput=$7',
            '[[ "$binary" = /* && -f "$binary" && ! -L "$binary" ]]',
            '[[ "$installer" = /* && -f "$installer" && ! -L "$installer" ]]',
            '"$installer_expected" =~ ^[0-9a-f]{64}$',
            '"$installer_identifier" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$',
            '"$PANTHEON_SIGN_CLIENT" == "$root/tools/signing/sirsi-sign"',
            '"$SIRSI_SIGN_HOSTS" == sirsi-m1',
            'installer_requirement="=identifier \\"$installer_identifier\\"',
            '/usr/bin/codesign --verify --strict -R "$requirement" "$binary"',
            "'guard digest mismatch' >&2; exit 1;",
            '/usr/bin/codesign --verify --strict -R "$installer_requirement" "$installer"',
            "'installer digest mismatch' >&2; exit 1;",
            "'INCOMPLETE: do not use; signing or mount outcome may be ambiguous'",
            '/bin/cp -p "$binary" "$dir/payload/sirsi-display-power-guard"',
            '/bin/cp -p "$installer" "$dir/payload/sirsi-pretoolguard-install"',
            '/bin/bash "$root/scripts/normalize-pretoolguard-payload-modes.sh" "$dir/payload"',
            "'staged guard digest mismatch' >&2; exit 1;",
            '/usr/bin/codesign --verify --strict -R "$requirement" "$dir/payload/sirsi-display-power-guard"',
            "'staged installer digest mismatch' >&2; exit 1;",
            '/usr/bin/codesign --verify --strict -R "$installer_requirement" "$dir/payload/sirsi-pretoolguard-install"',
            '/usr/bin/python3 -B "$root/scripts/verify-pretoolguard-entitlements.py" "$dir/payload/sirsi-display-power-guard"',
            '/usr/bin/python3 -B "$root/scripts/verify-pretoolguard-entitlements.py" "$dir/payload/sirsi-pretoolguard-install"',
            "'guard secure timestamp not reported' >&2; exit 1;",
            "'guard hardened runtime flag not reported' >&2; exit 1;",
            "'installer secure timestamp not reported' >&2; exit 1;",
            "'installer hardened runtime flag not reported' >&2; exit 1;",
            '/usr/bin/hdiutil create -quiet -format UDZO -layout NONE -fs HFS+ -srcfolder "$dir/payload" "$dir/pretoolguard.dmg"',
            '"$PANTHEON_SIGN_CLIENT" "$dir/pretoolguard.dmg" dmg',
            '"$signed.receipt.txt"',
            '/usr/bin/xcrun stapler validate "$signed"',
            'trap cleanup_mount EXIT',
            '/usr/bin/hdiutil attach -plist -readonly -nobrowse -mountpoint "$dir/mount" "$signed"',
            "unexpected unmounted image device entities",
            'mounted=1',
            "names = {'sirsi-display-power-guard', 'sirsi-pretoolguard-install'}",
            'if set(os.listdir(p)) != names:',
            'for name in names:',
            's = os.lstat(os.path.join(p, name))',
            'if not stat.S_ISREG(s.st_mode) or s.st_nlink != 1 or stat.S_IMODE(s.st_mode) != 0o755:',
            '"$dir/mount/sirsi-display-power-guard" | /usr/bin/awk',
            "'returned image payload differs' >&2; exit 1;",
            '/usr/bin/codesign --verify --strict -R "$requirement" "$dir/mount/sirsi-display-power-guard"',
            '"$dir/mount/sirsi-pretoolguard-install" | /usr/bin/awk',
            '== "$installer_expected" ]] || { echo \'returned image installer differs\' >&2; exit 1; }',
            '/usr/bin/codesign --verify --strict -R "$installer_requirement" "$dir/mount/sirsi-pretoolguard-install"',
            'if ! /usr/bin/hdiutil detach "$device"; then',
            'exit 1\nfi',
            'printf \'%s\\n\' "$expected" > "$dir/guard.sha256"',
            'printf \'%s\\n\' "$installer_expected" > "$dir/installer.sha256"',
            'mv "$dir/INCOMPLETE" "$dir/ACCEPTED"',
        )

    def test_failure_paths_and_cleanup_are_present(self):
        for source in (self.build, self.sign, self.package):
            with self.subTest(script=source[:80]):
                self.assertIn('set -euo pipefail', source)
                self.assertIn('"$dir/INCOMPLETE"', source)
                self.assertIn('mv "$dir/INCOMPLETE" "$dir/ACCEPTED"', source)
        self.ordered(
            self.package,
            'cleanup_mount() {',
            'if [[ "$mounted" == 1 ]]; then',
            '/usr/bin/hdiutil detach "$device" || echo "WARNING: detach failed;',
            'trap cleanup_mount EXIT',
        )

    def test_restrictive_umask_staged_modes_normalized(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            src = root / "source"
            src.mkdir(mode=0o700)
            payload = root / "payload"
            names = ("sirsi-display-power-guard", "sirsi-pretoolguard-install")
            for name, mode in zip(names, (0o700, 0o750)):
                (src / name).write_bytes(b"inert fixture")
                (src / name).chmod(mode)
            begin = self.package.index('mkdir -m 700 "$dir/payload"')
            end = self.package.index('\n', self.package.index(
                '/bin/bash "$root/scripts/normalize-pretoolguard-payload-modes.sh" "$dir/payload"', begin))
            staging = self.package[begin:end]
            subprocess.run(
                ["/bin/bash", "-c", 'set -e; umask 077; dir="$1"; binary="$2"; installer="$3"; root="$4"; ' + staging,
                 "fixture", str(root), str(src / names[0]), str(src / names[1]), str(SCRIPTS.parent)],
                check=True, timeout=10,
            )
            self.assertEqual(payload.stat().st_mode & 0o7777, 0o700)
            for name, original_mode in zip(names, (0o700, 0o750)):
                self.assertEqual((payload / name).stat().st_mode & 0o7777, 0o755)
                self.assertEqual((src / name).stat().st_mode & 0o7777, original_mode)

    def test_normalizer_refuses_hard_link_without_chmod(self):
        names = ("sirsi-display-power-guard", "sirsi-pretoolguard-install")
        for link_name, other_name in (names, names[::-1]):
            with self.subTest(link_name=link_name), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                payload = root / "payload"
                payload.mkdir(mode=0o700)
                original = root / "outside"
                original.write_bytes(b"inert fixture")
                original.chmod(0o700)
                os.link(original, payload / link_name)
                other = payload / other_name
                other.write_bytes(b"inert fixture")
                other.chmod(0o700)
                result = subprocess.run(
                    ["/bin/bash", str(SCRIPTS / "normalize-pretoolguard-payload-modes.sh"), str(payload)],
                    capture_output=True, timeout=10, check=False,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(original.stat().st_mode & 0o7777, 0o700)
                self.assertEqual(other.stat().st_mode & 0o7777, 0o700)


class EntitlementHelperTests(unittest.TestCase):
    helper = SCRIPTS / "verify-pretoolguard-entitlements.py"
    artifact = "/inert/signed-artifact"

    def invoke(self, result=None, *, side_effect=None, args=None):
        argv = [str(self.helper), *(args if args is not None else [self.artifact])]
        with mock.patch("sys.argv", argv), mock.patch(
            "subprocess.run", return_value=result, side_effect=side_effect
        ) as run:
            try:
                runpy.run_path(str(self.helper), run_name="__main__")
            finally:
                if len(argv) == 2:
                    run.assert_called_once_with(
                        ["/usr/bin/codesign", "-d", "--entitlements", ":-", argv[1]],
                        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                        timeout=30, check=False,
                    )
                else:
                    run.assert_not_called()

    @staticmethod
    def result(stdout=b"", stderr=b"", returncode=0):
        return subprocess.CompletedProcess([], returncode, stdout, stderr)

    def test_no_entitlements_and_empty_dictionaries_are_accepted(self):
        for data in (b"", plistlib.dumps({}), plistlib.dumps({}, fmt=plistlib.FMT_BINARY)):
            with self.subTest(data=data):
                self.invoke(self.result(stdout=data))

    def test_declared_entitlements_and_non_dictionary_plists_are_rejected(self):
        for value in ({"com.apple.security.get-task-allow": False}, [], "value", True):
            with self.subTest(value=value), self.assertRaisesRegex(SystemExit, "unexpected entitlements"):
                self.invoke(self.result(stdout=plistlib.dumps(value)))

    def test_malformed_and_failed_or_oversized_output_are_rejected(self):
        cases = (
            (self.result(stdout=b"not a plist"), "malformed entitlements"),
            (self.result(returncode=1), "cannot verify entitlement policy"),
            (self.result(stdout=b"x" * 65537), "cannot verify entitlement policy"),
            (self.result(stderr=b"x" * 65537), "cannot verify entitlement policy"),
        )
        for result, message in cases:
            with self.subTest(message=message, result=result), self.assertRaisesRegex(SystemExit, message):
                self.invoke(result)
        # The cap is inclusive; stderr is diagnostic, not entitlement content.
        self.invoke(self.result(stderr=b"x" * 65536))

    def test_timeout_and_launch_error_propagate_and_wrong_arity_is_rejected(self):
        for error in (subprocess.TimeoutExpired("codesign", 30), OSError("unavailable")):
            with self.subTest(error=error), self.assertRaises(type(error)):
                self.invoke(side_effect=error)
        for args in ([], [self.artifact, "extra"]):
            with self.subTest(args=args), self.assertRaisesRegex(SystemExit, "usage:"):
                self.invoke(args=args)


if __name__ == "__main__":
    unittest.main()
