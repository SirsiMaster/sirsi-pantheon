#!/usr/bin/env python3
"""Claim the reconciled v0.24.164 metadata successor and bind its existing draft.

An ambiguous create is retained as unknown and must never be retried blindly.
This program does not sign, upload or publish anything.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

REPO = 'SirsiMaster/sirsi-pantheon'
COMMIT = 'ca31d2a8984c02e54fa762de4c63333cd92bdf22'
REF = 'refs/heads/pantheon-recovery/v0.24.164-456022306-after-45278099354'
WORKFLOW = 'recover-v164-after-metadata.yml'


def api(endpoint, *args):
    command = ['gh', 'api', '--hostname', 'github.com', endpoint, *args]
    return json.loads(subprocess.run(command, check=True, stdout=subprocess.PIPE).stdout)


def job_log():
    result = subprocess.run(['gh', 'api', '--hostname', 'github.com', '--allow-escape-sequences',
                             'repos/' + REPO + '/actions/jobs/138055059522/logs'],
                            check=True, stdout=subprocess.PIPE)
    if len(result.stdout) > 2 * 1024 * 1024:
        raise RuntimeError('prior job log exceeds bound')
    return result.stdout


def save(path, value):
    with open(path, 'x', encoding='utf-8') as out:
        json.dump(value, out, sort_keys=True); out.write('\n'); out.flush(); os.fsync(out.fileno())
    fd = os.open(Path(path).parent, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def main():
    if len(sys.argv) != 3:
        raise SystemExit('usage: claim-v164-metadata-resume.py <reviewed-controller-sha> <new-evidence-directory>')
    controller, directory = sys.argv[1:]
    run_id = os.environ.get('GITHUB_RUN_ID', '')
    expected = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': REPO, 'GITHUB_RUN_ATTEMPT': '1',
                'GITHUB_REF': 'refs/heads/main', 'GITHUB_SHA': controller,
                'GITHUB_EVENT_NAME': 'workflow_dispatch', 'GITHUB_JOB': 'recovery_successor',
                'RUNNER_NAME': 'm1-pantheon',
                'GITHUB_WORKFLOW_REF': REPO + '/.github/workflows/' + WORKFLOW + '@refs/heads/main'}
    if (not re.fullmatch(r'[0-9a-f]{40}', controller) or not re.fullmatch(r'[1-9][0-9]*', run_id)
            or any(os.environ.get(k) != v for k, v in expected.items())):
        raise RuntimeError('recovery workflow identity mismatch')
    prefix = 'repos/' + REPO
    original = api(prefix + '/actions/runs/45270278775')
    if (original.get('head_sha') != COMMIT or original.get('head_branch') != 'v0.24.164'
            or original.get('event') != 'push' or original.get('status') != 'completed'
            or original.get('conclusion') != 'failure' or original.get('run_attempt') != 1):
        raise RuntimeError('original release run changed')
    release_runs = api(prefix + '/actions/workflows/release.yml/runs?event=push&branch=v0.24.164&per_page=100')
    release_entries = release_runs.get('workflow_runs')
    if (release_runs.get('total_count') != 1 or not isinstance(release_entries, list)
            or len(release_entries) != 1 or release_entries[0].get('id') != 45270278775
            or release_entries[0].get('head_sha') != COMMIT or release_entries[0].get('run_attempt') != 1
            or release_entries[0].get('status') != 'completed' or release_entries[0].get('conclusion') != 'failure'):
        raise RuntimeError('release publisher history changed')
    prior = api(prefix + '/actions/workflows/recover-v164-after-guard.yml/runs?event=workflow_dispatch&per_page=100')
    previous = prior.get('workflow_runs')
    if (prior.get('total_count') != 1 or not isinstance(previous, list) or len(previous) != 1
            or previous[0].get('id') != 45278099354 or previous[0].get('run_attempt') != 1
            or previous[0].get('head_sha') != '371057ee20cbb5f2f80a8bef43766a4986f741c0'
            or previous[0].get('status') != 'completed' or previous[0].get('conclusion') != 'failure'):
        raise RuntimeError('prior recovery is not the reconciled failed attempt')
    jobs = api(prefix + '/actions/runs/45278099354/jobs?per_page=100')
    matches = [j for j in jobs.get('jobs', []) if j.get('id') == 138055059522]
    if len(matches) != 1 or matches[0].get('conclusion') != 'failure':
        raise RuntimeError('prior recovery job identity changed')
    steps = {step.get('name'): step.get('conclusion') for step in matches[0].get('steps', [])}
    if (steps.get('Claim exactly one recovery') != 'success'
            or steps.get('Build and sign remaining artifacts') != 'failure'
            or steps.get('Publish only the existing draft') != 'skipped'):
        raise RuntimeError('prior recovery lifecycle changed')
    log = job_log()
    if b'original inputs not reproduced' not in log or b'Retaining signing directory:' in log:
        raise RuntimeError('prior recovery signer boundary is not established')
    old_ref = 'refs/heads/pantheon-recovery/v0.24.164-456022306-after-45270278775'
    old_claim = api(prefix + '/git/ref/heads/' + old_ref.removeprefix('refs/heads/'))
    if old_claim.get('ref') != old_ref or old_claim.get('object', {}).get('sha') != COMMIT:
        raise RuntimeError('prior recovery claim changed')
    current = api(prefix + '/actions/workflows/' + WORKFLOW + '/runs?event=workflow_dispatch&per_page=100')
    runs = current.get('workflow_runs')
    if (current.get('total_count') != 1 or not isinstance(runs, list) or len(runs) != 1
            or runs[0].get('id') != int(run_id) or runs[0].get('head_sha') != controller
            or runs[0].get('head_branch') != 'main' or runs[0].get('run_attempt') != 1
            or runs[0].get('status') != 'in_progress'):
        raise RuntimeError('recovery is not the unique first dispatch')
    release = api(prefix + '/releases/456022306')
    if (release.get('id') != 456022306 or release.get('tag_name') != 'v0.24.164'
            or release.get('target_commitish') != COMMIT or release.get('draft') is not True
            or release.get('assets') != []):
        raise RuntimeError('existing draft changed')
    ref = api(prefix + '/git/ref/tags/v0.24.164')
    tag = ref.get('object', {})
    if tag.get('type') != 'tag':
        raise RuntimeError('release tag is not annotated')
    peeled = api(prefix + '/git/tags/' + tag['sha']).get('object', {})
    if peeled.get('type') != 'commit' or peeled.get('sha') != COMMIT:
        raise RuntimeError('release tag changed')
    state = {'tag': 'v0.24.164', 'commit': COMMIT, 'release_id': 456022306,
             'original_run': 45270278775, 'prior_recovery_run': 45278099354,
             'prior_claim_ref': old_ref, 'successor_run': int(run_id),
             'controller_sha': controller, 'claim_ref': REF}
    path = Path(directory)
    save(path / 'claim-not-attempted.json', dict(state, status='claim_not_attempted'))
    save(path / 'claim-unknown.json', dict(state, status='claim_outcome_unknown'))
    result = api(prefix + '/git/refs', '--method', 'POST', '-f', 'ref=' + REF, '-f', 'sha=' + COMMIT)
    if result.get('ref') != REF or result.get('object', {}).get('sha') != COMMIT:
        raise RuntimeError('claim outcome unknown: do not retry')
    readback = api(prefix + '/git/ref/heads/' + REF.removeprefix('refs/heads/'))
    if readback.get('ref') != REF or readback.get('object', {}).get('sha') != COMMIT:
        raise RuntimeError('claim readback unknown: do not retry')
    save(path / 'claim-observed.json', dict(state, status='claim_observed'))
    save(path / 'reservation.json', dict(state, status='reserved'))


if __name__ == '__main__':
    main()
