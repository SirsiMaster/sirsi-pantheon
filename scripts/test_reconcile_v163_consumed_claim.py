import importlib.util
import hashlib
import io
import json
import os
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock

spec = importlib.util.spec_from_file_location('claim', Path(__file__).with_name('reconcile-v163-consumed-claim.py'))
claim = importlib.util.module_from_spec(spec)
spec.loader.exec_module(claim)


def archive(name, content, guard=False, accepted=False, marker='INCOMPLETE: do not use; signing outcome may be ambiguous\n'):
    result = io.BytesIO()
    with zipfile.ZipFile(result, 'w') as z:
        z.writestr(name, json.dumps(content))
        if guard:
            z.writestr('guard/signing.fixture/INCOMPLETE', marker)
            if accepted:
                z.writestr('guard/signing.fixture/ACCEPTED', '')
    return result.getvalue()


class ReconciliationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name) / 'sirsi-sign'
        for name in ('inbox', 'work', 'outbox'):
            (self.root / name).mkdir(parents=True, exist_ok=True)
        out = self.root / 'outbox' / claim.JOB
        out.mkdir()
        (out / 'done').write_bytes(b'')
        (out / 'receipt.txt').write_bytes(b'receipt')
        (out / 'sirsi-display-power-guard').write_bytes(b'output')
        self.receipt_hash = mock.patch.object(claim, 'RECEIPT', '6f32860910ca0fb2a20c7fda143666b09dbf8db5238195c90a586fb542ff0cad')
        self.output_hash = mock.patch.object(claim, 'OUTPUT', 'e0ee8bb50685e05fa0f47ed04203ae953fdfd055f5bd2892ea186504254f8c3a')
        self.receipt_hash.start(); self.output_hash.start()
        self.addCleanup(self.receipt_hash.stop); self.addCleanup(self.output_hash.stop)
        self.log = b'signed payload differs after signature normalization ' + claim.JOB.encode()
        self.log_hash = mock.patch.object(claim, 'LOG_SHA', hashlib.sha256(self.log).hexdigest())
        self.log_hash.start(); self.addCleanup(self.log_hash.stop)
        self.accepted = False
        self.marker = 'INCOMPLETE: do not use; signing outcome may be ambiguous\n'
        self.addCleanup(self.tmp.cleanup)
        self.reservation = {'claim_ref': claim.REF, 'commit': claim.COMMIT, 'original_run': 41592161041,
                            'recovery_run': 41602356213, 'release_id': 432359892,
                            'source_artifact_id': 13210139655, 'status': 'reserved', 'tag': 'v0.24.163'}
        self.custody = {'host': 'sirsi-m1', 'input_name': 'sirsi-display-power-guard',
                        'input_sha256': claim.INPUT, 'input_type': 'file', 'job': claim.JOB,
                        'kind': 'bin', 'state': 'enqueue-unknown'}
        self.run_entry = {'id': 41602356213, 'run_attempt': 1, 'head_sha': claim.CONTROLLER,
                          'head_branch': 'main', 'event': 'workflow_dispatch',
                          'status': 'completed', 'conclusion': 'failure'}
        self.steps = [{'name': n, 'status': 'completed', 'conclusion': c} for n, c in (
            ('Validate source and claim exactly one continuation', 'success'),
            ('Build and sign exact original source once', 'failure'),
            ('Preserve exact signed inputs', 'skipped'),
            ('Upload exact signed custody before publication', 'skipped'),
            ('Restore and verify exact signed inputs', 'skipped'),
            ('Publish only the bound draft', 'skipped'))]

    def run_api(self, args):
        endpoint = args[-1]
        if '/git/ref/' in endpoint:
            value = {'ref': claim.REF, 'object': {'sha': claim.COMMIT}}
        elif '/workflows/' in endpoint:
            value = {'total_count': 1, 'workflow_runs': [self.run_entry]}
        elif '/jobs?' in endpoint:
            value = {'total_count': 1, 'jobs': [{'id': 125803872116, 'name': 'recovery', 'conclusion': 'failure', 'steps': self.steps}]}
        elif '/jobs/125803872116/logs' in endpoint:
            return self.log
        elif '/artifacts?' in endpoint:
            value = {'total_count': 2, 'artifacts': [
                {'id': 13214773272, 'name': 'v163-recovery-41602356213-1', 'expired': False},
                {'id': 13214678382, 'name': 'v163-recovery-guard-41602356213-1', 'expired': False}]}
        elif '/13214773272/zip' in endpoint:
            return archive('v163-recovery-reservation.json', self.reservation)
        elif '/13214678382/zip' in endpoint:
            return archive(f'signer-jobs/{claim.JOB}.json', self.custody, guard=True,
                           accepted=self.accepted, marker=self.marker)
        else:
            raise AssertionError(endpoint)
        return json.dumps(value).encode()

    def invoke(self):
        return claim.reconcile(self.root, run=self.run_api,
                               reconcile_original=lambda *a, **k: {'status': 'reconciliation_review_required'},
                               host_check=lambda root: None)

    def test_exact_evidence_only(self):
        self.assertEqual(self.invoke()['status'], 'evidence_only_review_required')

    def test_refuse_replayed_run(self):
        self.run_entry['run_attempt'] = 2
        with self.assertRaises(RuntimeError): self.invoke()

    def test_refuse_possible_publication(self):
        self.steps[-1]['conclusion'] = 'success'
        with self.assertRaises(RuntimeError): self.invoke()

    def test_refuse_unknown_enqueue_input(self):
        self.custody['input_sha256'] = '0' * 64
        with self.assertRaises(RuntimeError): self.invoke()

    def test_refuse_queued_same_job(self):
        (self.root / 'inbox' / ('.' + claim.JOB + '.partial')).mkdir()
        with self.assertRaises(RuntimeError): self.invoke()

    def test_refuse_symlinked_done(self):
        marker = self.root / 'outbox' / claim.JOB / 'done'
        marker.unlink(); marker.symlink_to('receipt.txt')
        with self.assertRaises(OSError): self.invoke()

    def test_refuse_changed_output(self):
        (self.root / 'outbox' / claim.JOB / 'sirsi-display-power-guard').write_bytes(b'other')
        with self.assertRaises(RuntimeError): self.invoke()

    def test_refuse_wrong_host(self):
        with self.assertRaises(RuntimeError): claim.verify_host(self.root)

    def test_host_uuid_match_and_mismatch(self):
        host = Path('/Users/sirsimasterdev')
        local = mock.Mock(stdout=b'"IOPlatformUUID" = "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"')
        different = mock.Mock(stdout=b'"IOPlatformUUID" = "BBBBBBBB-BBBB-CCCC-DDDD-EEEEEEEEEEEE"')
        with mock.patch.object(claim.Path, 'home', return_value=host):
            with mock.patch.object(claim.subprocess, 'run', side_effect=[local, local]):
                claim.verify_host(host / 'sirsi-sign')
            with mock.patch.object(claim.subprocess, 'run', side_effect=[local, different]):
                with self.assertRaises(RuntimeError): claim.verify_host(host / 'sirsi-sign')

    def test_refuse_contradictory_acceptance(self):
        self.accepted = True
        with self.assertRaises(RuntimeError): self.invoke()

    def test_refuse_wrong_incomplete_marker(self):
        self.marker = ''
        with self.assertRaises(RuntimeError): self.invoke()

    def test_refuse_fifo_without_blocking(self):
        marker = self.root / 'outbox' / claim.JOB / 'done'
        marker.unlink(); os.mkfifo(marker)
        with self.assertRaises(RuntimeError): self.invoke()


if __name__ == '__main__':
    unittest.main()
