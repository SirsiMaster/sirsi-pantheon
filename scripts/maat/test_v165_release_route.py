"""Synthetic source/shell controls only; no signing, publishing or service calls."""
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[2]
SELECTOR = ROOT / 'scripts/maat/release-supervisor.sh'


class V165ProtectedRoute(unittest.TestCase):
    def _assert_ci_contract(self, source):
        block = source.split('      - name: Protected native release admission fixtures', 1)[1].split('      - name:', 1)[0]
        self.assertNotIn('\n        if:', block)
        for filename in ('test_release_admission.py', 'test_v165_release_route.py'):
            self.assertIn('python3 -B -m unittest discover -s scripts/maat -p ' + filename, block)

    def test_ci_invokes_both_protected_route_suites_without_step_skip(self):
        self._assert_ci_contract((ROOT / '.github/workflows/ci.yml').read_text())

    def test_ci_contract_rejects_missing_suite_or_added_skip(self):
        source = (ROOT / '.github/workflows/ci.yml').read_text()
        marker = '      - name: Protected native release admission fixtures\n'
        for filename in ('test_release_admission.py', 'test_v165_release_route.py'):
            with self.subTest(missing=filename):
                command = '          python3 -B -m unittest discover -s scripts/maat -p ' + filename + '\n'
                self.assertIn(command, source)
                with self.assertRaises(AssertionError):
                    self._assert_ci_contract(source.replace(command, '', 1))
        with self.subTest(skip=True):
            self.assertIn(marker, source)
            with self.assertRaises(AssertionError):
                self._assert_ci_contract(source.replace(marker, marker + '        if: false\n', 1))

    def _assert_retry_contract(self, source):
        for name in ('Build + sign + notarize DMG and PKG (arm64 — Apple silicon)',
                     'Preserve exact first-attempt publication inputs'):
            block = source.split('      - name: ' + name, 1)[1].split('      - name:', 1)[0]
            conditions = [line.strip() for line in block.splitlines() if line.startswith('        if:')]
            self.assertEqual(conditions, ['if: github.run_attempt == 1'], name)
        for name in ('Restore exact original reservation on retry',
                     'Restore exact prior-attempt publication inputs'):
            block = source.split('      - name: ' + name, 1)[1].split('      - name:', 1)[0]
            conditions = [line.strip() for line in block.splitlines() if line.startswith('        if:')]
            self.assertEqual(conditions, ['if: github.run_attempt != 1'], name)

    def test_retry_restores_assets_without_rebuilding_or_selecting_signer(self):
        self._assert_retry_contract((ROOT / '.github/workflows/release.yml').read_text())

    def test_retry_contract_rejects_removing_the_build_attempt_guard(self):
        source = (ROOT / '.github/workflows/release.yml').read_text()
        marker = '      - name: Build + sign + notarize DMG and PKG (arm64 — Apple silicon)'
        block = source.split(marker, 1)[1].split('      - name:', 1)[0]
        changed = block.replace('        if: github.run_attempt == 1\n', '', 1)
        self.assertNotEqual(changed, block)
        mutant = source.replace(marker + block, marker + changed, 1)
        with self.assertRaises(AssertionError):
            self._assert_retry_contract(mutant)

    def test_actual_context_shell_selects_protected_spool_only_for_named_native_routes(self):
        source = (ROOT / '.github/workflows/release.yml').read_text()
        block = source.split('      - name: Retain qualified runner context', 1)[1].split('      - name:', 1)[0]
        shell = textwrap.dedent(block.split('        run: |\n', 1)[1])
        for version, protected in [('0.24.164', True), ('0.24.165', True), ('0.24.162', False), ('0.24.166', False)]:
            with self.subTest(version=version), tempfile.TemporaryDirectory() as directory:
                destination = Path(directory) / 'environment'
                result = subprocess.run(['/bin/bash', '-euo', 'pipefail', '-c', shell],
                    env={'PATH': '/usr/bin:/bin', 'GITHUB_REF': 'refs/tags/v' + version, 'GITHUB_ENV': str(destination),
                         'BOUND_RUNNER_NAME': 'synthetic-only', 'BOUND_RUNNER_OS': 'synthetic-only',
                         'BOUND_RUNNER_ARCH': 'synthetic-only'}, capture_output=True, text=True, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr)
                content = destination.read_text()
                self.assertEqual('SIRSI_ROUTER_URL=spool:///var/sirsipantheon/relay' in content, protected)
                self.assertEqual('SIRSI_RELAY_TRUST_GROUP=_sirsipantheon' in content, protected)

    def test_v165_unenrolled_context_cannot_use_legacy_supervisor(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            legacy = root / '.local/bin/sirsi-maat-run'
            legacy.parent.mkdir(parents=True)
            legacy.write_text('#!/bin/bash\nprintf LEGACY_EXECUTED\nexit 63\n')
            legacy.chmod(0o700)
            reviewed = root / 'scripts/maat/admitted-run.py'
            reviewed.parent.mkdir(parents=True)
            reviewed.write_bytes(legacy.read_bytes())
            for ref, version, expected in [
                ('refs/tags/v0.24.165', '0.24.165', 'qualified runner context required'),
                ('refs/tags/v0.24.165', '0.24.164', 'tag/version disagree'),
                ('refs/tags/v0.24.164', '0.24.165', 'tag/version disagree'),
                ('refs/heads/v0.24.165', '0.24.165', 'tag/version disagree'),
                ('refs/tags/v0.24.165', '0.24.165-extra', 'tag/version disagree'),
            ]:
                with self.subTest(ref=ref, version=version):
                    environment = {'PATH': '/usr/bin:/bin', 'HOME': directory, 'GITHUB_REF': ref, 'BUILD_VERSION': version}
                    result = subprocess.run(['/bin/bash', str(SELECTOR), 'run', '--unused'],
                        cwd=root, env=environment, capture_output=True, text=True, timeout=5)
                    self.assertEqual(result.returncode, 1, result.stderr)
                    self.assertIn(expected, result.stderr)
                    self.assertNotIn('LEGACY_EXECUTED', result.stdout)

    def test_older_legacy_route_remains_distinct(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            legacy = root / '.local/bin/sirsi-maat-run'
            legacy.parent.mkdir(parents=True)
            legacy.write_text('#!/bin/bash\nprintf LEGACY_CONTROL\nexit 63\n')
            legacy.chmod(0o700)
            reviewed = root / 'scripts/maat/admitted-run.py'
            reviewed.parent.mkdir(parents=True)
            reviewed.write_bytes(legacy.read_bytes())
            result = subprocess.run(['/bin/bash', str(SELECTOR), 'run', '--unused'], cwd=root,
                env={'PATH': '/usr/bin:/bin', 'HOME': directory, 'GITHUB_REF': 'refs/tags/v0.24.162', 'BUILD_VERSION': '0.24.162'},
                capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 63)
            self.assertEqual(result.stdout, 'LEGACY_CONTROL')

    def test_workflow_has_exact_v165_native_only_conditions(self):
        source = (ROOT / '.github/workflows/release.yml').read_text()
        positive = "github.ref == 'refs/tags/v0.24.165'"
        negative = "github.ref != 'refs/tags/v0.24.165'"
        for name in ('Verify v164 release supervisor before reserving a draft',
                     'Verify v164 signer before reserving a draft',
                     'Select v164 delegated signing client'):
            with self.subTest(step=name):
                block = source.split('      - name: ' + name, 1)[1].split('      - name:', 1)[0]
                self.assertIn(positive, block)
                self.assertIn('github.run_attempt == 1', block)
        for name in ('Import Developer ID signing identities',
                     'Resolve enrolled or imported Developer ID identity',
                     'Preflight imported Developer ID Installer identity'):
            with self.subTest(step=name):
                block = source.split('      - name: ' + name, 1)[1].split('      - name:', 1)[0]
                self.assertIn(negative, block)
        build = source.split('      - name: Build + sign + notarize', 1)[1].split('      - name:', 1)[0]
        for name in ('DEVELOPER_ID_INSTALLER:', 'APPLE_ID:', 'APPLE_TEAM_ID:', 'APPLE_APP_PASSWORD:'):
            line = next(line for line in build.splitlines() if line.strip().startswith(name))
            self.assertIn(negative, line)
        npm = source.split('\n  publish-npm:\n', 1)[1]
        conditions = [line.strip() for line in npm.splitlines() if line.startswith('    if:')]
        self.assertEqual(conditions, ["if: ${{ github.ref != 'refs/tags/v0.24.164' && github.ref != 'refs/tags/v0.24.165' }}"])
        # Reuse the exact enrolled pair, not a newly fabricated runtime or relaxed pin.
        selector = SELECTOR.read_text()
        self.assertIn('runtime=/usr/local/libexec/sirsi-pantheon/release-runtimes/v0.24.164', selector)
        self.assertNotIn('runtime=/usr/local/libexec/sirsi-pantheon/release-runtimes/v0.24.165', selector)


if __name__ == '__main__':
    unittest.main()
