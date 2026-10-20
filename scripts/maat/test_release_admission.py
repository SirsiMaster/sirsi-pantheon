"""Source-contract tests only: never compile, sign, notarize or contact a router."""
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]


class ReleaseAdmissionContract(unittest.TestCase):
    def test_both_native_payloads_are_inside_one_maintained_supervisor(self):
        source = (ROOT / '.github/workflows/release.yml').read_text()
        block = source.split('      - name: Build + sign + notarize', 1)[1].split('      - name:', 1)[0]
        start = block.index('python3 scripts/maat/admitted-run.py')
        self.assertIn('--agent codex-pantheon', block[start:])
        self.assertIn('--register-executor', block[start:])
        self.assertIn('--cpu-cores 1 --timeout 1500 -- /bin/bash -euo pipefail -c', block[start:])
        payload = block[start:].split("-c '\n", 1)[1].split("\n            '", 1)[0]
        for script in ('build-dmg.sh', 'build-pkg.sh'):
            self.assertEqual(block.count('bash scripts/ci-private-sdk.sh scripts/' + script), 1)
            self.assertIn('bash scripts/ci-private-sdk.sh scripts/' + script, payload)
        for mask in ('GOMAXPROCS: "1"', 'GOFLAGS: "-p=1"', 'PANTHEON_BUILD_JOBS: "1"'):
            self.assertIn(mask, block)

    def test_swift_uses_validated_explicit_job_mask(self):
        source = (ROOT / 'scripts/build-dmg.sh').read_text()
        self.assertIn('SWIFT_BUILD_ARGS+=(--jobs "${PANTHEON_BUILD_JOBS}")', source)
        self.assertIn('swift build "${SWIFT_BUILD_ARGS[@]}"', source)
        self.assertIn('PANTHEON_BUILD_JOBS must be an integer from 1 to 64', source)

    def test_workflow_contract_rejects_missing_or_unbounded_supervision(self):
        source = (ROOT / '.github/workflows/release.yml').read_text()
        mutations = [source.replace('--register-executor', '--no-executor'),
                     source.replace('--cpu-cores 1', '--cpu-cores 8'),
                     source.replace('PANTHEON_BUILD_JOBS: "1"', 'PANTHEON_BUILD_JOBS: "8"'),
                     source.replace('python3 scripts/maat/admitted-run.py', 'python3 ignored.py')]
        for candidate in mutations:
            with self.subTest(candidate=candidate), patch.object(Path, 'read_text', return_value=candidate):
                with self.assertRaises((AssertionError, ValueError)):
                    self.test_both_native_payloads_are_inside_one_maintained_supervisor()


if __name__ == '__main__':
    unittest.main()
