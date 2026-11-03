import unittest
import json
import subprocess
from pathlib import Path


class RecoveryWorkflowContractTests(unittest.TestCase):
    def test_exact_one_shot_workflow_orders_custody_before_publish(self):
        workflow = (Path(__file__).parent.parent / '.github/workflows/recover-v163-release.yml').read_text()
        for required in ('pantheon-release-refs/tags/v0.24.163',
                         'a5967913c68c03be435ab009f24df01e908681b3',
                         '--run-id 41592161041 --release-id 432359892',
                         '--recovery-run-id "$GITHUB_RUN_ID"',
                         'github.run_attempt == 1',
                         'reviewed_controller_sha',
                         'scripts/build-release-pretoolguard.sh',
                         'scripts/build-dmg.sh', 'scripts/build-pkg.sh',
                         'scripts/release-custody.py', 'scripts/publish-release-assets.py',
                         'PANTHEON_GUARD_ATTEMPT }}/signer-jobs/*.json',
                         'pantheon-guard-attach-${{ github.run_id }}-${{ github.run_attempt }}-*.plist.stderr',
                         'cask-release verify'):
            self.assertIn(required, workflow)
        ordered = ('claim-release-recovery.py', 'Build and sign exact original source once',
                   'Upload exact signed custody before publication',
                   'Restore and verify exact signed inputs', 'Publish only the bound draft',
                   'Download exact published DMG', 'Render, publish and verify canonical cask')
        positions = [workflow.index(text) for text in ordered]
        self.assertEqual(positions, sorted(positions))
        self.assertNotIn('gh release create', workflow)
        self.assertNotIn('git push', workflow)

    def test_cask_receipt_selects_digest_from_actual_publisher_shape(self):
        workflow = (Path(__file__).parent.parent / '.github/workflows/recover-v163-release.yml').read_text()
        selector = '.assets["SirsiPantheon-0.24.163-arm64.dmg"].sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))'
        self.assertIn(selector, workflow)
        receipt = {'assets': {'SirsiPantheon-0.24.163-arm64.dmg': {'sha256': 'a' * 64, 'bytes': 123}}}
        result = subprocess.run(['jq', '-er', selector], input=json.dumps(receipt), text=True,
                                capture_output=True, check=True)
        self.assertEqual(result.stdout.strip(), 'a' * 64)

    def test_guard_globs_require_allocated_attempt_and_signing_sidecars_are_exact(self):
        workflow = (Path(__file__).parent.parent / '.github/workflows/recover-v163-release.yml').read_text()
        guard_step = workflow.split('      - name: Retain guard attempt evidence when allocated\n', 1)[1]
        guard_step = guard_step.split('      - name:', 1)[0]
        self.assertIn("if: always() && env.PANTHEON_GUARD_ATTEMPT != ''", guard_step)
        guard_lines = [line for line in workflow.splitlines() if 'PANTHEON_GUARD_ATTEMPT }}/' in line]
        self.assertTrue(guard_lines)
        self.assertTrue(all(line in guard_step for line in guard_lines))
        for sidecar in ('source/signed-Pantheon.app.receipt.txt',
                        'source/signed-.receipt.txt',
                        'source/bin/signed-SirsiPantheon-0.24.163-arm64.dmg.receipt.txt',
                        'source/bin/signed-SirsiPantheon-0.24.163-arm64.pkg.receipt.txt',
                        'source/bin/signed-.receipt.txt'):
            self.assertIn(sidecar, workflow)
        sidecar_step = workflow.split('      - name: Retain exact app, DMG and PKG signing sidecars\n', 1)[1]
        sidecar_step = sidecar_step.split('      - name:', 1)[0]
        self.assertIn("if: always() && env.PANTHEON_GUARD_ATTEMPT != ''", sidecar_step)
        self.assertIn('Refuse preexisting signing sidecars', workflow)
        self.assertLess(workflow.index('Refuse preexisting signing sidecars'), workflow.index('Build and sign exact original source once'))


if __name__ == '__main__':
    unittest.main()
