#!/usr/bin/env python3
"""One create-only claim for the retained v165 publication; never retry unknowns."""
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
COMMIT = '9f7b0362018e5ecd738dbbf7c91899c3a8f2c723'
TAG_OBJECT = 'e2b1612fe049206616aa4b6224eede17c816d89b'
REF = 'refs/heads/pantheon-recovery/v0.24.165-456974029-publication'
WORKFLOW = 'recover-v165-publication-only.yml'
OLD_MOUNT = '/private/var/folders/qs/sx7tq5qd6y5f8tq30v4gsmkc0000gn/T/pantheon-guard-publication-mount-bqzboagn'
OLD_DEVICE = '/dev/disk10'
PLIST = 'pantheon-guard-attach-45427969009-1-ai29aktv.plist'
HASHES = {
    'Pantheon.app.zip': 'd8fb73ac3099c166062e791a41382a4d1760b79a974d7cb0cd6d3e86c05407a8',
    'SirsiPantheon-0.24.165-arm64-pretoolguard.dmg': 'a3a6b77cc1d0ecb7a977c8feeb94df83d20501aea73f5aba4c8d39dca1c03eae',
    'SirsiPantheon-0.24.165-arm64.dmg': 'ef6ae348a65ace2a1381d4bda62735e1522966254e23c01354fc885a652b0951',
    'SirsiPantheon-0.24.165-arm64.pkg': 'a86a88e126f98395a3b160a873e65d77c992844999b94418890f5a6cbebc5d84',
    'payload-manifest.json': '4c9f57dcf5dc2d2e79ef3133c4db3dda1f8968912de8010211686224eea278dc',
}
ARTIFACTS = {
    14938886001: ('pantheon-reservation-45427969009', 'sha256:5582a23898591fc1d63d00d287791d412a79a394e85d42b948af80e88e15a9eb'),
    14938657648: ('pantheon-release-custody-45427969009', 'sha256:2811bb8cad1b6955daf397f3f1ee1e5f805bb6f905b81abbfc0d28dcea9e0cde'),
    14937028858: ('pantheon-publication-45427969009-1', 'sha256:bf28a3609ae1fcf8839b4ea76ec3bb701af3a13169cfb28fe1f7848f0b717fac'),
}


def digest(path):
    if path.is_symlink() or not path.is_file():
        raise RuntimeError('missing or symlinked evidence: ' + str(path))
    h = hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda: f.read(1048576), b''):
            h.update(chunk)
    return h.hexdigest()


def unique(root, name):
    paths = list(root.rglob(name))
    if len(paths) != 1 or paths[0].is_symlink() or not paths[0].is_file():
        raise RuntimeError('missing or ambiguous evidence: ' + name)
    return paths[0]


def old_mount_absent():
    result = subprocess.run(['/usr/bin/hdiutil', 'info', '-plist'], check=True, stdout=subprocess.PIPE, timeout=30)
    if len(result.stdout) > 4 * 1024 * 1024:
        raise RuntimeError('mount listing exceeds bound')
    images = plistlib.loads(result.stdout).get('images')
    if not isinstance(images, list):
        raise RuntimeError('unknown image listing')
    for image in images:
        entities = image.get('system-entities')
        if not isinstance(entities, list):
            raise RuntimeError('unknown image topology')
        for entity in entities:
            if not isinstance(entity, dict):
                raise RuntimeError('unknown image entity')
            mount, device = entity.get('mount-point'), entity.get('dev-entry')
            if isinstance(mount, str) and os.path.realpath(mount) == OLD_MOUNT:
                return False
            if isinstance(device, str) and (device == OLD_DEVICE or re.fullmatch(r'/dev/disk10s[0-9]+', device)):
                return False
    return not os.path.ismount(OLD_MOUNT)


def reconcile(custody, evidence, reservations):
    record = json.loads(unique(custody, 'custody.json').read_text())
    if record != {'commit': COMMIT, 'version': '0.24.165', 'sha256': HASHES}:
        raise RuntimeError('custody identity mismatch')
    for name, expected in HASHES.items():
        if digest(unique(custody, name)) != expected:
            raise RuntimeError('custody bytes changed: ' + name)
    reservation = json.loads(unique(reservations, 'pantheon-reservation-45427969009.json').read_text())
    if reservation != {'commit': COMMIT, 'creation_attempted': True, 'release_id': 456974029,
                       'status': 'reserved', 'tag': 'v0.24.165'}:
        raise RuntimeError('original reservation changed')
    receipt = json.loads(unique(evidence, 'pantheon-publication-45427969009-1.json').read_text())
    expected = {k: v for k, v in HASHES.items() if k.endswith(('.dmg', '.pkg'))}
    assets = receipt.get('assets')
    if (receipt.get('status') != 'failed' or receipt.get('published') is not None
            or receipt.get('tag') != 'v0.24.165' or receipt.get('commit') != COMMIT
            or receipt.get('uploads_attempted') != [] or receipt.get('publish_attempted') is not False
            or receipt.get('creation_attempted') is not False or not isinstance(assets, dict)
            or set(assets) != set(expected) or any(assets[k].get('sha256') != v for k, v in expected.items())
            or 'pantheon-guard-publication-mount-bqzboagn' not in receipt.get('error', '')
            or PLIST not in receipt.get('error', '')):
        raise RuntimeError('prior publication is not the known pre-upload failure')
    plist = unique(evidence, PLIST)
    if digest(plist) != '460a898db6145a9fa8f85248f28abb344b903813d5f564b2bff64ed542e98f01':
        raise RuntimeError('prior attach evidence changed')
    entities = plistlib.loads(plist.read_bytes()).get('system-entities')
    if not isinstance(entities, list) or len(entities) != 1 or entities[0].get('dev-entry') != OLD_DEVICE or entities[0].get('mount-point') != OLD_MOUNT:
        raise RuntimeError('prior device/mount identity changed')


def main():
    if len(sys.argv) != 6:
        raise SystemExit('usage: claim controller custody evidence reservation output')
    controller, custody, evidence, reservation, out = sys.argv[1:]
    run = os.environ.get('GITHUB_RUN_ID', '')
    env = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': base.REPO, 'GITHUB_RUN_ATTEMPT': '1',
           'GITHUB_REF': 'refs/heads/main', 'GITHUB_SHA': controller, 'GITHUB_EVENT_NAME': 'workflow_dispatch',
           'GITHUB_JOB': 'publication_successor', 'RUNNER_NAME': 'm1-pantheon',
           'GITHUB_WORKFLOW_REF': base.REPO + '/.github/workflows/' + WORKFLOW + '@refs/heads/main'}
    if not re.fullmatch(r'[0-9a-f]{40}', controller) or not re.fullmatch(r'[1-9][0-9]*', run) or any(os.environ.get(k) != v for k, v in env.items()):
        raise RuntimeError('workflow identity mismatch')
    prefix = 'repos/' + base.REPO
    original = base.api(prefix + '/actions/workflows/release.yml/runs?event=push&branch=v0.24.165&per_page=100')
    runs = original.get('workflow_runs')
    if original.get('total_count') != 1 or not isinstance(runs, list) or len(runs) != 1 or any(runs[0].get(k) != v for k, v in {'id': 45427969009, 'head_sha': COMMIT, 'head_branch': 'v0.24.165', 'status': 'completed', 'conclusion': 'failure', 'run_attempt': 1}.items()):
        raise RuntimeError('release run history changed')
    current = base.api(prefix + '/actions/workflows/' + WORKFLOW + '/runs?event=workflow_dispatch&per_page=100')
    entries = current.get('workflow_runs')
    if current.get('total_count') != 1 or not isinstance(entries, list) or len(entries) != 1 or any(entries[0].get(k) != v for k, v in {'id': int(run), 'head_sha': controller, 'head_branch': 'main', 'status': 'in_progress', 'run_attempt': 1}.items()):
        raise RuntimeError('not unique first dispatch')
    refs = base.api(prefix + '/git/matching-refs/heads/pantheon-recovery/v0.24.165-456974029')
    if refs != []:
        raise RuntimeError('prior recovery claim exists or unknown')
    for ident, (name, sha) in ARTIFACTS.items():
        a = base.api(prefix + '/actions/artifacts/' + str(ident))
        if a.get('id') != ident or a.get('name') != name or a.get('digest') != sha or a.get('expired') is not False or a.get('workflow_run', {}).get('id') != 45427969009:
            raise RuntimeError('artifact provenance changed')
    release = base.api(prefix + '/releases/456974029')
    if any(release.get(k) != v for k, v in {'id': 456974029, 'tag_name': 'v0.24.165', 'target_commitish': COMMIT, 'draft': True, 'assets': []}.items()):
        raise RuntimeError('draft changed')
    tag = base.api(prefix + '/git/ref/tags/v0.24.165').get('object', {})
    if tag.get('type') != 'tag' or tag.get('sha') != TAG_OBJECT or base.api(prefix + '/git/tags/' + TAG_OBJECT).get('object') != {'type': 'commit', 'sha': COMMIT, 'url': 'https://api.github.com/repos/SirsiMaster/sirsi-pantheon/git/commits/' + COMMIT}:
        raise RuntimeError('tag identity changed')
    reconcile(Path(custody), Path(evidence), Path(reservation))
    state = {'tag': 'v0.24.165', 'commit': COMMIT, 'release_id': 456974029, 'original_run': 45427969009,
             'successor_run': int(run), 'controller_sha': controller, 'claim_ref': REF}
    out = Path(out)
    base.save(out / 'claim-not-attempted.json', dict(state, status='claim_not_attempted'))
    if not old_mount_absent():
        raise RuntimeError('original mount or device still active')
    base.save(out / 'claim-unknown.json', dict(state, status='claim_outcome_unknown'))
    result = base.api(prefix + '/git/refs', '--method', 'POST', '-f', 'ref=' + REF, '-f', 'sha=' + COMMIT)
    if result.get('ref') != REF or result.get('object', {}).get('sha') != COMMIT:
        raise RuntimeError('claim outcome unknown: do not retry')
    check = base.api(prefix + '/git/ref/heads/' + REF.removeprefix('refs/heads/'))
    if check.get('ref') != REF or check.get('object', {}).get('sha') != COMMIT or not old_mount_absent():
        raise RuntimeError('claim or mount readback unknown: do not retry')
    base.save(out / 'claim-observed.json', dict(state, status='claim_observed'))
    base.save(out / 'reservation.json', dict(state, status='reserved'))


if __name__ == '__main__':
    main()
