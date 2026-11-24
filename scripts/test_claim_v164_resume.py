import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('claim_v164', Path(__file__).with_name('claim-v164-resume.py'))
claim = importlib.util.module_from_spec(spec)
spec.loader.exec_module(claim)


class ClaimTests(unittest.TestCase):
    def setUp(self):
        self.controller = 'a' * 40
        self.env = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': claim.REPO,
                    'GITHUB_RUN_ID': '123', 'GITHUB_RUN_ATTEMPT': '1',
                    'GITHUB_REF': 'refs/heads/main', 'GITHUB_SHA': self.controller,
                    'GITHUB_EVENT_NAME': 'workflow_dispatch', 'GITHUB_JOB': 'recovery_successor',
                    'RUNNER_NAME': 'm1-pantheon',
                    'GITHUB_WORKFLOW_REF': claim.REPO + '/.github/workflows/' + claim.WORKFLOW + '@refs/heads/main'}
        self.posts = 0

    def api(self, endpoint, *args):
        if endpoint.endswith('/actions/runs/45270278775'):
            return {'head_sha': claim.COMMIT, 'head_branch': 'v0.24.164', 'event': 'push',
                    'status': 'completed', 'conclusion': 'failure', 'run_attempt': 1}
        if '/actions/workflows/' in endpoint:
            return {'total_count': 1, 'workflow_runs': [{'id': 123, 'head_sha': self.controller,
                    'head_branch': 'main', 'run_attempt': 1, 'status': 'in_progress'}]}
        if endpoint.endswith('/releases/456022306'):
            return {'id': 456022306, 'tag_name': 'v0.24.164', 'target_commitish': claim.COMMIT,
                    'draft': True, 'assets': []}
        if endpoint.endswith('/git/ref/tags/v0.24.164'):
            return {'object': {'type': 'tag', 'sha': 'tag'}}
        if endpoint.endswith('/git/tags/tag'):
            return {'object': {'type': 'commit', 'sha': claim.COMMIT}}
        if endpoint.endswith('/git/refs'):
            self.posts += 1
            return {'ref': claim.REF, 'object': {'sha': claim.COMMIT}}
        if '/git/ref/heads/' in endpoint:
            return {'ref': claim.REF, 'object': {'sha': claim.COMMIT}}
        raise AssertionError(endpoint)

    def run_claim(self, directory, api):
        with patch.dict(os.environ, self.env, clear=True), patch.object(claim, 'api', side_effect=api), \
                patch('sys.argv', ['claim', self.controller, directory]):
            claim.main()

    def test_one_claim_and_bound_reservation(self):
        with tempfile.TemporaryDirectory() as directory:
            self.run_claim(directory, self.api)
            reservation = json.loads((Path(directory) / 'reservation.json').read_text())
            self.assertEqual((reservation['status'], reservation['release_id'], reservation['commit']),
                             ('reserved', 456022306, claim.COMMIT))
            self.assertEqual(self.posts, 1)
            with self.assertRaises(FileExistsError):
                self.run_claim(directory, self.api)
            self.assertEqual(self.posts, 1)

    def test_ambiguous_create_stays_unknown_without_reservation(self):
        def fail_post(endpoint, *args):
            if endpoint.endswith('/git/refs'):
                self.posts += 1
                raise OSError('connection lost')
            return self.api(endpoint, *args)
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(OSError):
                self.run_claim(directory, fail_post)
            self.assertTrue((Path(directory) / 'claim-unknown.json').exists())
            self.assertFalse((Path(directory) / 'reservation.json').exists())
            self.assertEqual(self.posts, 1)

    def test_changed_draft_refuses_before_post(self):
        def changed(endpoint, *args):
            result = self.api(endpoint, *args)
            if endpoint.endswith('/releases/456022306'):
                result['assets'] = [{'name': 'unexpected'}]
            return result
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(RuntimeError):
                self.run_claim(directory, changed)
            self.assertEqual(self.posts, 0)

    def test_recovery_supervisor_refuses_fallback_or_wrong_controller(self):
        supervisor = Path(__file__).parent / 'maat/recovery-supervisor-v164.sh'
        for env in ({}, {'BUILD_VERSION': '0.24.164', 'PANTHEON_RELEASE_REF': 'refs/tags/v0.24.164',
                         'GITHUB_REF': 'refs/heads/unreviewed', 'GITHUB_EVENT_NAME': 'workflow_dispatch'}):
            result = subprocess.run(['/bin/bash', str(supervisor), 'verify'], env=env,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self.assertNotEqual(result.returncode, 0)


if __name__ == '__main__':
    unittest.main()
