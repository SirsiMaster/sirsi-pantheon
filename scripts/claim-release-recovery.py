#!/usr/bin/env python3
"""Create one durable GitHub claim for a previously reconciled release.

No signing, release creation or tag mutation occurs here. A lost create reply
is deliberately not retried. The claim is retained permanently for custody.
"""
import argparse
import importlib.util
import json
import os
import re
from pathlib import Path
from urllib.parse import urlencode

spec = importlib.util.spec_from_file_location('reconcile_release', Path(__file__).with_name('reconcile-release-draft.py'))
reconcile_release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reconcile_release)


def claim(tag, commit, run_id, release_id, recovery_run_id, receipt_path, run=reconcile_release.execute,
          reconcile=reconcile_release.reconcile):
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?', tag):
        raise ValueError('invalid tag')
    if type(recovery_run_id) is not int or recovery_run_id <= 0:
        raise ValueError('exact current recovery run required')
    ref = f'refs/heads/pantheon-recovery/{tag}-{release_id}'
    state = {'tag': tag, 'commit': commit, 'release_id': release_id,
             'original_run': run_id, 'recovery_run': recovery_run_id,
             'claim_ref': ref, 'status': 'claim_not_attempted'}
    with open(receipt_path, 'x') as receipt:
        def save():
            receipt.seek(0); receipt.truncate()
            json.dump(state, receipt, sort_keys=True)
            receipt.write('\n'); receipt.flush(); os.fsync(receipt.fileno())
        save()
        evidence = reconcile(tag, commit, run_id, release_id, 13210139655, run=run)
        if evidence.get('status') != 'reconciliation_review_required':
            raise RuntimeError('reconciliation evidence mismatch')
        query = urlencode({'event': 'workflow_dispatch', 'per_page': 100, 'page': 1})
        runs = json.loads(run(['gh', 'api', '--hostname', 'github.com',
                               f'repos/{reconcile_release.REPO}/actions/workflows/recover-v163-release.yml/runs?' + query]))
        entries = runs.get('workflow_runs')
        if (runs.get('total_count') != 1 or not isinstance(entries, list) or len(entries) != 1 or
                entries[0].get('id') != recovery_run_id or entries[0].get('run_attempt') != 1):
            raise RuntimeError('recovery dispatch is not the sole first attempt')
        state.update(status='claim_outcome_unknown', source_artifact_id=evidence['source_artifact_id'])
        save()
        result = json.loads(run(['gh', 'api', '--hostname', 'github.com', '--method', 'POST',
                                 f'repos/{reconcile_release.REPO}/git/refs',
                                 '-f', 'ref=' + ref, '-f', 'sha=' + commit]))
        if result.get('ref') != ref or result.get('object', {}).get('sha') != commit:
            raise RuntimeError('claim response unknown; do not retry')
        observed = json.loads(run(['gh', 'api', '--hostname', 'github.com',
                                   f'repos/{reconcile_release.REPO}/git/ref/heads/pantheon-recovery/{tag}-{release_id}']))
        if observed.get('ref') != ref or observed.get('object', {}).get('sha') != commit:
            raise RuntimeError('claim readback unknown; do not retry')
        # Detect changes after the claim before handing off to the signer.
        reconcile(tag, commit, run_id, release_id, 13210139655, run=run)
        state['status'] = 'reserved'
        save()
    return state


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--run-id', type=int, required=True)
    parser.add_argument('--release-id', type=int, required=True)
    parser.add_argument('--recovery-run-id', type=int, required=True)
    parser.add_argument('--receipt', required=True)
    args = parser.parse_args()
    claim(args.tag, args.commit, args.run_id, args.release_id, args.recovery_run_id, args.receipt)


if __name__ == '__main__':
    main()
