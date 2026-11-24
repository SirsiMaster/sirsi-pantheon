#!/usr/bin/env python3
"""Claim a publication-only continuation from the exact retained v164 custody."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import sys

spec = importlib.util.spec_from_file_location('base', Path(__file__).with_name('claim-v164-metadata-resume.py'))
base = importlib.util.module_from_spec(spec)
spec.loader.exec_module(base)
REF = 'refs/heads/pantheon-recovery/v0.24.164-456022306-publish-after-45291181859'
WORKFLOW = 'recover-v164-publication-only.yml'
OLD_MOUNT = '/private/var/folders/qs/sx7tq5qd6y5f8tq30v4gsmkc0000gn/T/pantheon-guard-publication-mount-ia3ennux'
HASHES = {
    'Pantheon.app.zip': '7722bf0188c1123796da6f7d22722d90d7c878f3a5594675a456740aa408f16b',
    'SirsiPantheon-0.24.164-arm64-pretoolguard.dmg': 'de2faff8d1163ea6076d3f5ff42a197552755baeb99e8fb9f9630c647c7f9826',
    'SirsiPantheon-0.24.164-arm64.dmg': 'd6f7024226749aa35029aa5a5dd78dc68586a5a37fe69bab84dbb3354a43b4ca',
    'SirsiPantheon-0.24.164-arm64.pkg': 'f8b42d629c83dd4fa4383419e37aaa918593166578aa9a2c4d882c8adbed75da',
    'payload-manifest.json': '4578b1fa29cbc264c1fef4fd28a1c578dfeb9ba06fb486e820ebcbd8e4202cc2',
}


def digest(path):
    if not path.is_file() or path.is_symlink():
        raise RuntimeError('custody file missing or symlinked')
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def old_mount_absent():
    proc = subprocess.run(['/usr/bin/hdiutil', 'info', '-plist'], check=True, stdout=subprocess.PIPE, timeout=30)
    if len(proc.stdout) > 4 * 1024 * 1024:
        raise RuntimeError('mount listing exceeds bound')
    images = plistlib.loads(proc.stdout).get('images')
    if not isinstance(images, list):
        raise RuntimeError('mount listing invalid')
    mounts = [entity.get('mount-point') for image in images for entity in image.get('system-entities', [])]
    return OLD_MOUNT not in mounts and not os.path.ismount(OLD_MOUNT)


def unique_file(root, name):
    matches = list(root.rglob(name))
    if len(matches) != 1 or not matches[0].is_file() or matches[0].is_symlink():
        raise RuntimeError('prior evidence missing or ambiguous: ' + name)
    return matches[0]


def reconcile_evidence(custody, evidence):
    record = json.loads((custody / 'custody.json').read_text())
    if record != {'commit': base.COMMIT, 'version': '0.24.164', 'sha256': HASHES}:
        raise RuntimeError('signed custody record changed')
    if any(digest(custody / name) != value for name, value in HASHES.items()):
        raise RuntimeError('signed custody bytes changed')
    receipt = json.loads(unique_file(evidence, 'publication.json').read_text())
    expected = {name: value for name, value in HASHES.items() if name.endswith(('.dmg', '.pkg'))}
    actual = receipt.get('assets')
    if (receipt.get('status') != 'failed' or receipt.get('tag') != 'v0.24.164'
            or receipt.get('commit') != base.COMMIT or receipt.get('uploads_attempted') != []
            or receipt.get('publish_attempted') is not False
            or not str(receipt.get('error', '')).startswith('guard image mount/device association unknown; inspect /var/folders/qs/')):
        raise RuntimeError('prior publication outcome is not the known pre-upload failure')
    if not isinstance(actual, dict) or set(actual) != set(expected) or any(actual[n].get('sha256') != v for n, v in expected.items()):
        raise RuntimeError('prior publisher assets do not match custody')
    claim = json.loads(unique_file(evidence, 'claim-observed.json').read_text())
    if (claim.get('status') != 'claim_observed' or claim.get('successor_run') != 45284797296
            or claim.get('release_id') != 456022306 or claim.get('commit') != base.COMMIT
            or claim.get('controller_sha') != '3fe7dde3526a1e905f7b5aefd00af541a37a1941'):
        raise RuntimeError('prior successor claim evidence changed')
    plist = unique_file(evidence, 'pantheon-guard-attach-45284797296-1-6rz9atxm.plist')
    if digest(plist) != '0530d21d7c2c6b42b2f6088711cc5f2838e13d436d93bfe2517a6c474fccb3a7':
        raise RuntimeError('prior attachment evidence changed')
    entities = plistlib.loads(plist.read_bytes()).get('system-entities')
    if not isinstance(entities, list) or len(entities) != 1 or entities[0].get('dev-entry') != '/dev/disk10' or entities[0].get('mount-point') != OLD_MOUNT:
        raise RuntimeError('prior attachment identity changed')
    cask = unique_file(evidence, 'sirsi-pantheon.rb')
    if digest(cask) != '0e8c7665e0bd09443ddbf924d70cc84cb92ce4f23bdade30b387a5e8e1a063e2':
        raise RuntimeError('retained cask changed')
    if not old_mount_absent():
        raise RuntimeError('prior mount remains active')
    return cask


def main():
    if len(sys.argv) != 5:
        raise SystemExit('usage: claim-v164-publication-resume.py <controller> <custody> <evidence> <claim-dir>')
    controller, custody, evidence, directory = sys.argv[1:]
    run_id = os.environ.get('GITHUB_RUN_ID', '')
    expected = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': base.REPO, 'GITHUB_RUN_ATTEMPT': '1',
                'GITHUB_REF': 'refs/heads/main', 'GITHUB_SHA': controller, 'GITHUB_EVENT_NAME': 'workflow_dispatch',
                'GITHUB_JOB': 'publication_successor', 'RUNNER_NAME': 'm1-pantheon',
                'GITHUB_WORKFLOW_REF': base.REPO + '/.github/workflows/' + WORKFLOW + '@refs/heads/main'}
    if (not re.fullmatch(r'[0-9a-f]{40}', controller) or not re.fullmatch(r'[1-9][0-9]*', run_id)
            or any(os.environ.get(k) != v for k, v in expected.items())):
        raise RuntimeError('publication workflow identity mismatch')
    prefix = 'repos/' + base.REPO
    for workflow, event, branch, run, sha in (
        ('release.yml', 'push', '&branch=v0.24.164', 45270278775, base.COMMIT),
        ('recover-v164-after-guard.yml', 'workflow_dispatch', '', 45278099354, '371057ee20cbb5f2f80a8bef43766a4986f741c0'),
        ('recover-v164-after-metadata.yml', 'workflow_dispatch', '', 45284797296, '3fe7dde3526a1e905f7b5aefd00af541a37a1941'),
    ):
        data = base.api(prefix + '/actions/workflows/' + workflow + '/runs?event=' + event + branch + '&per_page=100')
        entries = data.get('workflow_runs')
        status = 'in_progress' if run == int(run_id) else 'completed'
        if (data.get('total_count') != 1 or not isinstance(entries, list) or len(entries) != 1
                or entries[0].get('id') != run or entries[0].get('head_sha') != sha
                or entries[0].get('run_attempt') != 1 or entries[0].get('status') != status
                or (run != int(run_id) and entries[0].get('conclusion') != 'failure')):
            raise RuntimeError('release or recovery run history changed')
    runs = base.api(prefix + '/actions/workflows/' + WORKFLOW + '/runs?event=workflow_dispatch&per_page=100')
    entries = runs.get('workflow_runs', [])
    expected_runs = {
        45291181859: ('f630401e1b3a1cb1ed679422f7dcf5b332edb4be', 'completed', 'failure'),
        int(run_id): (controller, 'in_progress', None),
    }
    if runs.get('total_count') != 2 or len(entries) != 2 or {e.get('id') for e in entries} != set(expected_runs):
        raise RuntimeError('publication run history changed')
    for entry in entries:
        sha, status, conclusion = expected_runs[entry['id']]
        if (entry.get('head_sha') != sha or entry.get('status') != status
                or entry.get('conclusion') != conclusion or entry.get('run_attempt') != 1):
            raise RuntimeError('publication run identity changed')
    failed = base.api(prefix + '/actions/runs/45291181859/jobs?per_page=100')
    prior_jobs = [job for job in failed.get('jobs', []) if job.get('id') == 138100055662]
    if len(prior_jobs) != 1 or prior_jobs[0].get('conclusion') != 'failure':
        raise RuntimeError('prior publication-only job changed')
    prior_steps = {step.get('name'): step.get('conclusion') for step in prior_jobs[0].get('steps', [])}
    if (prior_steps.get('Claim one publication-only continuation') != 'failure'
            or prior_steps.get('Publish only the existing draft') != 'skipped'):
        raise RuntimeError('prior publication-only lifecycle changed')
    refs = base.api(prefix + '/git/matching-refs/heads/pantheon-recovery/v0.24.164-456022306')
    expected_refs = {
        'refs/heads/pantheon-recovery/v0.24.164-456022306-after-45270278775',
        'refs/heads/pantheon-recovery/v0.24.164-456022306-after-45278099354',
    }
    if not isinstance(refs, list) or {ref.get('ref') for ref in refs} != expected_refs:
        raise RuntimeError('unexpected publication claim exists')
    prior = base.api(prefix + '/actions/runs/45284797296/jobs?per_page=100')
    jobs = [job for job in prior.get('jobs', []) if job.get('id') == 138078240323]
    if len(jobs) != 1 or jobs[0].get('conclusion') != 'failure':
        raise RuntimeError('prior publication job changed')
    steps = {step.get('name'): step.get('conclusion') for step in jobs[0].get('steps', [])}
    for name, result in (('Claim exactly one recovery', 'success'), ('Build and sign remaining artifacts', 'success'),
                         ('Restore and verify exact signed inputs', 'success'), ('Publish only the existing draft', 'failure')):
        if steps.get(name) != result:
            raise RuntimeError('prior publication lifecycle changed')
    for ref in ('refs/heads/pantheon-recovery/v0.24.164-456022306-after-45270278775',
                'refs/heads/pantheon-recovery/v0.24.164-456022306-after-45278099354'):
        old = base.api(prefix + '/git/ref/heads/' + ref.removeprefix('refs/heads/'))
        if old.get('ref') != ref or old.get('object', {}).get('sha') != base.COMMIT:
            raise RuntimeError('prior claim changed')
    for artifact_id, name, artifact_digest in (
        (14872220529, 'v164-metadata-custody-45284797296', 'sha256:790a270ed85bac61524ec3ff8fedd2b92c183f2db6e2b7bf8c5c8b31be2e0bd3'),
        (14872310501, 'v164-metadata-successor-45284797296-1', 'sha256:23ea294fac9c404903ee47c15ed7eacebbef334fc01de5c7f97915ef4a499f14'),
    ):
        artifact = base.api(prefix + '/actions/artifacts/' + str(artifact_id))
        if (artifact.get('id') != artifact_id or artifact.get('name') != name or artifact.get('expired') is not False
                or artifact.get('digest') != artifact_digest or artifact.get('workflow_run', {}).get('id') != 45284797296):
            raise RuntimeError('prior custody artifact changed')
    release = base.api(prefix + '/releases/456022306')
    if (release.get('id') != 456022306 or release.get('tag_name') != 'v0.24.164'
            or release.get('target_commitish') != base.COMMIT or release.get('draft') is not True or release.get('assets') != []):
        raise RuntimeError('existing draft changed')
    tag = base.api(prefix + '/git/ref/tags/v0.24.164').get('object', {})
    if (tag.get('type') != 'tag' or tag.get('sha') != 'c2e8a1bfed5a1a3cae7ba208b953e4aa89656bf7'
            or base.api(prefix + '/git/tags/' + tag['sha']).get('object', {}).get('sha') != base.COMMIT):
        raise RuntimeError('release tag changed')
    cask = reconcile_evidence(Path(custody), Path(evidence))
    out = Path(directory)
    with (out / 'sirsi-pantheon.rb').open('xb') as target:
        target.write(cask.read_bytes()); target.flush(); os.fsync(target.fileno())
    state = {'tag': 'v0.24.164', 'commit': base.COMMIT, 'release_id': 456022306,
             'prior_recovery_run': 45284797296, 'successor_run': int(run_id),
             'controller_sha': controller, 'claim_ref': REF}
    base.save(out / 'claim-not-attempted.json', dict(state, status='claim_not_attempted'))
    base.save(out / 'claim-unknown.json', dict(state, status='claim_outcome_unknown'))
    result = base.api(prefix + '/git/refs', '--method', 'POST', '-f', 'ref=' + REF, '-f', 'sha=' + base.COMMIT)
    if result.get('ref') != REF or result.get('object', {}).get('sha') != base.COMMIT:
        raise RuntimeError('claim outcome unknown: do not retry')
    check = base.api(prefix + '/git/ref/heads/' + REF.removeprefix('refs/heads/'))
    if check.get('ref') != REF or check.get('object', {}).get('sha') != base.COMMIT or not old_mount_absent():
        raise RuntimeError('claim or mount readback unknown: do not retry')
    base.save(out / 'claim-observed.json', dict(state, status='claim_observed'))
    base.save(out / 'reservation.json', dict(state, status='reserved'))


if __name__ == '__main__':
    main()
