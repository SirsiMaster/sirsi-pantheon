import importlib.util
import json
import os
from pathlib import Path
import plistlib
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('publication_claim', Path(__file__).with_name('claim-v164-publication-resume.py'))
claim = importlib.util.module_from_spec(spec)
spec.loader.exec_module(claim)


class PublicationClaimTests(unittest.TestCase):
    def setUp(self):
        self.controller = 'a' * 40
        self.env = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': claim.base.REPO,
                    'GITHUB_RUN_ID': '123', 'GITHUB_RUN_ATTEMPT': '1', 'GITHUB_REF': 'refs/heads/main',
                    'GITHUB_SHA': self.controller, 'GITHUB_EVENT_NAME': 'workflow_dispatch',
                    'GITHUB_JOB': 'publication_successor', 'RUNNER_NAME': 'm1-pantheon',
                    'GITHUB_WORKFLOW_REF': claim.base.REPO + '/.github/workflows/' + claim.WORKFLOW + '@refs/heads/main'}
        self.posts = 0

    def api(self, endpoint, *args):
        if claim.WORKFLOW + '/runs?' in endpoint:
            return {'total_count': 2, 'workflow_runs': [
                {'id': 45291181859, 'head_sha': 'f630401e1b3a1cb1ed679422f7dcf5b332edb4be',
                 'run_attempt': 1, 'status': 'completed', 'conclusion': 'failure'},
                {'id': 123, 'head_sha': self.controller, 'run_attempt': 1, 'status': 'in_progress', 'conclusion': None}]}
        if endpoint.endswith('/actions/runs/45291181859/jobs?per_page=100'):
            return {'jobs': [{'id': 138100055662, 'conclusion': 'failure', 'steps': [
                {'name': 'Claim one publication-only continuation', 'conclusion': 'failure'},
                {'name': 'Publish only the existing draft', 'conclusion': 'skipped'}]}]}
        if '/git/matching-refs/heads/pantheon-recovery/v0.24.164-456022306' in endpoint:
            return [{'ref': 'refs/heads/pantheon-recovery/v0.24.164-456022306-after-45270278775'},
                    {'ref': 'refs/heads/pantheon-recovery/v0.24.164-456022306-after-45278099354'}]
        histories = {
            'release.yml/runs?': (45270278775, claim.base.COMMIT),
            'recover-v164-after-guard.yml/runs?': (45278099354, '371057ee20cbb5f2f80a8bef43766a4986f741c0'),
            'recover-v164-after-metadata.yml/runs?': (45284797296, '3fe7dde3526a1e905f7b5aefd00af541a37a1941'),
        }
        for key, (run, sha) in histories.items():
            if key in endpoint:
                return {'total_count': 1, 'workflow_runs': [{'id': run, 'head_sha': sha, 'run_attempt': 1,
                    'status': 'in_progress' if run == 123 else 'completed', 'conclusion': None if run == 123 else 'failure'}]}
        if endpoint.endswith('/actions/runs/45284797296/jobs?per_page=100'):
            return {'jobs': [{'id': 138078240323, 'conclusion': 'failure', 'steps': [
                {'name': 'Claim exactly one recovery', 'conclusion': 'success'},
                {'name': 'Build and sign remaining artifacts', 'conclusion': 'success'},
                {'name': 'Restore and verify exact signed inputs', 'conclusion': 'success'},
                {'name': 'Publish only the existing draft', 'conclusion': 'failure'}]}]}
        if endpoint.endswith('/actions/artifacts/14872220529'):
            return {'id': 14872220529, 'name': 'v164-metadata-custody-45284797296', 'expired': False,
                    'digest': 'sha256:790a270ed85bac61524ec3ff8fedd2b92c183f2db6e2b7bf8c5c8b31be2e0bd3', 'workflow_run': {'id': 45284797296}}
        if endpoint.endswith('/actions/artifacts/14872310501'):
            return {'id': 14872310501, 'name': 'v164-metadata-successor-45284797296-1', 'expired': False,
                    'digest': 'sha256:23ea294fac9c404903ee47c15ed7eacebbef334fc01de5c7f97915ef4a499f14', 'workflow_run': {'id': 45284797296}}
        if endpoint.endswith('/releases/456022306'):
            return {'id': 456022306, 'tag_name': 'v0.24.164', 'target_commitish': claim.base.COMMIT, 'draft': True, 'assets': []}
        if endpoint.endswith('/git/ref/tags/v0.24.164'):
            return {'object': {'type': 'tag', 'sha': 'c2e8a1bfed5a1a3cae7ba208b953e4aa89656bf7'}}
        if endpoint.endswith('/git/tags/c2e8a1bfed5a1a3cae7ba208b953e4aa89656bf7'):
            return {'object': {'sha': claim.base.COMMIT}}
        if endpoint.endswith('/git/refs'):
            self.posts += 1
            return {'ref': claim.REF, 'object': {'sha': claim.base.COMMIT}}
        if '/git/ref/heads/' in endpoint:
            return {'ref': 'refs/heads/' + endpoint.split('/git/ref/heads/', 1)[1], 'object': {'sha': claim.base.COMMIT}}
        raise AssertionError(endpoint)

    def run_claim(self, directory, api, mount=True):
        cask = Path(directory) / 'input-cask.rb'
        cask.write_text('fixture')
        output = Path(directory) / 'claim'
        output.mkdir()
        with patch.dict(os.environ, self.env, clear=True), patch.object(claim.base, 'api', side_effect=api), \
                patch.object(claim, 'reconcile_evidence', return_value=cask), \
                patch.object(claim, 'old_mount_absent', return_value=mount), \
                patch('sys.argv', ['claim', self.controller, directory, directory, str(output)]):
            claim.main()
        return output

    def test_one_claim_binds_existing_draft(self):
        with tempfile.TemporaryDirectory() as directory:
            out = self.run_claim(directory, self.api)
            self.assertEqual(json.loads((out / 'reservation.json').read_text())['release_id'], 456022306)
            self.assertEqual(self.posts, 1)

    def test_prior_upload_or_second_release_refuses_before_claim(self):
        def changed(endpoint, *args):
            data = self.api(endpoint, *args)
            if endpoint.endswith('/releases/456022306'):
                data['assets'] = [{'name': 'already-uploaded'}]
            return data
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(RuntimeError):
                self.run_claim(directory, changed)
            self.assertEqual(self.posts, 0)

    def test_changed_custody_digest_refuses_before_claim(self):
        def changed(endpoint, *args):
            data = self.api(endpoint, *args)
            if endpoint.endswith('/actions/artifacts/14872220529'):
                data['digest'] = 'sha256:' + '0' * 64
            return data
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(RuntimeError):
                self.run_claim(directory, changed)
            self.assertEqual(self.posts, 0)

    def test_uncertain_claim_has_no_reservation(self):
        def fail_post(endpoint, *args):
            if endpoint.endswith('/git/refs'):
                self.posts += 1
                raise OSError('connection lost')
            return self.api(endpoint, *args)
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(OSError):
                self.run_claim(directory, fail_post)
            self.assertTrue((Path(directory) / 'claim/claim-unknown.json').exists())
            self.assertFalse((Path(directory) / 'claim/reservation.json').exists())

    def test_mount_reappearing_after_claim_blocks_reservation(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(RuntimeError):
                self.run_claim(directory, self.api, mount=False)
            self.assertEqual(self.posts, 1)
            self.assertFalse((Path(directory) / 'claim/reservation.json').exists())

    def test_prior_upload_receipt_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            custody, evidence = root / 'custody', root / 'evidence'
            custody.mkdir(); evidence.mkdir()
            (custody / 'custody.json').write_text(json.dumps({'commit': claim.base.COMMIT, 'version': '0.24.164', 'sha256': claim.HASHES}))
            for name in claim.HASHES:
                (custody / name).touch()
            receipt = {'status': 'failed', 'tag': 'v0.24.164', 'commit': claim.base.COMMIT, 'publish_attempted': False,
                       'uploads_attempted': ['unexpected'], 'error': 'guard image mount/device association unknown; inspect /var/folders/qs/',
                       'assets': {name: {'sha256': value} for name, value in claim.HASHES.items() if name.endswith(('.dmg', '.pkg'))}}
            (evidence / 'publication.json').write_text(json.dumps(receipt))
            with patch.object(claim, 'digest', side_effect=lambda p: claim.HASHES[p.name]), \
                    patch.object(claim, 'old_mount_absent', return_value=True):
                with self.assertRaises(RuntimeError):
                    claim.reconcile_evidence(custody, evidence)

    def test_reconciles_complete_prior_evidence_and_refuses_active_mount(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            custody, evidence = root / 'custody', root / 'evidence'
            custody.mkdir(); evidence.mkdir()
            (custody / 'custody.json').write_text(json.dumps({'commit': claim.base.COMMIT, 'version': '0.24.164', 'sha256': claim.HASHES}))
            for name in claim.HASHES:
                (custody / name).touch()
            receipt = {'status': 'failed', 'tag': 'v0.24.164', 'commit': claim.base.COMMIT, 'uploads_attempted': [], 'publish_attempted': False,
                       'error': 'guard image mount/device association unknown; inspect /var/folders/qs/',
                       'assets': {name: {'sha256': value} for name, value in claim.HASHES.items() if name.endswith(('.dmg', '.pkg'))}}
            (evidence / 'publication.json').write_text(json.dumps(receipt))
            (evidence / 'claim-observed.json').write_text(json.dumps({'status': 'claim_observed', 'successor_run': 45284797296,
                'release_id': 456022306, 'commit': claim.base.COMMIT, 'controller_sha': '3fe7dde3526a1e905f7b5aefd00af541a37a1941'}))
            plist = evidence / 'pantheon-guard-attach-45284797296-1-6rz9atxm.plist'
            plist.write_bytes(plistlib.dumps({'system-entities': [{'dev-entry': '/dev/disk10', 'mount-point': claim.OLD_MOUNT}]}))
            cask = evidence / 'sirsi-pantheon.rb'; cask.touch()
            fake_hashes = dict(claim.HASHES, **{plist.name: '0530d21d7c2c6b42b2f6088711cc5f2838e13d436d93bfe2517a6c474fccb3a7',
                                               cask.name: '0e8c7665e0bd09443ddbf924d70cc84cb92ce4f23bdade30b387a5e8e1a063e2'})
            with patch.object(claim, 'digest', side_effect=lambda p: fake_hashes[p.name]), \
                    patch.object(claim, 'old_mount_absent', return_value=True):
                self.assertEqual(claim.reconcile_evidence(custody, evidence), cask)
                receipt['publish_attempted'] = True
                (evidence / 'publication.json').write_text(json.dumps(receipt))
                with self.assertRaises(RuntimeError):
                    claim.reconcile_evidence(custody, evidence)
                receipt['publish_attempted'] = False
                (evidence / 'publication.json').write_text(json.dumps(receipt))
            with patch.object(claim, 'digest', side_effect=lambda p: fake_hashes[p.name]), \
                    patch.object(claim, 'old_mount_absent', return_value=False):
                with self.assertRaises(RuntimeError):
                    claim.reconcile_evidence(custody, evidence)


if __name__ == '__main__':
    unittest.main()
