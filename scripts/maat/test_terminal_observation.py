"""Owned terminal receipts with inert wait/reap, process and backend fixtures.

These tests do not register, reserve, signal, spawn or release a real workload.
In particular, importing this discovery module must not change process signals,
timers or resource limits; the external qualification driver owns its limits.
"""
import ctypes
import errno
import importlib.util
import json
import os
from pathlib import Path
import signal
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

source = Path(__file__).with_name('admitted-run.py')
spec = importlib.util.spec_from_file_location('terminal_observation_runner', source)
supervisor = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = supervisor
spec.loader.exec_module(supervisor)
@unittest.skipUnless(sys.platform == 'darwin', 'Darwin public wait ABI fixtures')
class TerminalObservationTests(unittest.TestCase):
    def setUp(self):
        self.addCleanup(ctypes.set_errno, ctypes.get_errno())

    def child(self, local=False):
        # Deliberately synthetic identity. This is not registration or authority
        # over a real PID; every wait, snapshot, reap and signal below is inert.
        child = SimpleNamespace(pid=424242, returncode=None)
        identity_type = supervisor._LocalChildIdentity if local else supervisor._FabricChildIdentity
        owned = identity_type(child, child.pid, os.getpid(), os.getuid(),
                              os.getuid(), child.pid, 'synthetic-start')
        setattr(child, '_sirsi_local_identity' if local else '_sirsi_fabric_identity', owned)
        return child, owned

    def wait(self, child, status, code=1, **overrides):
        def fake_wait(kind, pid, pointer, flags):
            self.assertEqual((kind, pid, flags), (1, child.pid, 1 | 4 | 0x20))
            info = ctypes.cast(pointer, ctypes.POINTER(supervisor._Siginfo)).contents
            values = dict(pid=child.pid, uid=0, signo=int(signal.SIGCHLD),
                          error=0, code=code, status=status)
            values.update(overrides)
            for key, value in values.items():
                setattr(info, key, value)
            return 0
        return fake_wait

    def observe(self, child, owned, status, code=1, **overrides):
        return supervisor._nonreaping_child_terminal(
            child, owned, wait=self.wait(child, status, code, **overrides))

    def test_ordinary_exit_and_signal_controls(self):
        for code, raw, expected in [(1, 0, 0), (1, 1, 1), (1, 2, 2),
                                    (1, 255, 255), (2, 9, -9), (3, 11, -11)]:
            with self.subTest(code=code, raw=raw):
                child, owned = self.child()
                self.assertEqual(self.observe(child, owned, raw, code), expected)
                self.assertEqual(supervisor._terminal_expected_reap(child, expected), expected)

    def test_wide_nonzero_observation_never_reports_success(self):
        for raw, portable, reap in [(256, 1, 0), (258, 2, 2),
                                     (65538, 2, 2), (16777215, 255, 255)]:
            with self.subTest(raw=raw):
                child, owned = self.child()
                self.assertEqual(self.observe(child, owned, raw), portable)
                self.assertNotEqual(portable, 0)
                self.assertEqual(supervisor._terminal_expected_reap(child, portable), reap)
                evidence = supervisor._child_terminal_evidence(child)
                self.assertEqual((evidence['raw_status'], evidence['portable_exit'],
                                  evidence['expected_reap_exit']), (raw, portable, reap))

    def test_first_nondeciding_observer_latches_without_replacing_raw(self):
        child, owned = self.child()
        self.assertEqual(supervisor.fabric_child_terminal(child, wait=self.wait(child, 256)), 1)
        first = child._sirsi_terminal_receipt
        self.assertEqual(self.observe(child, owned, 256), 1)
        self.assertIs(child._sirsi_terminal_receipt, first)
        self.assertEqual(first.raw_status, 256)

    def test_later_narrow_or_different_status_refuses(self):
        child, owned = self.child()
        self.observe(child, owned, 256)
        for raw in (0, 1, 258):
            with self.subTest(raw=raw), self.assertRaises(supervisor.LocalWaitRefused):
                self.observe(child, owned, raw)
        self.assertEqual(child._sirsi_terminal_receipt.raw_status, 256)

    def test_empty_after_terminal_and_lost_wait_custody_refuse(self):
        child, owned = self.child()
        empty = lambda *unused: 0
        self.assertIsNone(supervisor._nonreaping_child_terminal(child, owned, wait=empty))
        self.observe(child, owned, 256)
        with self.assertRaises(supervisor.LocalWaitRefused):
            supervisor._nonreaping_child_terminal(child, owned, wait=empty)
        def lost(*unused):
            ctypes.set_errno(errno.ECHILD)
            return -1
        with self.assertRaises(supervisor.LocalWaitRefused):
            supervisor._nonreaping_child_terminal(child, owned, wait=lost)

    def test_unknown_negative_foreign_and_malformed_observations_refuse(self):
        bad = [(1, -1, {}), (1, 0x01000000, {}), (4, 0, {}),
               (2, 0, {}), (2, signal.NSIG, {}), (1, 0, {'pid': 424243}),
               (1, 0, {'uid': os.getuid() + 1}), (1, 0, {'signo': 0}),
               (1, 0, {'error': 1})]
        for code, raw, fields in bad:
            with self.subTest(code=code, raw=raw, fields=fields):
                child, owned = self.child()
                with self.assertRaises(supervisor.LocalWaitRefused):
                    self.observe(child, owned, raw, code, **fields)
                self.assertFalse(hasattr(child, '_sirsi_terminal_receipt'))

    def test_reap_binding_cannot_be_borrowed_by_replaced_identity(self):
        child, owned = self.child()
        self.observe(child, owned, 256)
        child._sirsi_fabric_identity = SimpleNamespace(handle=child, pid=child.pid)
        with self.assertRaises(supervisor.Refused):
            supervisor._terminal_expected_reap(child, 1)

    def test_fabric_cleanup_compares_reap_not_portable_result(self):
        for returned, accepted in [(0, True), (7, False)]:
            with self.subTest(returned=returned):
                child, owned = self.child()
                snapshot = lambda group: [dict(pid=child.pid, ppid=os.getpid(),
                    pgid=child.pid, uid=os.getuid(), status='Z')]
                def reap(target):
                    target.returncode = returned
                    return returned
                def stop():
                    supervisor.stop(child, observe=lambda c: self.observe(c, owned, 256),
                        snapshot_group=snapshot, reap=reap,
                        send_group=lambda *args: self.fail('No synthetic signal expected'))
                if accepted:
                    stop()
                    self.assertTrue(owned.reaped)
                    self.assertEqual(child.returncode, 0)
                    self.assertEqual(child._sirsi_terminal_receipt.portable_exit, 1)
                else:
                    with self.assertRaises(supervisor.Refused):
                        stop()
                    self.assertFalse(owned.reaped)

    def test_non_darwin_and_boolean_terminal_values_refuse(self):
        for platform, code, raw in [('linux', 1, 256), ('darwin', True, 0),
                                     ('darwin', 1, True)]:
            with self.subTest(platform=platform, code=code, raw=raw):
                child, owned = self.child()
                with patch.object(supervisor.sys, 'platform', platform), \
                     self.assertRaises(supervisor.LocalWaitRefused):
                    supervisor._latch_child_terminal(child, owned, code, raw, {})
                self.assertFalse(hasattr(child, '_sirsi_terminal_receipt'))

    def test_binding_refusal_never_marks_reap_started(self):
        child, owned = self.child()
        snapshot = lambda group: [dict(pid=child.pid, ppid=os.getpid(),
            pgid=child.pid, uid=os.getuid(), status='Z')]
        with patch.object(supervisor, '_terminal_expected_reap',
                          side_effect=supervisor.Refused('synthetic binding refusal')), \
             self.assertRaises(supervisor.Refused):
            supervisor.stop(child, observe=lambda c: self.observe(c, owned, 256),
                snapshot_group=snapshot,
                reap=lambda c: self.fail('Refusal must happen before consuming wait'),
                send_group=lambda *args: self.fail('No synthetic signal expected'))
        self.assertFalse(owned.reap_started)
        self.assertFalse(owned.reaped)
        self.assertIsNone(child.returncode)

    def test_matching_reap_return_with_wrong_returncode_refuses(self):
        child, owned = self.child()
        snapshot = lambda group: [dict(pid=child.pid, ppid=os.getpid(),
            pgid=child.pid, uid=os.getuid(), status='Z')]
        def inconsistent_reap(target):
            target.returncode = 7
            return 0
        with self.assertRaises(supervisor.Refused):
            supervisor.stop(child, observe=lambda c: self.observe(c, owned, 256),
                snapshot_group=snapshot, reap=inconsistent_reap,
                send_group=lambda *args: self.fail('No synthetic signal expected'))
        self.assertFalse(owned.reaped)

    def test_genuine_portable_binding_mismatch_never_marks_reap_started(self):
        child, owned = self.child()
        snapshot = lambda group: [dict(pid=child.pid, ppid=os.getpid(),
            pgid=child.pid, uid=os.getuid(), status='Z')]
        def inconsistent_observer(target):
            # Real decoder/receipt and real binding validator. Only the
            # observer's returned portable value is inconsistent here.
            self.assertEqual(self.observe(target, owned, 256), 1)
            return 2
        with self.assertRaisesRegex(supervisor.Refused, 'terminal/reap binding differs'):
            supervisor.stop(child, observe=inconsistent_observer,
                snapshot_group=snapshot,
                reap=lambda c: self.fail('Mismatch must precede consuming wait'),
                send_group=lambda *args: self.fail('No signal expected'))
        self.assertEqual(child._sirsi_terminal_receipt.portable_exit, 1)
        self.assertFalse(owned.reap_started)
        self.assertFalse(owned.reaped)
        self.assertTrue(owned.signals_closed)
        self.assertIsNone(child.returncode)

    def test_local_matching_reap_return_with_wrong_returncode_refuses(self):
        child, owned = self.child(local=True)
        original_observe = supervisor.local_child_terminal
        def inconsistent_reap(timeout):
            child.returncode = 7
            return 0
        child.wait = inconsistent_reap
        with patch.object(supervisor, 'local_group_snapshot', return_value=[
                 dict(pid=child.pid, uid=os.getuid(), status='Z')]), \
             patch.object(supervisor, 'local_anchor_matches', return_value=True), \
             patch.object(supervisor, 'local_child_terminal', side_effect=lambda c: original_observe(c, wait=self.wait(c, 256))), \
             self.assertRaises(supervisor.Refused):
            supervisor.local_stop_owned(child, owned.start)

    def test_local_full_record_preserves_raw_failure_and_zero_reap(self):
        child, owned = self.child(local=True)
        original_observe = supervisor.local_child_terminal
        snapshots, waits = [], []
        rows = iter([[dict(pid=child.pid, uid=os.getuid(), status='Z')], []])
        def reap(timeout):
            waits.append(timeout)
            child.returncode = 0
            return 0
        child.wait = reap
        with tempfile.TemporaryDirectory(prefix='terminal-record-') as raw:
            root = Path(raw)
            root.chmod(0o700)
            folder = root / 'fixture-execution'
            folder.mkdir(mode=0o700)
            record = {}
            def persist():
                snapshots.append(json.loads(json.dumps(record)))
            with patch.object(supervisor.subprocess, 'Popen', return_value=child), \
                 patch.object(supervisor, 'process_identity', return_value=(os.getpid(), owned.start)), \
                 patch.object(supervisor, 'local_bind_owned_child'), \
                 patch.object(supervisor, 'release_local_child'), \
                 patch.object(supervisor, 'local_group_snapshot', side_effect=lambda group: next(rows)), \
                 patch.object(supervisor, 'local_anchor_matches', return_value=True), \
                 patch.object(supervisor, 'local_child_terminal', side_effect=lambda c: original_observe(c, wait=self.wait(c, 256))), \
                 patch.object(supervisor.os, 'killpg', side_effect=AssertionError('No real signal')):
                result = supervisor.supervise_local(['/owned/inert-payload'], 10,
                                                    record, folder, persist)
            self.assertEqual(result, 1)
            self.assertEqual(record['status'], 'child_failed')
            self.assertEqual(record['child_exit_code'], 1)
            self.assertEqual(record['exit_code'], 1)
            self.assertEqual(record['child_terminal_observation']['raw_status'], 256)
            self.assertEqual(record['child_terminal_observation']['expected_reap_exit'], 0)
            self.assertEqual(snapshots[-1], record)
            self.assertEqual(child.returncode, 0)
            self.assertEqual(len(waits), 1)
            self.assertNotIn('cleanup_debt', record)
            self.assertEqual((root / '.local-cpu.lock').read_bytes(), b'')

    def assert_native_full_record(self, raw_status, portable, expected_reap):
        child, owned = self.child()
        backend = supervisor.Native()  # Constructor only; all control calls inert.
        backend.cpu_cores, backend.requested_regime = 1, 'build'
        backend.executor_thread = 'synthetic-executor-not-a-live-thread'
        backend.call = lambda *args, **kwargs: self.fail('No real control call')
        row = dict(id='synthetic-not-a-real-admission', resource='m5',
                   holder='fixture-lane', work='fixture-work', status='active',
                   regime='build', cores=1, share='full', exempt_pid=os.getpid(),
                   heartbeat_at=supervisor.dt.datetime.now(supervisor.dt.timezone.utc).isoformat(),
                   lease_ttl_sec=120)
        releases, reaps = [], []
        backend.registered = lambda *args: None
        backend.reserve = lambda *args: dict(granted=True, reservation=dict(row))
        backend.lookup = lambda *args: dict(row)
        backend.release = lambda identity, **kwargs: releases.append((identity, kwargs))
        original_observe = supervisor.fabric_child_terminal
        def reap(timeout):
            reaps.append(timeout)
            child.returncode = expected_reap
            return expected_reap
        child.wait = reap
        record = {}
        with patch.object(supervisor.subprocess, 'Popen', return_value=child), \
             patch.object(supervisor, 'fabric_bind_child', return_value=owned), \
             patch.object(supervisor, 'release_fabric_child'), \
             patch.object(supervisor, 'fabric_child_terminal', side_effect=lambda c: original_observe(c, wait=self.wait(c, raw_status))), \
             patch.object(supervisor, 'native_group_snapshot', return_value=[
                 dict(pid=child.pid, ppid=os.getpid(), pgid=child.pid, uid=os.getuid(), status='Z')]), \
             patch.object(supervisor.os, 'killpg', side_effect=AssertionError('No real signal')):
            result = supervisor.supervise(['/owned/inert-payload'], 'm5',
                'fixture-lane', 'synthetic-thread', 'fixture-work', 10, backend, record)
        self.assertEqual(result, portable)
        self.assertEqual(record['status'], 'child_failed')
        self.assertEqual(record['exit_code'], portable)  # Native's existing result field.
        self.assertEqual(record['child_terminal_observation']['raw_status'], raw_status)
        self.assertEqual(record['child_terminal_observation']['portable_exit'], portable)
        self.assertEqual(record['child_terminal_observation']['expected_reap_exit'], expected_reap)
        self.assertEqual(releases, [(row['id'], {'exit_code': portable})])
        self.assertEqual(child.returncode, expected_reap)
        self.assertEqual(len(reaps), 1)
        self.assertNotIn('cleanup_debt', record)

    def test_native_full_record_and_release_argument_preserve_raw_failure(self):
        self.assert_native_full_record(256, 1, 0)

    def test_native_full_record_status_258_reports_and_releases_two(self):
        # Original refusal's nonzero low-byte branch, through the complete
        # Native supervise path. The backend itself remains an inert fixture.
        self.assert_native_full_record(258, 2, 2)

    def test_local_cleanup_uses_separate_reap_expectation(self):
        child, owned = self.child(local=True)
        observations = iter([[dict(pid=child.pid, uid=os.getuid(), status='Z')], []])
        original_observe = supervisor.local_child_terminal
        def reap(timeout):
            child.returncode = 0
            return 0
        child.wait = reap
        with patch.object(supervisor, 'local_group_snapshot', side_effect=lambda group: next(observations)), \
             patch.object(supervisor, 'local_anchor_matches', return_value=True), \
             patch.object(supervisor, 'local_child_terminal', side_effect=lambda c: original_observe(c, wait=self.wait(c, 256))):
            supervisor.local_stop_owned(child, owned.start)
        self.assertEqual(child.returncode, 0)
        self.assertEqual(supervisor._child_terminal_evidence(child)['portable_exit'], 1)


if __name__ == '__main__':
    unittest.main(verbosity=2)
