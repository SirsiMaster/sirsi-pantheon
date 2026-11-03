import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

spec = importlib.util.spec_from_file_location('handoff', Path(__file__).with_name('prepare-v163-successor-publication.py'))
handoff = importlib.util.module_from_spec(spec)
spec.loader.exec_module(handoff)


class HandoffTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name) / 'claim'
        self.output = Path(self.tmp.name) / 'reservation.json'
        self.controller = 'a' * 40
        self.repo = handoff.successor.consumed.original.REPO
        self.expected = {'tag': 'v0.24.163', 'commit': handoff.successor.consumed.COMMIT,
                         'release_id': 432359892, 'old_recovery_run': 41602356213,
                         'successor_run': 10, 'controller_sha': self.controller,
                         'old_claim_ref': handoff.successor.consumed.REF, 'claim_ref': handoff.successor.REF}
        for suffix, status in (('.not-attempted', 'claim_not_attempted'),
                               ('.unknown', 'claim_outcome_unknown'),
                               ('.observed', 'successor_claim_observed_review_required')):
            Path(str(self.base) + suffix).write_text(json.dumps(dict(self.expected, status=status)))
        context = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': self.repo, 'GITHUB_RUN_ID': '10',
                   'GITHUB_RUN_ATTEMPT': '1', 'GITHUB_REF': 'refs/heads/main', 'GITHUB_SHA': self.controller,
                   'GITHUB_EVENT_NAME': 'workflow_dispatch',
                   'GITHUB_JOB': 'recovery_successor', 'RUNNER_NAME': 'm1-pantheon',
                   'GITHUB_WORKFLOW_REF': self.repo + '/.github/workflows/recover-v163-after-consumed.yml@refs/heads/main'}
        self.context = mock.patch.dict(os.environ, context)
        self.context.start(); self.addCleanup(self.context.stop)
        self.step_outcome = 'success'
        self.prepare_number = 8
        self.runner_name = 'm1-pantheon'
        self.run_count = 1

    def run_api(self, args):
        endpoint = args[-1]
        if '/workflows/' in endpoint:
            result = {'total_count': self.run_count, 'workflow_runs': [
                {'id': 10, 'run_attempt': 1, 'head_sha': self.controller, 'head_branch': 'main',
                 'event': 'workflow_dispatch', 'status': 'in_progress'}]}
        elif '/jobs?' in endpoint:
            result = {'total_count': 1, 'jobs': [{'name': 'recovery-successor', 'status': 'in_progress',
                'runner_name': self.runner_name,
                'steps': [{'name': 'Claim reviewed successor', 'number': 7, 'status': 'completed', 'conclusion': self.step_outcome},
                          {'name': 'Prepare successor publisher handoff', 'number': self.prepare_number, 'status': 'in_progress'}]}]}
        else:
            result = {'ref': handoff.successor.REF, 'object': {'sha': handoff.successor.consumed.COMMIT}}
        return json.dumps(result).encode()

    def invoke(self):
        return handoff.prepare(10, self.controller, self.base, self.output, run=self.run_api,
                               reconcile=lambda *a, **k: {'status': 'evidence_only_review_required'})

    def test_exact_handoff(self):
        self.assertEqual(self.invoke()['status'], 'reserved')
        self.assertEqual(json.loads(self.output.read_text())['claim_ref'], handoff.successor.REF)

    def test_unknown_without_observed_is_not_reservation(self):
        Path(str(self.base) + '.observed').unlink()
        with self.assertRaises(OSError): self.invoke()
        self.assertFalse(self.output.exists())

    def test_partial_later_state_refuses(self):
        Path(str(self.base) + '.observed').write_text('{')
        with self.assertRaises(ValueError): self.invoke()
        self.assertFalse(self.output.exists())

    def test_observed_without_successful_step_refuses(self):
        self.step_outcome = 'failure'
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertFalse(self.output.exists())

    def test_second_dispatch_refuses(self):
        self.run_count = 2
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertFalse(self.output.exists())

    def test_wrong_runner_refuses(self):
        self.runner_name = 'another-runner'
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertFalse(self.output.exists())

    def test_nonadjacent_step_refuses(self):
        self.prepare_number = 9
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertFalse(self.output.exists())

    def test_mismatched_controller_refuses(self):
        observed = Path(str(self.base) + '.observed')
        value = json.loads(observed.read_text()); value['controller_sha'] = 'b' * 40
        observed.write_text(json.dumps(value))
        with self.assertRaises(RuntimeError): self.invoke()
        self.assertFalse(self.output.exists())


if __name__ == '__main__':
    unittest.main()
