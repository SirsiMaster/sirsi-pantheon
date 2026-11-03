import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location('claim', Path(__file__).with_name('claim-release-recovery.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ClaimFixtureTests(unittest.TestCase):
    def test_second_dispatch_is_refused_before_claim(self):
        with tempfile.TemporaryDirectory() as directory:
            calls = []
            def reconcile(*args, **kwargs):
                return {'status': 'reconciliation_review_required', 'source_artifact_id': 17}
            def run(argv):
                calls.append(argv)
                return json.dumps({'total_count': 2, 'workflow_runs': [{'id': 456}, {'id': 457}]}).encode()
            path = Path(directory) / 'receipt.json'
            with self.assertRaises(RuntimeError):
                module.claim('v0.24.163', 'a' * 40, 123, 314, 456, str(path), run=run, reconcile=reconcile)
            self.assertEqual(json.loads(path.read_text())['status'], 'claim_not_attempted')
            self.assertFalse(any('--method' in call for call in calls))

    def test_single_claim_and_revalidation_produce_bound_reservation(self):
        with tempfile.TemporaryDirectory() as directory:
            calls, checks = [], []
            commit, tag = 'a' * 40, 'v0.24.163'
            ref = 'refs/heads/pantheon-recovery/v0.24.163-314'
            def reconcile(*args, **kwargs):
                checks.append(args)
                return {'status': 'reconciliation_review_required', 'source_artifact_id': 17}
            def run(argv):
                calls.append(argv)
                if '/runs?' in argv[-1]:
                    return json.dumps({'total_count': 1, 'workflow_runs': [{'id': 456, 'run_attempt': 1}]}).encode()
                return json.dumps({'ref': ref, 'object': {'sha': commit}}).encode()
            path = Path(directory) / 'receipt.json'
            result = module.claim(tag, commit, 123, 314, 456, str(path), run=run, reconcile=reconcile)
            self.assertEqual(result['status'], 'reserved')
            self.assertEqual(json.loads(path.read_text()), result)
            self.assertEqual(len(checks), 2)
            self.assertEqual(sum('--method' in call for call in calls), 1)
            with self.assertRaises(FileExistsError):
                module.claim(tag, commit, 123, 314, 456, str(path), run=run, reconcile=reconcile)
            self.assertEqual(sum('--method' in call for call in calls), 1)

    def test_unknown_claim_outcome_is_preserved_and_never_retried(self):
        with tempfile.TemporaryDirectory() as directory:
            calls = []
            def reconcile(*args, **kwargs):
                return {'status': 'reconciliation_review_required', 'source_artifact_id': 17}
            def run(argv):
                calls.append(argv)
                if '/runs?' in argv[-1]:
                    return json.dumps({'total_count': 1, 'workflow_runs': [{'id': 456, 'run_attempt': 1}]}).encode()
                raise RuntimeError('lost response')
            path = Path(directory) / 'receipt.json'
            with self.assertRaises(RuntimeError):
                module.claim('v0.24.163', 'a' * 40, 123, 314, 456, str(path), run=run, reconcile=reconcile)
            self.assertEqual(json.loads(path.read_text())['status'], 'claim_outcome_unknown')
            self.assertEqual(sum('--method' in call for call in calls), 1)

    def test_changed_readback_after_claim_never_yields_reserved(self):
        with tempfile.TemporaryDirectory() as directory:
            checks = []
            def reconcile(*args, **kwargs):
                checks.append(True)
                if len(checks) == 2:
                    raise RuntimeError('draft changed')
                return {'status': 'reconciliation_review_required', 'source_artifact_id': 17}
            def run(argv):
                if '/runs?' in argv[-1]:
                    return json.dumps({'total_count': 1, 'workflow_runs': [{'id': 456, 'run_attempt': 1}]}).encode()
                return json.dumps({'ref': 'refs/heads/pantheon-recovery/v0.24.163-314',
                                   'object': {'sha': 'a' * 40}}).encode()
            path = Path(directory) / 'receipt.json'
            with self.assertRaises(RuntimeError):
                module.claim('v0.24.163', 'a' * 40, 123, 314, 456, str(path), run=run, reconcile=reconcile)
            self.assertEqual(json.loads(path.read_text())['status'], 'claim_outcome_unknown')


if __name__ == '__main__':
    unittest.main()
