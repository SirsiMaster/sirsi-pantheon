#!/usr/bin/env python3
"""Prepare a source-bound publisher reservation only after a qualified claim step.

This script neither signs nor publishes. It must run only as the immediately
following protected workflow step on the reviewed controller, within the same
host-safety and admission boundary. Snapshot checks cannot serialize external
privileged actors or protect against hostile same-user code.
"""
import argparse
import importlib.util
import json
import os
import re
import stat
from pathlib import Path
from urllib.parse import urlencode

spec = importlib.util.spec_from_file_location('successor', Path(__file__).with_name('claim-v163-recovery-successor.py'))
successor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(successor)


def prepare(run_id, controller_sha, claim_path, output_path, run=successor.consumed.original.execute,
            reconcile=successor.consumed.reconcile):
    context = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': successor.consumed.original.REPO,
               'GITHUB_RUN_ID': str(run_id), 'GITHUB_RUN_ATTEMPT': '1', 'GITHUB_REF': 'refs/heads/main',
               'GITHUB_SHA': controller_sha, 'GITHUB_EVENT_NAME': 'workflow_dispatch',
               'GITHUB_JOB': 'recovery_successor', 'RUNNER_NAME': 'm1-pantheon',
               'GITHUB_WORKFLOW_REF': successor.consumed.original.REPO + '/.github/workflows/recover-v163-after-consumed.yml@refs/heads/main'}
    if (type(run_id) is not int or run_id <= 0 or not re.fullmatch(r'[0-9a-f]{40}', controller_sha) or
            any(os.environ.get(k) != v for k, v in context.items())):
        raise RuntimeError('trusted successor workflow context mismatch')
    expected = {'tag': 'v0.24.163', 'commit': successor.consumed.COMMIT, 'release_id': 432359892,
                'old_recovery_run': 41602356213, 'successor_run': run_id, 'controller_sha': controller_sha,
                'old_claim_ref': successor.consumed.REF, 'claim_ref': successor.REF}
    for suffix, status in (('.not-attempted', 'claim_not_attempted'),
                           ('.unknown', 'claim_outcome_unknown'),
                           ('.observed', 'successor_claim_observed_review_required')):
        fd = os.open(str(claim_path) + suffix, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_size > 65536:
                raise RuntimeError('invalid successor state file')
            with os.fdopen(fd, 'rb', closefd=False) as stream:
                value = json.load(stream)
        finally:
            os.close(fd)
        if value != dict(expected, status=status):
            raise RuntimeError('incomplete, contradictory or misbound successor state')
    prefix = f'repos/{successor.consumed.original.REPO}'
    def api(endpoint):
        return json.loads(run(['gh', 'api', '--hostname', 'github.com', endpoint]))
    query = urlencode({'event': 'workflow_dispatch', 'per_page': 100, 'page': 1})
    dispatches = api(prefix + '/actions/workflows/recover-v163-after-consumed.yml/runs?' + query)
    runs = dispatches.get('workflow_runs')
    if (dispatches.get('total_count') != 1 or not isinstance(runs, list) or len(runs) != 1 or
            runs[0].get('id') != run_id or runs[0].get('run_attempt') != 1 or
            runs[0].get('head_sha') != controller_sha or runs[0].get('head_branch') != 'main' or
            runs[0].get('event') != 'workflow_dispatch' or runs[0].get('status') != 'in_progress'):
        raise RuntimeError('successor dispatch changed')
    jobs = api(f'{prefix}/actions/runs/{run_id}/jobs?per_page=100')
    entries = jobs.get('jobs')
    if not isinstance(entries, list) or jobs.get('total_count') != len(entries) or len(entries) > 100:
        raise RuntimeError('successor job listing incomplete')
    selected = [j for j in entries if j.get('name') == 'recovery-successor']
    if (len(selected) != 1 or selected[0].get('status') != 'in_progress' or
            selected[0].get('runner_name') != os.environ['RUNNER_NAME']):
        raise RuntimeError('live successor job not unique')
    steps = selected[0].get('steps')
    if not isinstance(steps, list):
        raise RuntimeError('successor step list missing')
    claim_steps = [s for s in steps if s.get('name') == 'Claim reviewed successor']
    prepare_steps = [s for s in steps if s.get('name') == 'Prepare successor publisher handoff']
    if (len(claim_steps) != 1 or len(prepare_steps) != 1 or
            claim_steps[0].get('status') != 'completed' or claim_steps[0].get('conclusion') != 'success' or
            prepare_steps[0].get('status') != 'in_progress' or
            type(claim_steps[0].get('number')) is not int or
            prepare_steps[0].get('number') != claim_steps[0]['number'] + 1):
        raise RuntimeError('successful claim step not established')
    ref = api(prefix + '/git/ref/heads/pantheon-recovery/v0.24.163-432359892-after-41602356213')
    if ref.get('ref') != successor.REF or ref.get('object', {}).get('sha') != successor.consumed.COMMIT:
        raise RuntimeError('successor claim changed')
    old = reconcile(Path.home() / 'sirsi-sign', run=run)
    if old.get('status') != 'evidence_only_review_required':
        raise RuntimeError('old release custody changed')
    result = dict(expected, status='reserved')
    stage_path = str(output_path) + '.staged'
    with open(stage_path, 'x') as output:
        json.dump(result, output, sort_keys=True); output.write('\n'); output.flush(); os.fsync(output.fileno())
    # Exclusive link avoids replacing any pre-existing output; a later workflow
    # step MUST also require this prepare step's successful conclusion.
    os.link(stage_path, output_path, follow_symlinks=False)
    parent = os.open(Path(output_path).parent, os.O_RDONLY)
    try: os.fsync(parent)
    finally: os.close(parent)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-id', type=int, required=True)
    parser.add_argument('--controller-sha', required=True)
    parser.add_argument('--claim-path', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    prepare(args.run_id, args.controller_sha, args.claim_path, args.output)


if __name__ == '__main__':
    main()
