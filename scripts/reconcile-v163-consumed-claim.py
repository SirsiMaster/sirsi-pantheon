#!/usr/bin/env python3
"""Reconcile the single consumed v163 recovery claim and write a new receipt.

This is not authority to sign, retry, publish, create a claim, or accept the
old signed payload as equivalent. All GitHub and filesystem reads are snapshots.
"""
import argparse
import hashlib
import importlib.util
import io
import json
import os
import re
import stat
import subprocess
import zipfile
from pathlib import Path
from urllib.parse import urlencode

spec = importlib.util.spec_from_file_location('original', Path(__file__).with_name('reconcile-release-draft.py'))
original = importlib.util.module_from_spec(spec)
spec.loader.exec_module(original)

COMMIT = 'a5967913c68c03be435ab009f24df01e908681b3'
CONTROLLER = '683557bb5523631abd1db923a07da8fde93d986b'
JOB = '1c68b8d0-5d6e-44f8-afaf-a32e855ab16b'
INPUT = '9f72e8f1516761b63e415a0417834e586e691bb1b47a00f9b1e277c0ee1f03c5'
OUTPUT = '63416de2f7da1d12100256afdb7acc59feada6f1c5e2352e5fd43a776f1ee9d5'
RECEIPT = 'ea15c228df3e350efcccf29244cae45a64c55107fbde6e21a2a5b122b47243d7'
LOG_SHA = '6391703dbbe48fafe759d22258789b489641c06857bf3f17f4c483d4d82b418a'
REF = 'refs/heads/pantheon-recovery/v0.24.163-432359892'


def checked_file(root, path, limit):
    # Verify every component from the sign root, and do not follow a final link.
    for component in (Path('.'), *reversed(tuple(path.relative_to(root).parents)[:-1])):
        p = root / component
        if not stat.S_ISDIR(os.lstat(p).st_mode):
            raise RuntimeError('custody directory missing or not a directory')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode) or st.st_size > limit:
            raise RuntimeError('custody file type or size mismatch')
        with os.fdopen(fd, 'rb', closefd=False) as stream:
            data = stream.read(limit + 1)
        if len(data) != st.st_size:
            raise RuntimeError('custody file changed during read')
        return data
    finally:
        os.close(fd)


def verify_host(sign_root):
    if sign_root != Path('/Users/sirsimasterdev/sirsi-sign') or Path.home() != sign_root.parent:
        raise RuntimeError('signer host home mismatch')
    local = subprocess.run(['/usr/sbin/ioreg', '-rd1', '-c', 'IOPlatformExpertDevice'],
                           check=True, capture_output=True, timeout=10).stdout
    remote = subprocess.run(['/usr/bin/ssh', '-o', 'ConnectTimeout=10', '-o', 'BatchMode=yes',
                             '-o', 'StrictHostKeyChecking=yes', '-o', 'PasswordAuthentication=no',
                             '-o', 'KbdInteractiveAuthentication=no', 'sirsi-m1',
                             '/usr/sbin/ioreg -rd1 -c IOPlatformExpertDevice'],
                            check=True, capture_output=True, timeout=15).stdout
    pattern = rb'"IOPlatformUUID"\s*=\s*"([A-Fa-f0-9-]{36})"'
    local_ids, remote_ids = re.findall(pattern, local), re.findall(pattern, remote)
    if len(local_ids) != 1 or local_ids != remote_ids:
        raise RuntimeError('local host does not match enrolled M1')


def reconcile(sign_root, run=original.execute, reconcile_original=original.reconcile, host_check=verify_host):
    def api(endpoint):
        return json.loads(run(['gh', 'api', '--hostname', 'github.com', endpoint]))
    prefix = f'repos/{original.REPO}'
    original_evidence = reconcile_original('v0.24.163', COMMIT, 41592161041, 432359892,
                                           13210139655, run=run)
    if original_evidence.get('status') != 'reconciliation_review_required':
        raise RuntimeError('original draft reconciliation mismatch')
    claim = api(prefix + '/git/ref/heads/pantheon-recovery/v0.24.163-432359892')
    if claim.get('ref') != REF or claim.get('object', {}).get('sha') != COMMIT:
        raise RuntimeError('consumed claim mismatch')
    query = urlencode({'event': 'workflow_dispatch', 'per_page': 100, 'page': 1})
    runs = api(prefix + '/actions/workflows/recover-v163-release.yml/runs?' + query)
    entries = runs.get('workflow_runs')
    if runs.get('total_count') != 1 or not isinstance(entries, list) or len(entries) != 1:
        raise RuntimeError('recovery run listing not unique')
    item = entries[0]
    expected = {'id': 41602356213, 'run_attempt': 1, 'head_sha': CONTROLLER,
                'head_branch': 'main', 'event': 'workflow_dispatch',
                'status': 'completed', 'conclusion': 'failure'}
    if any(item.get(k) != v for k, v in expected.items()):
        raise RuntimeError('recovery run identity mismatch')
    host_check(sign_root)
    listing = api(prefix + '/actions/runs/41602356213/jobs?per_page=100')
    jobs = listing.get('jobs')
    if not isinstance(jobs, list) or listing.get('total_count') != len(jobs) or len(jobs) > 100:
        raise RuntimeError('recovery job list incomplete')
    found = [j for j in jobs if j.get('name') == 'recovery']
    if len(found) != 1 or found[0].get('id') != 125803872116 or found[0].get('conclusion') != 'failure':
        raise RuntimeError('failed recovery job not unique')
    log = run(['gh', 'api', '--allow-escape-sequences', '--hostname', 'github.com',
               prefix + '/actions/jobs/125803872116/logs'])
    if (hashlib.sha256(log).hexdigest() != LOG_SHA or
            b'signed payload differs after signature normalization' not in log or JOB.encode() not in log):
        raise RuntimeError('failed signer comparison log mismatch')
    steps = found[0].get('steps')
    if not isinstance(steps, list):
        raise RuntimeError('recovery steps missing')
    for name, result in (('Validate source and claim exactly one continuation', 'success'),
                         ('Build and sign exact original source once', 'failure'),
                         ('Preserve exact signed inputs', 'skipped'),
                         ('Upload exact signed custody before publication', 'skipped'),
                         ('Restore and verify exact signed inputs', 'skipped'),
                         ('Publish only the bound draft', 'skipped')):
        matches = [s for s in steps if s.get('name') == name]
        if len(matches) != 1 or matches[0].get('status') != 'completed' or matches[0].get('conclusion') != result:
            raise RuntimeError('recovery step disposition mismatch')
    listing = api(prefix + '/actions/runs/41602356213/artifacts?per_page=100')
    artifacts = listing.get('artifacts')
    if not isinstance(artifacts, list) or listing.get('total_count') != len(artifacts) or len(artifacts) > 100:
        raise RuntimeError('recovery artifact list incomplete')
    def artifact(artifact_id, name, member, guard=False):
        matches = [a for a in artifacts if a.get('name') == name and a.get('expired') is False]
        if len(matches) != 1 or matches[0].get('id') != artifact_id:
            raise RuntimeError('recovery artifact identity mismatch')
        archive = run(['gh', 'api', '--hostname', 'github.com', f'{prefix}/actions/artifacts/{artifact_id}/zip'])
        with zipfile.ZipFile(io.BytesIO(archive)) as bundle:
            members = bundle.infolist()
            exact = [m for m in members if m.filename == member]
            if len(members) > 100 or len(exact) != 1 or exact[0].is_dir() or exact[0].file_size > 65536:
                raise RuntimeError('recovery artifact member mismatch')
            if guard:
                jobs = [m.filename for m in members if m.filename.startswith('signer-jobs/')]
                incomplete = [m for m in members if m.filename.startswith('guard/') and m.filename.endswith('/INCOMPLETE')]
                accepted = [m for m in members if m.filename.startswith('guard/') and m.filename.endswith('/ACCEPTED')]
                if (jobs != [member] or len(incomplete) != 1 or accepted or incomplete[0].is_dir() or
                        incomplete[0].file_size > 65536 or
                        bundle.read(incomplete[0]) != b'INCOMPLETE: do not use; signing outcome may be ambiguous\n'):
                    raise RuntimeError('guard signer job count or incomplete marker mismatch')
            return json.loads(bundle.read(exact[0]))
    reservation = artifact(13214773272, 'v163-recovery-41602356213-1', 'v163-recovery-reservation.json')
    expected_reservation = {'claim_ref': REF, 'commit': COMMIT, 'original_run': 41592161041,
                            'recovery_run': 41602356213, 'release_id': 432359892,
                            'source_artifact_id': 13210139655, 'status': 'reserved', 'tag': 'v0.24.163'}
    if reservation != expected_reservation:
        raise RuntimeError('recovery reservation evidence mismatch')
    custody = artifact(13214678382, 'v163-recovery-guard-41602356213-1', f'signer-jobs/{JOB}.json', guard=True)
    if custody != {'host': 'sirsi-m1', 'input_name': 'sirsi-display-power-guard',
                   'input_sha256': INPUT, 'input_type': 'file', 'job': JOB,
                   'kind': 'bin', 'state': 'enqueue-unknown'}:
        raise RuntimeError('original signer request evidence mismatch')
    if not stat.S_ISDIR(os.lstat(sign_root).st_mode):
        raise RuntimeError('signer root is not a directory')
    if not stat.S_ISDIR(os.lstat(sign_root / 'outbox').st_mode):
        raise RuntimeError('signer outbox is not a directory')
    for directory in ('inbox', 'work'):
        parent = sign_root / directory
        if not stat.S_ISDIR(os.lstat(parent).st_mode):
            raise RuntimeError('signer queue directory not available')
        with os.scandir(parent) as entries:
            if any(JOB in entry.name for entry in entries):
                raise RuntimeError('old job still appears in signer queue')
    outbox = sign_root / 'outbox' / JOB
    if not stat.S_ISDIR(os.lstat(outbox).st_mode):
        raise RuntimeError('exact signer outbox unavailable')
    if (outbox / 'failed').exists() or (outbox / 'failed').is_symlink():
        raise RuntimeError('conflicting signer failure marker')
    if checked_file(sign_root, outbox / 'done', 1) != b'':
        raise RuntimeError('signer done marker mismatch')
    if hashlib.sha256(checked_file(sign_root, outbox / 'receipt.txt', 65536)).hexdigest() != RECEIPT:
        raise RuntimeError('signer receipt digest mismatch')
    if hashlib.sha256(checked_file(sign_root, outbox / 'sirsi-display-power-guard', 32 * 1024 * 1024)).hexdigest() != OUTPUT:
        raise RuntimeError('signed output digest mismatch')
    return {'status': 'evidence_only_review_required', 'claim_ref': REF, 'source_commit': COMMIT,
            'old_controller': CONTROLLER, 'old_run': 41602356213, 'signer_job': JOB,
            'signer_input_sha256': INPUT, 'signer_output_sha256': OUTPUT, 'signer_receipt_sha256': RECEIPT,
            'scope': 'old job done; publication skipped; original byte equivalence not established; no retry authority'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--receipt', required=True)
    args = parser.parse_args()
    result = reconcile(Path.home() / 'sirsi-sign')
    with open(args.receipt, 'x') as output:
        json.dump(result, output, sort_keys=True); output.write('\n'); output.flush(); os.fsync(output.fileno())


if __name__ == '__main__':
    main()
