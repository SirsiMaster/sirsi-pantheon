#!/usr/bin/env python3
"""Read-only reconciliation of one failed pre-sign release reservation.

This emits a *new* receipt only; it neither creates a release nor authorizes a
second publisher. Callers must separately serialize a reviewed continuation.
GitHub's paginated listings are not an atomic snapshot against external actors.
"""
import argparse
import io
import json
import os
import re
import subprocess
import zipfile
from urllib.parse import urlencode

REPO = 'SirsiMaster/sirsi-pantheon'
SKIPPED = ('Import Developer ID signing identities',
           'Resolve enrolled or imported Developer ID identity',
           'Preflight imported Developer ID Installer identity',
           'Build + sign + notarize DMG and PKG (arm64 — Apple silicon)')


def execute(argv):
    result = subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=60, check=False)
    if result.returncode or len(result.stdout) + len(result.stderr) > 2 * 1024 * 1024:
        raise RuntimeError('readback failed or exceeded bound')
    return result.stdout


def reconcile(tag, commit, run_id, release_id, expected_artifact_id, run=execute):
    if (not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?', tag) or
            not re.fullmatch(r'[0-9a-f]{40}', commit) or
            type(run_id) is not int or run_id <= 0 or type(release_id) is not int or release_id <= 0 or
            type(expected_artifact_id) is not int or expected_artifact_id <= 0):
        raise ValueError('exact tag, commit, run ID and release ID required')

    def api(endpoint):
        return json.loads(run(['gh', 'api', '--hostname', 'github.com', endpoint]))

    refs = run(['git', 'ls-remote', '--exit-code', 'origin', 'refs/tags/' + tag,
                'refs/tags/' + tag + '^{}']).decode().splitlines()
    refs = dict(line.split('\t', 1)[::-1] for line in refs)
    if refs.get('refs/tags/' + tag + '^{}') != commit:
        raise RuntimeError('annotated tag does not peel to expected commit')
    query = urlencode({'branch': tag, 'event': 'push', 'per_page': 100, 'page': 1})
    runs = api(f'repos/{REPO}/actions/workflows/release.yml/runs?' + query)
    entries = runs.get('workflow_runs')
    if (type(runs.get('total_count')) is not int or not isinstance(entries, list) or
            runs['total_count'] != 1 or len(entries) != 1):
        raise RuntimeError('single original tag-run custody not established')
    original = entries[0]
    if (original.get('id') != run_id or original.get('head_sha') != commit or
            original.get('head_branch') != tag or original.get('event') != 'push' or
            original.get('run_attempt') != 1 or original.get('status') != 'completed' or
            original.get('conclusion') != 'failure'):
        raise RuntimeError('original run identity or terminal state mismatch')
    jobs = api(f'repos/{REPO}/actions/runs/{run_id}/jobs?per_page=100')
    entries = jobs.get('jobs')
    if (type(jobs.get('total_count')) is not int or not isinstance(entries, list) or
            jobs['total_count'] != len(entries) or len(entries) > 100):
        raise RuntimeError('job listing incomplete')
    build = [job for job in entries if job.get('name') == 'Build Pantheon (macOS DMG + PKG)']
    if len(build) != 1 or build[0].get('conclusion') != 'failure':
        raise RuntimeError('original failed build job not unique')
    steps = build[0].get('steps')
    if not isinstance(steps, list):
        raise RuntimeError('original step evidence missing')
    def step(name, outcome):
        matches = [item for item in steps if item.get('name') == name]
        return len(matches) == 1 and matches[0].get('status') == 'completed' and matches[0].get('conclusion') == outcome
    if not step('Reserve exact source-bound release draft', 'failure') or not all(step(name, 'skipped') for name in SKIPPED):
        raise RuntimeError('pre-sign failure not established')
    artifact_name = f'pantheon-reservation-{run_id}'
    artifacts = api(f'repos/{REPO}/actions/runs/{run_id}/artifacts?per_page=100')
    entries = artifacts.get('artifacts')
    if (type(artifacts.get('total_count')) is not int or not isinstance(entries, list) or
            artifacts['total_count'] != len(entries) or len(entries) > 100):
        raise RuntimeError('artifact listing incomplete')
    matching = [a for a in entries if a.get('name') == artifact_name and a.get('expired') is False]
    if len(matching) != 1 or matching[0].get('id') != expected_artifact_id:
        raise RuntimeError('reservation artifact not unique')
    artifact_id = matching[0]['id']
    archive = run(['gh', 'api', '--hostname', 'github.com',
                   f'repos/{REPO}/actions/artifacts/{artifact_id}/zip'])
    with zipfile.ZipFile(io.BytesIO(archive)) as bundle:
        members = bundle.infolist()
        if (len(members) != 1 or members[0].filename != artifact_name + '.json' or
                members[0].is_dir() or members[0].file_size > 65536):
            raise RuntimeError('unexpected reservation artifact contents')
        receipt = json.loads(bundle.read(members[0]))
    if (receipt.get('tag') != tag or receipt.get('commit') != commit or
            receipt.get('release_id') != release_id or receipt.get('creation_attempted') is not True or
            receipt.get('status') != 'create_outcome_unknown' or
            receipt.get('error') != 'ambiguous post-create release discovery'):
        raise RuntimeError('retained reservation identity mismatch')
    seen, same_tag = set(), []
    for page in range(1, 11):
        entries = api(f'repos/{REPO}/releases?per_page=100&page={page}')
        if not isinstance(entries, list) or len(entries) > 100:
            raise RuntimeError('release discovery incomplete')
        for item in entries:
            if (not isinstance(item, dict) or type(item.get('id')) is not int or item['id'] <= 0 or
                    item['id'] in seen or not isinstance(item.get('tag_name'), str) or
                    type(item.get('draft')) is not bool):
                raise RuntimeError('ambiguous release discovery')
            seen.add(item['id'])
            if item['tag_name'] == tag:
                same_tag.append(item['id'])
        if len(entries) < 100:
            break
    else:
        raise RuntimeError('release discovery incomplete')
    if same_tag != [release_id]:
        raise RuntimeError('observed conflicting draft identity')
    release = api(f'repos/{REPO}/releases/{release_id}')
    if (release.get('id') != release_id or release.get('tag_name') != tag or
            release.get('target_commitish') != commit or release.get('draft') is not True or
            release.get('assets') != []):
        raise RuntimeError('exact draft identity or empty assets mismatch')
    return {'tag': tag, 'commit': commit, 'release_id': release_id, 'status': 'reconciliation_review_required',
            'reconciled_from_run': run_id, 'source_artifact_id': artifact_id,
            'scope': 'pre-sign readback only; external actors not serialized'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--run-id', type=int, required=True)
    parser.add_argument('--release-id', type=int, required=True)
    parser.add_argument('--artifact-id', type=int, required=True)
    parser.add_argument('--receipt', required=True)
    args = parser.parse_args()
    result = reconcile(args.tag, args.commit, args.run_id, args.release_id, args.artifact_id)
    with open(args.receipt, 'x') as output:
        json.dump(result, output, sort_keys=True)
        output.write('\n'); output.flush(); os.fsync(output.fileno())


if __name__ == '__main__':
    main()
