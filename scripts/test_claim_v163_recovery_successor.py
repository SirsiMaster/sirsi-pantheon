import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

spec = importlib.util.spec_from_file_location('successor', Path(__file__).with_name('claim-v163-recovery-successor.py'))
successor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(successor)


class ClaimTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.receipt = Path(self.tmp.name) / 'claim.json'
        self.controller = 'a' * 40
        self.entry = {'id': 10, 'run_attempt': 1, 'head_branch': 'main',
                      'head_sha': self.controller, 'event': 'workflow_dispatch', 'status': 'in_progress'}
        context = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': successor.consumed.original.REPO,
                   'GITHUB_RUN_ID': '10', 'GITHUB_RUN_ATTEMPT': '1', 'GITHUB_REF': 'refs/heads/main',
                   'GITHUB_SHA': self.controller, 'GITHUB_EVENT_NAME': 'workflow_dispatch',
                   'GITHUB_WORKFLOW_REF': successor.consumed.original.REPO + '/.github/workflows/recover-v163-after-consumed.yml@refs/heads/main'}
        self.context = mock.patch.dict(os.environ, context)
        self.context.start(); self.addCleanup(self.context.stop)
        self.posts = 0
        self.ambiguous = False
        self.bad_readback = False
        self.reconcile_count = 0
        self.fail_post_reconciliation = False
        self.dispatch_count = 1

    def run_api(self, args):
        if '--method' in args:
            self.posts += 1
            if self.ambiguous:
                raise RuntimeError('response lost')
            result = {'ref': successor.REF, 'object': {'sha': successor.consumed.COMMIT}}
        elif '/actions/workflows/' in args[-1]:
            result = {'total_count': self.dispatch_count, 'workflow_runs': [self.entry]}
        else:
            result = {'ref': successor.REF, 'object': {'sha': 'f' * 40 if self.bad_readback else successor.consumed.COMMIT}}
        return json.dumps(result).encode()

    def invoke(self):
        def reconcile(*args, **kwargs):
            self.reconcile_count += 1
            if self.fail_post_reconciliation and self.reconcile_count == 2:
                raise RuntimeError('post-claim evidence changed')
            return {'status': 'evidence_only_review_required'}
        return successor.claim(10, self.controller, self.receipt, run=self.run_api,
                               reconcile=reconcile)

    def test_one_new_claim(self):
        self.assertEqual(self.invoke()['status'], 'successor_claim_observed_review_required')
        self.assertEqual(self.posts, 1)
        observed = Path(str(self.receipt) + '.observed')
        self.assertEqual(json.loads(observed.read_text())['claim_ref'], successor.REF)
        self.assertNotEqual(json.loads(observed.read_text())['status'], 'reserved')

    def test_unknown_post_is_preserved_and_not_retried(self):
        self.ambiguous = True
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertEqual(self.posts, 1)
        unknown = Path(str(self.receipt) + '.unknown')
        self.assertEqual(json.loads(unknown.read_text())['status'], 'claim_outcome_unknown')
        with self.assertRaises(FileExistsError): self.invoke()
        self.assertEqual(self.posts, 1)

    def test_wrong_controller_refuses_before_post(self):
        self.entry['head_sha'] = 'b' * 40
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertEqual(self.posts, 0)

    def test_second_attempt_refuses_before_post(self):
        self.entry['run_attempt'] = 2
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertEqual(self.posts, 0)

    def test_competing_caller_refuses_before_post(self):
        os.environ['GITHUB_RUN_ID'] = '11'
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertEqual(self.posts, 0)

    def test_duplicate_dispatch_refuses_before_post(self):
        self.dispatch_count = 2
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertEqual(self.posts, 0)

    def test_bad_readback_keeps_unknown_without_observed(self):
        self.bad_readback = True
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertEqual(self.posts, 1)
        self.assertEqual(json.loads(Path(str(self.receipt) + '.unknown').read_text())['status'], 'claim_outcome_unknown')
        self.assertFalse(Path(str(self.receipt) + '.observed').exists())

    def test_post_claim_reconciliation_failure_keeps_unknown(self):
        self.fail_post_reconciliation = True
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertEqual(self.posts, 1)
        self.assertEqual(json.loads(Path(str(self.receipt) + '.unknown').read_text())['status'], 'claim_outcome_unknown')
        self.assertFalse(Path(str(self.receipt) + '.observed').exists())


if __name__ == '__main__':
    unittest.main()
