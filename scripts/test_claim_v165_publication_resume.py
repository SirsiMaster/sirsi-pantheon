import importlib.util
import json
import os
from pathlib import Path
import plistlib
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('claim', Path(__file__).with_name('claim-v165-publication-resume.py'))
claim = importlib.util.module_from_spec(spec)
spec.loader.exec_module(claim)


class ClaimTest(unittest.TestCase):
    def setUp(self):
        self.controller = 'a' * 40
        self.posts = 0
        self.env = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': claim.base.REPO,
                    'GITHUB_RUN_ATTEMPT': '1', 'GITHUB_RUN_ID': '123', 'GITHUB_REF': 'refs/heads/main',
                    'GITHUB_SHA': self.controller, 'GITHUB_EVENT_NAME': 'workflow_dispatch',
                    'GITHUB_JOB': 'publication_successor', 'RUNNER_NAME': 'm1-pantheon',
                    'GITHUB_WORKFLOW_REF': claim.base.REPO + '/.github/workflows/' + claim.WORKFLOW + '@refs/heads/main'}

    def api(self, endpoint, *args):
        if '/actions/workflows/release.yml/runs?' in endpoint:
            return {'total_count': 1, 'workflow_runs': [{'id': 45427969009, 'head_sha': claim.COMMIT,
                'head_branch': 'v0.24.165', 'status': 'completed', 'conclusion': 'failure', 'run_attempt': 1}]}
        if '/actions/workflows/' + claim.WORKFLOW + '/runs?' in endpoint:
            return {'total_count': 1, 'workflow_runs': [{'id': 123, 'head_sha': self.controller,
                'head_branch': 'main', 'status': 'in_progress', 'run_attempt': 1}]}
        if '/git/matching-refs/' in endpoint:
            return []
        if '/actions/artifacts/' in endpoint:
            ident = int(endpoint.rsplit('/', 1)[1]); name, sha = claim.ARTIFACTS[ident]
            return {'id': ident, 'name': name, 'digest': sha, 'expired': False, 'workflow_run': {'id': 45427969009}}
        if endpoint.endswith('/releases/456974029'):
            return {'id': 456974029, 'tag_name': 'v0.24.165', 'target_commitish': claim.COMMIT, 'draft': True, 'assets': []}
        if endpoint.endswith('/git/ref/tags/v0.24.165'):
            return {'object': {'type': 'tag', 'sha': claim.TAG_OBJECT}}
        if endpoint.endswith('/git/tags/' + claim.TAG_OBJECT):
            return {'object': {'type': 'commit', 'sha': claim.COMMIT, 'url': 'https://api.github.com/repos/SirsiMaster/sirsi-pantheon/git/commits/' + claim.COMMIT}}
        if endpoint.endswith('/git/refs'):
            self.posts += 1
            return {'ref': claim.REF, 'object': {'sha': claim.COMMIT}}
        if '/git/ref/heads/' in endpoint:
            return {'ref': claim.REF, 'object': {'sha': claim.COMMIT}}
        raise AssertionError(endpoint)

    def invoke(self, out, api=None, mount=True):
        with patch.dict(os.environ, self.env, clear=True), patch.object(claim.base, 'api', side_effect=api or self.api), \
             patch.object(claim, 'reconcile'), patch.object(claim, 'old_mount_absent', side_effect=mount if isinstance(mount, list) else None, return_value=mount), \
             patch('sys.argv', ['claim', self.controller, 'custody', 'evidence', 'reservation', str(out)]):
            claim.main()

    def test_single_claim_and_reservation(self):
        with tempfile.TemporaryDirectory() as d:
            self.invoke(Path(d))
            self.assertEqual(self.posts, 1)
            self.assertEqual(json.loads((Path(d)/'reservation.json').read_text())['release_id'], 456974029)

    def test_refusal_before_create(self):
        for mode in ('assets', 'claim', 'artifact', 'two_runs', 'mount'):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as d:
                self.posts = 0
                def altered(endpoint, *args):
                    data = self.api(endpoint, *args)
                    if mode == 'assets' and '/releases/' in endpoint: data['assets'] = [{'id': 1}]
                    if mode == 'claim' and '/git/matching-refs/' in endpoint: return [{'ref': claim.REF}]
                    if mode == 'artifact' and '/actions/artifacts/' in endpoint: data['digest'] = 'wrong'
                    if mode == 'two_runs' and claim.WORKFLOW + '/runs?' in endpoint: data['total_count'] = 2
                    return data
                with self.assertRaises(RuntimeError): self.invoke(Path(d), altered, mode != 'mount')
                self.assertEqual(self.posts, 0)
                self.assertFalse((Path(d)/'reservation.json').exists())

    def test_unknown_create_and_postclaim_mount_do_not_authorize_publishing(self):
        for mode in ('lost_response', 'mount_reappears'):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as d:
                self.posts = 0
                def api(endpoint, *args):
                    if mode == 'lost_response' and endpoint.endswith('/git/refs'):
                        self.posts += 1
                        raise OSError('connection lost after create')
                    return self.api(endpoint, *args)
                with self.assertRaises((OSError, RuntimeError)):
                    self.invoke(Path(d), api, [True, False] if mode == 'mount_reappears' else True)
                self.assertEqual(self.posts, 1)
                self.assertTrue((Path(d)/'claim-unknown.json').exists())
                self.assertFalse((Path(d)/'reservation.json').exists())

    def test_listing_checks_old_device_and_both_mount_spellings(self):
        for entities, expected in [([], True), ([{'dev-entry':'/dev/disk10'}], False),
                ([{'dev-entry':'/dev/disk10s2'}], False), ([{'mount-point':claim.OLD_MOUNT}], False),
                ([{'mount-point':claim.OLD_MOUNT.removeprefix('/private')}], False)]:
            with self.subTest(entities=entities), patch.object(claim.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, plistlib.dumps({'images':[{'system-entities':entities}]}))), \
                 patch.object(claim.os.path, 'ismount', return_value=False):
                self.assertEqual(claim.old_mount_absent(), expected)
        with patch.object(claim.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, plistlib.dumps({'images':[{}]}))):
            with self.assertRaises(RuntimeError): claim.old_mount_absent()

    def test_competing_create_has_only_one_winner(self):
        claimed = False
        def atomic_api(endpoint, *args):
            nonlocal claimed
            if endpoint.endswith('/git/refs'):
                if claimed:
                    raise RuntimeError('already exists')
                claimed = True
            return self.api(endpoint, *args)
        with tempfile.TemporaryDirectory() as a, tempfile.TemporaryDirectory() as b:
            self.invoke(Path(a), atomic_api)
            with self.assertRaises(RuntimeError): self.invoke(Path(b), atomic_api)
            self.assertTrue((Path(a)/'reservation.json').exists())
            self.assertTrue((Path(b)/'claim-unknown.json').exists())
            self.assertFalse((Path(b)/'reservation.json').exists())

    def test_real_retained_evidence_and_altered_receipt(self):
        import shutil
        root = Path('/private/tmp/pantheon-v165-retained-custody-review-20261125')
        evidence = Path('/private/tmp/pantheon-v165-pub-evidence-45427969009')
        reservation = Path('/private/tmp/pantheon-v165-reservation-review-20261125')
        if not all(p.is_dir() for p in (root, evidence, reservation)):
            self.skipTest('retained evidence not available on this host')
        claim.reconcile(root, evidence, reservation)
        with tempfile.TemporaryDirectory() as d:
            copy = Path(d)
            for name in (claim.PLIST, 'pantheon-publication-45427969009-1.json'):
                shutil.copy(evidence/name, copy/name)
            receipt_path = copy/'pantheon-publication-45427969009-1.json'
            receipt = json.loads(receipt_path.read_text())
            receipt['uploads_attempted'] = ['unknown']
            receipt_path.write_text(json.dumps(receipt))
            with self.assertRaises(RuntimeError): claim.reconcile(root, copy, reservation)

    def test_workflow_uses_controller_and_no_signer(self):
        text = (Path(__file__).resolve().parents[1]/'.github/workflows/recover-v165-publication-only.yml').read_text()
        self.assertIn('$GITHUB_WORKSPACE/controller/scripts/publish-release-assets.py', text)
        self.assertNotIn('sign-pretoolguard', text)
        self.assertNotIn('build-dmg', text)
        self.assertNotIn('HOMEBREW_TAP_TOKEN', text)
        self.assertLess(text.index('Verify retained evidence before extraction'), text.index('Allocate private output'))
        self.assertLess(text.index('Claim one publication-only continuation'), text.index('Publish only the existing draft'))


if __name__ == '__main__':
    unittest.main()
