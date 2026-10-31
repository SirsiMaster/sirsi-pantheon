"""Owned inert staging only: no signing, pkgbuild, install or real app changes."""
import hashlib
import importlib.util
import os
import plistlib
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import textwrap
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
        self.assertLess(dmg.index('"${BUILD_DIR}/sirsi" package-inventory'), dmg.index("# --- Code signing ---"))
        self.assertIn('artifact_modes.py" --bundle "${BUNDLE_DIR}" --check', dmg)
        self.assertLess(pkg.index('artifact_modes.py" --payload "$PAYLOAD_ROOT"'), pkg.index('COPYFILE_DISABLE=1 /usr/bin/pkgbuild'))
        self.assertLess(pkg.index('"${BUILD_DIR}/sirsi" package-inventory'), pkg.index('COPYFILE_DISABLE=1 /usr/bin/pkgbuild'))
        self.assertIn('artifact_modes.py" --payload "$EXPANDED_ROOT/Payload" --check', pkg)

    def test_apollo_digest_selection_is_fresh_for_repeated_same_version_builds(self):
        selector = Path(__file__).with_name("new-apollo-digest-receipt.sh")
        same_build_output = self.root / "0.24.147-arm64"
        selected = []
        for _ in range(2):
            result = subprocess.run(
                ["/bin/bash", str(selector), str(same_build_output)],
                capture_output=True, text=True, timeout=10, check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            receipt = Path(result.stdout.strip())
            self.assertTrue(receipt.is_absolute())
            self.assertFalse(receipt.exists())
            self.assertTrue(receipt.parent.is_dir())
            selected.append(receipt)
        self.assertNotEqual(selected[0], selected[1])

    def test_profile_inventory_argv_under_macos_bash_32(self):
        if sys.platform != "darwin":
            self.skipTest("the production regression target is macOS /bin/bash 3.2")
        version = subprocess.run(
            ["/bin/bash", "-c", 'printf "%s.%s" "${BASH_VERSINFO[0]}" "${BASH_VERSINFO[1]}"'],
            capture_output=True, text=True, timeout=5, check=True,
        ).stdout
        self.assertEqual(version, "3.2", "macOS package CI must exercise system Bash 3.2")

        scripts = Path(__file__).resolve().parent
        root = scripts.parent
        capture_markers = (
            "# BEGIN tested Apollo inventory argument construction",
            "# END tested Apollo inventory argument construction",
        )
        command_markers = (
            "# BEGIN tested pre-sign package-inventory command",
            "# END tested pre-sign package-inventory command",
        )

        def source_block(path, markers):
            source = path.read_text()
            self.assertEqual(source.count(markers[0]), 1, str(path))
            self.assertEqual(source.count(markers[1]), 1, str(path))
            start = source.index(markers[0]) + len(markers[0])
            end = source.index(markers[1], start)
            return source[start:end]

        profile_bytes = b'{"fixture":"profile path contains spaces"}\n'
        for builder in ("build-dmg.sh", "build-pkg.sh"):
            builder_path = scripts / builder
            argument_block = source_block(builder_path, capture_markers)
            command_block = source_block(builder_path, command_markers)
            for capability in ("disabled", "enabled"):
                with self.subTest(builder=builder, capability=capability):
                    case_root = self.root / builder / capability
                    bundle = case_root / "owned bundle with spaces"
                    build_dir = case_root / "owned build bin"
                    project_root = case_root / "project fixture"
                    bundle.mkdir(parents=True)
                    build_dir.mkdir(parents=True)
                    project_root.mkdir(parents=True)
                    capture = case_root / "captured argv.txt"
                    fake_cli = build_dir / "sirsi"
                    fake_cli.write_text(
                        "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGV_CAPTURE\"\n"
                    )
                    fake_cli.chmod(0o755)

                    profile = ""
                    profile_path = None
                    if capability == "enabled":
                        profile_path = case_root / "private profile fixture.json"
                        profile_path.write_bytes(profile_bytes)
                        profile = str(profile_path)

                    harness = """set -euo pipefail
APOLLO_CAPABILITY="$1"
if [[ -n "$2" ]]; then
    PANTHEON_APOLLO_PROFILE="$2"
    export PANTHEON_APOLLO_PROFILE
else
    unset PANTHEON_APOLLO_PROFILE
fi
BUNDLE_DIR="$3"
APP_PATH="$3"
BUILD_DIR="$4"
PROJECT_ROOT="$5"
VERSION=fixture
FROZEN_APOLLO_DIGESTS="$6"
FROZEN_APOLLO_ARGS=(--expected-info-sha256 fixture --expected-apollo-profile-sha256 absent)
export ARGV_CAPTURE="$7"
""" + argument_block + "\n" + command_block
                    result = subprocess.run(
                        [
                            "/bin/bash", "-c", harness, "bash32-apollo-argv-fixture",
                            capability, profile, str(bundle), str(build_dir), str(project_root),
                            str(case_root / "owned digest receipt"), str(capture),
                        ],
                        env={"PATH": "/usr/bin:/bin", "HOME": str(self.root)},
                        capture_output=True, text=True, timeout=5, check=False,
                    )
                    self.assertEqual(result.returncode, 0, result.stderr)
                    argv = capture.read_text().splitlines()
                    index = argv.index("--apollo-capability")
                    boundary = "--write-frozen-digests" if builder == "build-dmg.sh" else "--expected-info-sha256"
                    end = argv.index(boundary, index)
                    expected = ["--apollo-capability", capability]
                    if capability == "enabled":
                        expected.extend(["--apollo-profile", profile])
                    self.assertEqual(argv[index:end], expected)

                    if builder == "build-dmg.sh" and profile_path is not None:
                        copied = bundle / "Contents/Resources/Apollo/native-profile.json"
                        self.assertEqual(copied.read_bytes(), profile_bytes)
                        self.assertEqual(stat.S_IMODE(copied.stat().st_mode), 0o644)

    def test_frozen_digest_receipt_reader_under_macos_bash_32(self):
        if sys.platform != "darwin":
            self.skipTest("the production regression target is macOS /bin/bash 3.2")
        version = subprocess.run(
            ["/bin/bash", "-c", 'printf "%s.%s" "${BASH_VERSINFO[0]}" "${BASH_VERSINFO[1]}"'],
            capture_output=True, text=True, timeout=5, check=True,
        ).stdout
        self.assertEqual(version, "3.2", "macOS package CI must exercise system Bash 3.2")

        scripts = Path(__file__).resolve().parent
        begin = "# BEGIN tested frozen Apollo digest receipt read"
        end = "# END tested frozen Apollo digest receipt read"

        def receipt_block(path):
            source = path.read_text()
            self.assertEqual(source.count(begin), 1, str(path))
            self.assertEqual(source.count(end), 1, str(path))
            start = source.index(begin) + len(begin)
            finish = source.index(end, start)
            return source[start:finish]

        digest = hashlib.sha256(b"inert frozen Info.plist fixture").hexdigest()
        profile = hashlib.sha256(b"inert Apollo profile fixture").hexdigest()
        valid_disabled = f"info_sha256={digest}\napollo_profile_sha256=absent\n"
        valid_enabled = f"info_sha256={digest}\napollo_profile_sha256={profile}\n"
        cases = (
            ("valid-disabled", valid_disabled, False, (digest, "absent")),
            ("valid-enabled", valid_enabled, False, (digest, profile)),
            ("invalid-digest", f"info_sha256={'z' * 64}\napollo_profile_sha256=absent\n", False, None),
            ("invalid-profile-digest", f"info_sha256={digest}\napollo_profile_sha256={'z' * 64}\n", False, None),
            ("missing-key", f"info_sha256={digest}\n", False, None),
            ("empty", "", False, None),
            ("duplicate-key", f"info_sha256={digest}\ninfo_sha256={digest}\napollo_profile_sha256=absent\n", False, None),
            ("extra-key", valid_disabled + "unexpected=value\n", False, None),
            ("absent-file", None, False, None),
            ("parser-failure", valid_disabled, True, None),
        )
        python3 = shutil.which("python3")
        self.assertIsNotNone(python3, "the receipt reader requires Python 3")
        parser_path = str(Path(python3).parent)
        harness = """set -euo pipefail
FROZEN_APOLLO_DIGESTS="$1"
"""
        blocks = {builder: receipt_block(scripts / builder) for builder in ("build-dmg.sh", "build-pkg.sh")}
        self.assertEqual(blocks["build-dmg.sh"], blocks["build-pkg.sh"], "paired builders must share receipt validation")
        for builder, block in blocks.items():
            for name, contents, fail_parser, expected in cases:
                with self.subTest(builder=builder, case=name):
                    case_root = self.root / "frozen receipt fixture" / builder / name
                    case_root.mkdir(parents=True)
                    receipt = case_root / "receipt.txt"
                    if contents is not None:
                        receipt.write_text(contents)
                    if fail_parser:
                        stub_bin = case_root / "stub-bin"
                        stub_bin.mkdir()
                        parser = stub_bin / "python3"
                        parser.write_text(f"#!/bin/sh\nprintf '%s\\n' '{digest} {profile}'\nexit 19\n")
                        parser.chmod(0o755)
                        search_path = os.pathsep.join((str(stub_bin), parser_path, "/usr/bin", "/bin"))
                    else:
                        search_path = os.pathsep.join((parser_path, "/usr/bin", "/bin"))
                    parsed = case_root / "parsed.txt"
                    continued = case_root / "continued.txt"
                    script = harness + block + "\nprintf '%s\\n%s\\n' \"$FROZEN_INFO_SHA256\" \"$FROZEN_PROFILE_SHA256\" > \"$2\"\n: > \"$3\"\n"
                    result = subprocess.run(
                        ["/bin/bash", "-c", script, "bash32-frozen-receipt-fixture", str(receipt), str(parsed), str(continued)],
                        env={"PATH": search_path},
                        stdin=subprocess.DEVNULL,
                        capture_output=True, text=True, timeout=5, check=False,
                    )
                    if expected is None:
                        self.assertNotEqual(result.returncode, 0, result.stderr)
                        self.assertFalse(continued.exists(), "receipt failure continued into release verification")
                        self.assertFalse(parsed.exists(), "receipt failure emitted digest fields")
                    else:
                        self.assertEqual(result.returncode, 0, result.stderr)
                        self.assertEqual(parsed.read_text().splitlines(), list(expected))
                        self.assertTrue(continued.exists(), "valid receipt did not continue to release verification")

    def test_supported_paired_builders_share_one_selected_receipt_without_pkg_recapture(self):
        root = Path(__file__).resolve().parent.parent
        makefile = (root / "Makefile").read_text()
        workflow = (root / ".github/workflows/release.yml").read_text()
        dmg = (root / "scripts/build-dmg.sh").read_text()
        pkg = (root / "scripts/build-pkg.sh").read_text()

        def target_block(name):
            lines = makefile.splitlines()
            start = next(i for i, line in enumerate(lines) if line == name + ":")
            block = [lines[start]]
            for line in lines[start + 1:]:
                if line and not line[0].isspace():
                    break
                block.append(line)
            return "\n".join(block)

        for name in ("dmg-dev", "release-dmg"):
            with self.subTest(target=name):
                block = target_block(name)
                self.assertEqual(block.count("new-apollo-digest-receipt.sh"), 1)
                self.assertEqual(block.count('PANTHEON_APOLLO_DIGEST_RECEIPT="$$receipt"'), 1)
                self.assertIn("scripts/build-dmg.sh", block)

        for name in ("pkg-dev", "release-pkg"):
            with self.subTest(target=name):
                block = target_block(name)
                self.assertEqual(block.count("new-apollo-digest-receipt.sh"), 1)
                self.assertEqual(block.count('PANTHEON_APOLLO_DIGEST_RECEIPT="$$receipt"'), 2)
                self.assertIn("scripts/build-dmg.sh", block)
                self.assertIn("scripts/build-pkg.sh", block)

        self.assertIn('PANTHEON_APOLLO_DIGEST_RECEIPT="$(bash scripts/new-apollo-digest-receipt.sh "$RUNNER_TEMP")"', workflow)
        self.assertIn("export PANTHEON_APOLLO_DIGEST_RECEIPT", workflow)
        self.assertLess(workflow.index("new-apollo-digest-receipt.sh"), workflow.index("scripts/build-dmg.sh --release"))
        self.assertLess(workflow.index("scripts/build-dmg.sh --release"), workflow.index("scripts/build-pkg.sh --release"))
        self.assertIn('--write-frozen-digests "$FROZEN_APOLLO_DIGESTS"', dmg)
        self.assertNotIn("--write-frozen-digests", pkg)
        self.assertNotIn('apollo-input-digests-${VERSION}-${ARCH}.txt', dmg + pkg)
        self.assertIn("PKG requires an existing regular pre-sign digest receipt", pkg)

    def test_supported_pkg_callers_preserve_absolute_path_and_receipt_under_macos_bash_32(self):
        if sys.platform != "darwin":
            self.skipTest("the production workflow regression target is macOS /bin/bash 3.2")
        version = subprocess.run(
            ["/bin/bash", "-c", 'printf "%s.%s" "${BASH_VERSINFO[0]}" "${BASH_VERSINFO[1]}"'],
            capture_output=True, text=True, timeout=5, check=True,
        ).stdout
        self.assertEqual(version, "3.2", "release workflow must exercise system Bash 3.2")

        workflow = (Path(__file__).resolve().parent.parent / ".github/workflows/release.yml").read_text()
        step_start = workflow.index("      - name: Build + sign + notarize DMG and PKG (arm64")
        run_marker = "        run: |\n"
        start = workflow.index(run_marker, step_start) + len(run_marker)
        end = workflow.index("\n      - name:", start)
        workflow_step = textwrap.dedent(workflow[start:end])
        payload_marker = "-- /bin/bash -euo pipefail -c '\n"
        payload_start = workflow_step.index(payload_marker) + len(payload_marker)
        payload_lines = []
        for line in workflow_step[payload_start:].splitlines():
            if line.strip() == "'":
                break
            payload_lines.append(line)
        else:
            self.fail("release workflow's inner Bash payload was not closed")
        payload = textwrap.dedent("\n".join(payload_lines))
        app_arg = '--app "$GITHUB_WORKSPACE/Pantheon.app"'
        self.assertEqual(payload.count(app_arg), 1)
        upload_start = workflow.index("      - name: Retain exact publication verification receipt (always)")
        upload_end = workflow.index("\n      - name:", upload_start)
        upload_step = workflow[upload_start:upload_end]
        self.assertIn("if: always()", upload_step)
        self.assertIn("path: |", upload_step)
        self.assertIn(
            "${{ runner.temp }}/pantheon-publication-${{ github.run_id }}-${{ github.run_attempt }}.json",
            upload_step,
        )
        self.assertIn("${{ env.PANTHEON_APOLLO_DIGEST_RECEIPT }}", upload_step)

        def run_fixture(case_name, body=None, fail_dmg_before_receipt=False, make_target=None):
            root = self.root / case_name
            workspace = root / "workspace"
            scripts = workspace / "scripts"
            runner_temp = root / "runner temp"
            github_env = root / "fixture GITHUB_ENV"
            app = workspace / "Pantheon.app"
            for path in (scripts, runner_temp, app):
                path.mkdir(parents=True, exist_ok=True)
            (workspace / "bin").mkdir()
            (workspace / "VERSION").write_text("0.24.151-fixture\n")
            github_env.touch()
            order = root / "order.log"

            (scripts / "new-apollo-digest-receipt.sh").write_text("""#!/bin/bash
set -euo pipefail
mkdir -p "$1"
receipt_root="$(cd "$1" && pwd -P)"
receipt_dir="$receipt_root/apollo-input-digests.fixture"
mkdir -p "$receipt_dir"
receipt="$receipt_dir/pre-sign-digests.txt"
printf 'receipt-maker\\n' >> "$ORDER_LOG"
printf '%s\\n' "$receipt"
""")
            (scripts / "ci-private-sdk.sh").write_text("""#!/bin/bash
set -euo pipefail
builder="$1"
shift
printf 'sdk:%s\\n' "$builder" >> "$ORDER_LOG"
"$builder" "$@"
""")
            (scripts / "build-dmg.sh").write_text("""#!/bin/bash
set -euo pipefail
printf 'dmg\\n' >> "$ORDER_LOG"
if [[ "${FIXTURE_FAIL_DMG_BEFORE_RECEIPT:-}" == true ]]; then exit 46; fi
if [[ "${FIXTURE_CHECK_GITHUB_ENV:-}" == true ]]; then
    grep -Fqx "PANTHEON_APOLLO_DIGEST_RECEIPT=$PANTHEON_APOLLO_DIGEST_RECEIPT" "$GITHUB_ENV" || exit 47
fi
[[ ! -e "$PANTHEON_APOLLO_DIGEST_RECEIPT" ]] || { echo 'receipt was not create-only' >&2; exit 44; }
( set -o noclobber; printf 'info_sha256=%s\\napollo_profile_sha256=absent\\n' aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa > "$PANTHEON_APOLLO_DIGEST_RECEIPT" )
printf '%s\\n' "$PANTHEON_APOLLO_DIGEST_RECEIPT" > "$FIXTURE_ROOT/dmg.receipt"
printf '%s\\0' "$@" > "$FIXTURE_ROOT/dmg.argv"
""")
            (scripts / "build-pkg.sh").write_text("""#!/bin/bash
set -euo pipefail
printf 'pkg\\n' >> "$ORDER_LOG"
printf '%s\\n' "$PANTHEON_APOLLO_DIGEST_RECEIPT" > "$FIXTURE_ROOT/pkg.receipt"
printf '%s\\0' "$@" > "$FIXTURE_ROOT/pkg.argv"
app=''
while [[ $# -gt 0 ]]; do
    case "$1" in
        --app) app="$2"; shift 2 ;;
        *) shift ;;
    esac
done
if [[ "$app" != /* ]]; then
    echo 'canonical input path must be absolute' >&2
    exit 41
fi
[[ -d "$app" ]] || { echo 'fixture app path is missing' >&2; exit 42; }
[[ -f "$PANTHEON_APOLLO_DIGEST_RECEIPT" && ! -L "$PANTHEON_APOLLO_DIGEST_RECEIPT" ]] || exit 45
if [[ "${FIXTURE_CHECK_GITHUB_ENV:-}" == true ]]; then
    grep -Fqx "PANTHEON_APOLLO_DIGEST_RECEIPT=$PANTHEON_APOLLO_DIGEST_RECEIPT" "$GITHUB_ENV" || exit 47
fi
""")
            for path in (
                scripts / "new-apollo-digest-receipt.sh",
                scripts / "ci-private-sdk.sh",
                scripts / "build-dmg.sh",
                scripts / "build-pkg.sh",
            ):
                path.chmod(0o755)

            env = {
                "PATH": "/usr/bin:/bin:/usr/sbin:/sbin",
                "GITHUB_WORKSPACE": str(workspace),
                "RUNNER_TEMP": str(runner_temp),
                "GITHUB_ENV": str(github_env),
                "BUILD_VERSION": "0.24.151-fixture",
                "PANTHEON_APOLLO_CAPABILITY": "disabled",
                "ORDER_LOG": str(order),
                "FIXTURE_ROOT": str(root),
            }
            if fail_dmg_before_receipt:
                env["FIXTURE_FAIL_DMG_BEFORE_RECEIPT"] = "true"
            if not make_target:
                env["FIXTURE_CHECK_GITHUB_ENV"] = "true"
            for name in ("HOME", "CODEX_HOME"):
                if name in os.environ:
                    env[name] = os.environ[name]
            if make_target:
                make = shutil.which("make", path=env["PATH"])
                self.assertIsNotNone(make, "Make paired-caller fixture requires make")
                command = [make, "-f", str(Path(__file__).resolve().parent.parent / "Makefile"), make_target]
            else:
                command = ["/bin/bash", "-euo", "pipefail", "-c", body]
            result = subprocess.run(
                command, cwd=workspace, env=env, stdin=subprocess.DEVNULL,
                capture_output=True, text=True, timeout=10, check=False,
            )
            return root, order, result

        root, order, result = run_fixture("workflow-absolute", payload)
        self.assertEqual(result.returncode, 0, result.stderr)
        receipt = str(root / "runner temp/apollo-input-digests.fixture/pre-sign-digests.txt")
        self.assertEqual(
            Path(receipt).read_text(),
            "info_sha256=" + "a" * 64 + "\napollo_profile_sha256=absent\n",
        )
        self.assertEqual(
            (root / "fixture GITHUB_ENV").read_text(),
            f"PANTHEON_APOLLO_DIGEST_RECEIPT={receipt}\n",
        )
        self.assertEqual((root / "dmg.receipt").read_text().strip(), receipt)
        self.assertEqual((root / "pkg.receipt").read_text().strip(), receipt)
        self.assertEqual(
            order.read_text().splitlines(),
            [
                "receipt-maker",
                "sdk:scripts/build-dmg.sh",
                "dmg",
                "sdk:scripts/build-pkg.sh",
                "pkg",
            ],
        )
        dmg_argv = (root / "dmg.argv").read_bytes().split(b"\0")[:-1]
        self.assertEqual(
            dmg_argv,
            [b"--release", b"--version", b"0.24.151-fixture", b"--arch", b"arm64"],
        )
        pkg_argv = (root / "pkg.argv").read_bytes().split(b"\0")[:-1]
        self.assertEqual(
            pkg_argv,
            [
                b"--release", b"--version", b"0.24.151-fixture", b"--arch", b"arm64",
                b"--app", str(root / "workspace/Pantheon.app").encode(),
            ],
        )

        legacy_body = payload.replace(app_arg, '--app "Pantheon.app"')
        self.assertNotEqual(legacy_body, payload)
        legacy_root, legacy_order, legacy = run_fixture("workflow-relative-control", legacy_body)
        self.assertEqual(legacy.returncode, 41, legacy.stderr)
        self.assertIn("canonical input path must be absolute", legacy.stderr)
        self.assertEqual(
            legacy_order.read_text().splitlines(),
            [
                "receipt-maker",
                "sdk:scripts/build-dmg.sh",
                "dmg",
                "sdk:scripts/build-pkg.sh",
                "pkg",
            ],
        )
        legacy_pkg_argv = (legacy_root / "pkg.argv").read_bytes().split(b"\0")[:-1]
        self.assertEqual(legacy_pkg_argv[-2:], [b"--app", b"Pantheon.app"])

        missing_root, missing_order, missing = run_fixture(
            "workflow-before-receipt", payload, fail_dmg_before_receipt=True,
        )
        self.assertEqual(missing.returncode, 46, missing.stderr)
        missing_path = str(missing_root / "runner temp/apollo-input-digests.fixture/pre-sign-digests.txt")
        self.assertEqual(
            (missing_root / "fixture GITHUB_ENV").read_text(),
            f"PANTHEON_APOLLO_DIGEST_RECEIPT={missing_path}\n",
        )
        self.assertFalse(Path(missing_path).exists(), "path recording must not fabricate the receipt")
        self.assertEqual(missing_order.read_text().splitlines(), ["receipt-maker", "sdk:scripts/build-dmg.sh", "dmg"])
        self.assertIn("if-no-files-found: warn", upload_step)

        for target, mode in (("pkg-dev", "--development"), ("release-pkg", "--release")):
            with self.subTest(make_target=target):
                make_root, make_order, make_result = run_fixture(
                    f"private make workspace with spaces {target}", make_target=target,
                )
                self.assertEqual(make_result.returncode, 0, make_result.stderr)
                make_receipt = str(
                    make_root / "workspace/bin/apollo-input-digests.fixture/pre-sign-digests.txt"
                )
                self.assertEqual(
                    Path(make_receipt).read_text(),
                    "info_sha256=" + "a" * 64 + "\napollo_profile_sha256=absent\n",
                )
                self.assertEqual((make_root / "dmg.receipt").read_text().strip(), make_receipt)
                self.assertEqual((make_root / "pkg.receipt").read_text().strip(), make_receipt)
                self.assertEqual(
                    make_order.read_text().splitlines(),
                    ["receipt-maker", "dmg", "pkg"],
                )
                make_dmg_argv = (make_root / "dmg.argv").read_bytes().split(b"\0")[:-1]
                self.assertEqual(
                    make_dmg_argv,
                    [mode.encode(), b"--version", b"0.24.151-fixture", b"--arch", os.uname().machine.encode()],
                )
                make_pkg_argv = (make_root / "pkg.argv").read_bytes().split(b"\0")[:-1]
                self.assertEqual(
                    make_pkg_argv,
                    [
                        mode.encode(), b"--version", b"0.24.151-fixture", b"--arch",
                        os.uname().machine.encode(), b"--app",
                        str(make_root / "workspace/Pantheon.app").encode(),
                    ],
                )

    def test_release_apollo_capability_requires_explicit_profile_pairing(self):
        scripts = Path(__file__).resolve().parent
        for name in ("build-dmg.sh", "build-pkg.sh"):
            source = (scripts / name).read_text()
            self.assertIn("release requires PANTHEON_APOLLO_CAPABILITY=enabled|disabled", source)
            self.assertIn("enabled Apollo capability requires an absolute regular non-symlink profile", source)
            self.assertIn("disabled Apollo capability forbids PANTHEON_APOLLO_PROFILE", source)
            self.assertIn("--apollo-capability", source)
            self.assertIn('elif [[ -n "${PANTHEON_APOLLO_PROFILE:-}" ]]', source)
        inventory = (scripts / "../internal/packageinventorycmd/verify.go").resolve().read_text()
        self.assertIn("Info.plist Apollo capability/profile mismatch", inventory)

    def test_release_profile_refusals_happen_before_any_signer_or_profile_read(self):
        scripts = Path(__file__).resolve().parent
        regular = self.root / "synthetic-profile.json"
        regular.write_text('{"fixture":true}')
        symlink = self.root / "profile-link.json"
        symlink.symlink_to(regular)
        cases = (
            ("missing capability", {}, "release requires PANTHEON_APOLLO_CAPABILITY"),
            ("missing enabled profile", {"PANTHEON_APOLLO_CAPABILITY": "enabled"}, "requires an absolute regular non-symlink profile"),
            ("symlinked enabled profile", {"PANTHEON_APOLLO_CAPABILITY": "enabled", "PANTHEON_APOLLO_PROFILE": str(symlink)}, "requires an absolute regular non-symlink profile"),
            ("disabled with supplied profile", {"PANTHEON_APOLLO_CAPABILITY": "disabled", "PANTHEON_APOLLO_PROFILE": str(regular)}, "disabled Apollo capability forbids"),
        )
        for script in ("build-dmg.sh", "build-pkg.sh"):
            for name, variables, expected in cases:
                with self.subTest(script=script, case=name):
                    env = {"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": str(self.root), "TMPDIR": str(self.root), **variables}
                    result = subprocess.run(
                        ["/bin/bash", str(scripts / script), "--release", "--version", "0.24.147-fixture"],
                        env=env, capture_output=True, text=True, timeout=10, check=False,
                    )
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(expected, result.stderr)
                    self.assertNotIn("codesign", result.stderr)
                    self.assertNotIn("pkgbuild", result.stderr)

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
