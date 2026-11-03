#!/usr/bin/env python3
"""Create one separate successor claim after exact old-custody reconciliation.

Never retries an uncertain POST. This does not sign, publish, modify the old
claim, or establish a host-safe window. It is not itself authority to proceed.
State files are create-only: .unknown dominates .not-attempted, and .observed
dominates both. Missing, partial, or contradictory state requires reconciliation.
Workflow environment binding assumes the trusted runner, not hostile same-UID code.
"""
import argparse
import importlib.util
import json
import os
import re
from pathlib import Path
from urllib.parse import urlencode

spec = importlib.util.spec_from_file_location('consumed', Path(__file__).with_name('reconcile-v163-consumed-claim.py'))
consumed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(consumed)

REF = 'refs/heads/pantheon-recovery/v0.24.163-432359892-after-41602356213'


def claim(run_id, controller_sha, receipt_path, run=consumed.original.execute, reconcile=consumed.reconcile):
    if type(run_id) is not int or run_id <= 0 or not re.fullmatch(r'[0-9a-f]{40}', controller_sha):
        raise ValueError('exact successor run ID and controller SHA required')
    prefix = f'repos/{consumed.original.REPO}'
    expected_context = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': consumed.original.REPO,
                        'GITHUB_RUN_ID': str(run_id), 'GITHUB_RUN_ATTEMPT': '1',
                        'GITHUB_REF': 'refs/heads/main', 'GITHUB_SHA': controller_sha,
                        'GITHUB_EVENT_NAME': 'workflow_dispatch',
                        'GITHUB_WORKFLOW_REF': consumed.original.REPO + '/.github/workflows/recover-v163-after-consumed.yml@refs/heads/main'}
    if any(os.environ.get(k) != v for k, v in expected_context.items()):
        raise RuntimeError('current workflow context mismatch')
    def api(endpoint):
        return json.loads(run(['gh', 'api', '--hostname', 'github.com', endpoint]))
    def check_single_dispatch():
        query = urlencode({'event': 'workflow_dispatch', 'per_page': 100, 'page': 1})
        result = api(prefix + '/actions/workflows/recover-v163-after-consumed.yml/runs?' + query)
        entries = result.get('workflow_runs')
        if (result.get('total_count') != 1 or not isinstance(entries, list) or len(entries) != 1 or
                entries[0].get('id') != run_id or entries[0].get('run_attempt') != 1 or
                entries[0].get('head_branch') != 'main' or entries[0].get('head_sha') != controller_sha or
                entries[0].get('event') != 'workflow_dispatch' or entries[0].get('status') != 'in_progress'):
            raise RuntimeError('successor dispatch is not the unique first attempt')
    state = {'tag': 'v0.24.163', 'commit': consumed.COMMIT, 'release_id': 432359892,
             'old_recovery_run': 41602356213, 'successor_run': run_id, 'controller_sha': controller_sha,
             'old_claim_ref': consumed.REF, 'claim_ref': REF, 'status': 'claim_not_attempted'}
    receipt_path = Path(receipt_path)
    def save(suffix):
        path = Path(str(receipt_path) + suffix)
        with open(path, 'x') as receipt:
            json.dump(state, receipt, sort_keys=True); receipt.write('\n')
            receipt.flush(); os.fsync(receipt.fileno())
        parent = os.open(path.parent, os.O_RDONLY)
        try: os.fsync(parent)
        finally: os.close(parent)
    save('.not-attempted')
    observed = reconcile(Path.home() / 'sirsi-sign', run=run)
    if observed.get('status') != 'evidence_only_review_required':
        raise RuntimeError('old custody reconciliation mismatch')
    check_single_dispatch()
    state['status'] = 'claim_outcome_unknown'
    save('.unknown')
    result = json.loads(run(['gh', 'api', '--hostname', 'github.com', '--method', 'POST',
                             prefix + '/git/refs', '-f', 'ref=' + REF,
                             '-f', 'sha=' + consumed.COMMIT]))
    if result.get('ref') != REF or result.get('object', {}).get('sha') != consumed.COMMIT:
        raise RuntimeError('successor claim outcome unknown; do not retry')
    observed_ref = api(prefix + '/git/ref/heads/pantheon-recovery/v0.24.163-432359892-after-41602356213')
    if observed_ref.get('ref') != REF or observed_ref.get('object', {}).get('sha') != consumed.COMMIT:
        raise RuntimeError('successor claim readback unknown; do not retry')
    check_single_dispatch()
    observed = reconcile(Path.home() / 'sirsi-sign', run=run)
    if observed.get('status') != 'evidence_only_review_required':
        raise RuntimeError('old custody changed after claim')
    state['status'] = 'successor_claim_observed_review_required'
    save('.observed')
    return state


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-id', type=int, required=True)
    parser.add_argument('--controller-sha', required=True)
    parser.add_argument('--receipt', required=True)
    args = parser.parse_args()
    claim(args.run_id, args.controller_sha, args.receipt)


if __name__ == '__main__':
    main()
