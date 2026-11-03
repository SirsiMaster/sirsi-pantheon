import importlib.util
import io
import json
import unittest
import zipfile
from pathlib import Path

spec = importlib.util.spec_from_file_location('reconcile', Path(__file__).with_name('reconcile-release-draft.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ReconciliationFixtureTests(unittest.TestCase):
    def fixture(self, *, mutation=None):
        tag, commit, run_id, release_id = 'v0.24.163', 'a' * 40, 123, 314
        original = {'id': run_id, 'head_sha': commit, 'head_branch': tag, 'event': 'push',
                    'run_attempt': 1, 'status': 'completed', 'conclusion': 'failure'}
        steps = [{'name': 'Reserve exact source-bound release draft', 'status': 'completed', 'conclusion': 'failure'}]
        steps += [{'name': name, 'status': 'completed', 'conclusion': 'skipped'} for name in module.SKIPPED]
        jobs = {'total_count': 1, 'jobs': [{'name': 'Build Pantheon (macOS DMG + PKG)',
                                             'conclusion': 'failure', 'steps': steps}]}
        release = {'id': release_id, 'tag_name': tag, 'target_commitish': commit, 'draft': True, 'assets': []}
        receipt = {'tag': tag, 'commit': commit, 'release_id': release_id,
                   'creation_attempted': True, 'status': 'create_outcome_unknown',
                   'error': 'ambiguous post-create release discovery'}
        data = {'original': original, 'jobs': jobs, 'release': release, 'receipt': receipt, 'artifact_id': 17,
                'listing': [release]}
        if mutation:
            mutation(data)
        calls = []

        def run(argv):
            calls.append(argv)
            if argv[0] == 'git':
                return (('f' * 40) + '\trefs/tags/' + tag + '\n' + commit + '\trefs/tags/' + tag + '^{}\n').encode()
            endpoint = argv[-1]
            if endpoint.endswith('/artifacts/17/zip'):
                buffer = io.BytesIO()
                with zipfile.ZipFile(buffer, 'w') as bundle:
                    bundle.writestr(f'pantheon-reservation-{run_id}.json', json.dumps(data['receipt']))
                return buffer.getvalue()
            if '/actions/workflows/' in endpoint:
                return json.dumps({'total_count': 1, 'workflow_runs': [data['original']]}).encode()
            if endpoint.endswith('/jobs?per_page=100'):
                return json.dumps(data['jobs']).encode()
            if endpoint.endswith('/artifacts?per_page=100'):
                return json.dumps({'total_count': 1, 'artifacts': [{'id': data['artifact_id'], 'expired': False,
                                      'name': f'pantheon-reservation-{run_id}'}]}).encode()
            if endpoint.endswith('/releases/314'):
                return json.dumps(data['release']).encode()
            if '/releases?per_page=100&page=' in endpoint:
                return json.dumps(data['listing']).encode()
            raise AssertionError(argv)

        return lambda: module.reconcile(tag, commit, run_id, release_id, 17, run=run), calls

    def test_exact_presign_draft_is_read_only_and_bound(self):
        invoke, calls = self.fixture()
        result = invoke()
        self.assertEqual(result['release_id'], 314)
        self.assertEqual(result['status'], 'reconciliation_review_required')
        self.assertEqual(result['reconciled_from_run'], 123)
        self.assertEqual(result['source_artifact_id'], 17)
        self.assertTrue(any(call[-1].endswith('/artifacts/17/zip') for call in calls))
        self.assertFalse(any('--method' in call or '-X' in call for call in calls))

    def test_changed_receipt_draft_assets_or_run_refuses(self):
        changes = [lambda d: d['receipt'].update(release_id=315),
                   lambda d: d.update(artifact_id=18),
                   lambda d: d['receipt'].update(error='release already exists; reconcile before signing'),
                   lambda d: d['listing'].append({'id': 315, 'tag_name': 'v0.24.163', 'draft': True}),
                   lambda d: d['release'].update(assets=[{'name': 'unexpected'}]),
                   lambda d: d['release'].update(draft=False),
                   lambda d: d['original'].update(run_attempt=2),
                   lambda d: d['jobs']['jobs'][0]['steps'][-1].update(conclusion='success')]
        for change in changes:
            with self.subTest(change=change):
                invoke, calls = self.fixture(mutation=change)
                with self.assertRaises(RuntimeError):
                    invoke()
                self.assertFalse(any('--method' in call or '-X' in call for call in calls))


if __name__ == '__main__':
    unittest.main()
