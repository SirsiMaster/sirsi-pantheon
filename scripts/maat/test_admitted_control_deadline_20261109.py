"""SOURCE ONLY / ALL UNRUN. No live CLI, child, socket, lease or provider.

All control subprocesses and clocks are mocked. These are new-source oracles,
not a replay of the native incident or proof the loaded relay runs this source.
"""
import datetime as dt
import importlib.util
import json
from pathlib import Path
import subprocess
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    'caller_deadline_fixture', Path(__file__).with_name('admitted-run.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


def spool_deadline(method='MaatRead', cause='context deadline exceeded'):
    return ('routerstore: remote ' + method + ': Post "http://spool/v1/call/' + method +
            '": spool: ' + method + ' id 1794259694706-1-d50a22b6: OUTCOME UNKNOWN — '
            'a relay consumed the request but no response arrived (' + cause +
            '); re-query before retrying a mutation')


class Clock:
    def __init__(self):
        self.now = 0.0

    def monotonic(self):
        return self.now

    def sleep(self, seconds):
        self.now += seconds


class CallerDeadlineControls(unittest.TestCase):
    def setUp(self):
        self.clock = Clock()
        self.patches = [
            mock.patch.object(runner.time, 'monotonic', self.clock.monotonic),
            mock.patch.object(runner.time, 'sleep', self.clock.sleep),
            mock.patch.object(runner.subprocess, 'Popen', side_effect=AssertionError('no child allowed')),
            mock.patch.object(runner.subprocess, 'run', side_effect=AssertionError('unmocked CLI forbidden')),
        ]
        for patch in self.patches:
            patch.start()
            self.addCleanup(patch.stop)

    def native(self, deadline=20):
        native = runner.Native()
        native.owned_admission = 'owned-fixture'
        native.release_scope = {'id': 'owned-fixture', 'resource': 'm1',
                                'holder': 'lane', 'work': 'finite-work'}
        native.deadline = deadline
        return native

    def row(self, native, status='active'):
        return dict(native.release_scope, status=status)

    def failure(self, method='MaatRead', detail=None):
        return subprocess.CalledProcessError(1, 'private-cli-fixture',
                                             stderr=detail if detail is not None else spool_deadline(method))

    def test_exact_consumed_read_deadline_is_transient_not_immediate_refusal(self):
        native = self.native()
        for detail in (spool_deadline(), ' \x1b[31m𓁵\x1b[0m '+spool_deadline()+'\n'):
            with self.subTest(rendering=detail[:20]), \
                    mock.patch.object(runner.subprocess, 'run', side_effect=self.failure(detail=detail)) as call:
                with self.assertRaises(runner.TransientTransport):
                    native.lookup('m1')
                self.assertEqual(call.call_count, 1)  # classification never repeats a call
                self.assertEqual(call.call_args.args[0],
                                 [native.cli, 'maat', 'status', '--id', 'owned-fixture', '--json'])
                self.assertEqual(native.deadline, 20)

    def test_canceled_foreign_broad_unowned_and_bad_diagnostics_stay_refused(self):
        for kind in ('canceled', 'foreign', 'broad', 'unowned', 'no-scope', 'no-proof',
                     'extra-diagnostic', 'unusable-response', 'denied', 'truncated-denial'):
            with self.subTest(kind=kind):
                native = self.native()
                args = ('maat', 'status', '--id', 'owned-fixture', '--json')
                detail = spool_deadline()
                if kind == 'canceled': detail = spool_deadline(cause='context canceled')
                if kind == 'foreign': args = ('maat', 'status', '--id', 'foreign', '--json')
                if kind == 'broad': args = ('maat', 'who-is-on', 'm1', '--json')
                if kind == 'unowned': native.owned_admission = None
                if kind == 'no-scope': native.release_scope = {'id': 'different'}
                if kind == 'no-proof': native.deadline = None
                if kind == 'extra-diagnostic': detail += '\nother failure'
                if kind == 'unusable-response': detail = detail.replace('a relay consumed the request but no response arrived', 'response file unusable')
                if kind == 'denied': detail = 'HTTP 403 scope denied\n'+detail
                if kind == 'truncated-denial': detail = "caller's session does not own this lease\n"+'x'*1800+'\n'+detail
                with mock.patch.object(runner.subprocess, 'run', side_effect=self.failure(detail=detail)) as call:
                    with self.assertRaises(runner.Refused) as caught:
                        native.call(*args)
                    self.assertNotIsInstance(caught.exception, runner.TransientTransport)
                    self.assertEqual(call.call_count, 1)

    def test_no_command_after_proven_deadline(self):
        native = self.native(deadline=0)
        with mock.patch.object(runner.subprocess, 'run') as call:
            with self.assertRaises(runner.Refused):
                native.lookup('m1')
            call.assert_not_called()

    def test_late_active_reply_cannot_reopen_execution_rights(self):
        native = self.native(deadline=2)
        def late(*args, **kwargs):
            self.assertEqual(kwargs['timeout'], 2)
            self.clock.now = 3
            return mock.Mock(stdout=json.dumps([self.row(native)]))
        with mock.patch.object(runner.subprocess, 'run', side_effect=late):
            with self.assertRaisesRegex(runner.Refused, 'Proven admission deadline exceeded'):
                native.lookup('m1')
        self.assertEqual(native.deadline, 2)

    def test_late_error_or_timeout_does_not_enter_recovery_after_expiry(self):
        for failure in (self.failure(), subprocess.TimeoutExpired('fixture-cli', 2)):
            native = self.native(deadline=2)
            self.clock.now = 0
            def late(*args, **kwargs):
                self.clock.now = 3
                raise failure
            with self.subTest(error=type(failure).__name__), mock.patch.object(runner.subprocess, 'run', side_effect=late):
                with self.assertRaises(runner.Refused) as caught:
                    native.lookup('m1')
                self.assertNotIsInstance(caught.exception, runner.TransientTransport)
            self.assertEqual(native.deadline, 2)

    def test_exact_release_unknown_reads_outcome_without_repeating_write(self):
        for initial in (self.failure('MaatMutate'), subprocess.TimeoutExpired('fixture-cli', 60)):
            with self.subTest(initial=type(initial).__name__):
                self.clock.now = 0
                native = self.native(deadline=20)
                calls = []
                reads = iter(('active', 'released'))
                def control(argv, **kwargs):
                    operation = tuple(argv[1:])
                    calls.append(operation)
                    self.assertLessEqual(kwargs['timeout'], 20-self.clock.now)
                    if operation[:2] == ('maat', 'release'):
                        if sum(op[:2] == ('maat', 'release') for op in calls) != 1:
                            self.fail('mutation replay used for recovery')
                        raise initial
                    self.assertEqual(operation, ('maat', 'status', '--id', 'owned-fixture', '--json'))
                    return mock.Mock(stdout=json.dumps([self.row(native, next(reads))]))
                with mock.patch.object(runner.subprocess, 'run', side_effect=control):
                    row = native.release('owned-fixture')
                self.assertEqual(row['status'], 'released')
                self.assertNotIn('exit_code', row)  # release is not a fabricated successful test
                self.assertEqual(sum(op[:2] == ('maat', 'release') for op in calls), 1)
                self.assertEqual(len(calls), 3)
                self.assertEqual(native.deadline, 20)

    def test_active_release_readback_is_not_permission_to_retry_mutation(self):
        native = self.native()
        calls = []
        def control(argv, **kwargs):
            calls.append(tuple(argv[1:]))
            if tuple(argv[1:3]) == ('maat', 'release'):
                if len(calls) != 1: self.fail('release mutation was repeated')
                raise self.failure('MaatMutate')
            return mock.Mock(stdout=json.dumps([self.row(native)]))
        with mock.patch.object(runner.subprocess, 'run', side_effect=control):
            with self.assertRaises(runner.TransientTransport):
                native.release('owned-fixture')
        self.assertEqual(len(calls), 4)  # one write, at most three exact reads
        self.assertLess(self.clock.now, 20)
        self.assertEqual(native.deadline, 20)

    def test_cleanup_read_budget_cannot_extend_an_existing_shorter_bound(self):
        native = self.native(deadline=1)
        calls = []
        def control(argv, **kwargs):
            calls.append(tuple(argv[1:]))
            if len(calls) == 1: raise self.failure('MaatMutate')
            self.assertLessEqual(kwargs['timeout'], 1)
            return mock.Mock(stdout=json.dumps([self.row(native)]))
        with mock.patch.object(runner.subprocess, 'run', side_effect=control):
            with self.assertRaises(runner.TransientTransport):
                native.release('owned-fixture')
        self.assertEqual(len(calls), 2)
        self.assertEqual(native.deadline, 1)

    def test_changed_or_malformed_release_readback_refuses_no_replay(self):
        for payload in ('{}', '[]', '{bad', json.dumps([{'id': 'foreign', 'status': 'released'}]),
                        json.dumps([{'id': 'owned-fixture', 'resource': 'different', 'status': 'released'}])):
            native = self.native()
            calls = []
            def control(argv, **kwargs):
                calls.append(tuple(argv[1:]))
                if len(calls) == 1: raise self.failure('MaatMutate')
                return mock.Mock(stdout=payload)
            with self.subTest(payload=payload), mock.patch.object(runner.subprocess, 'run', side_effect=control):
                with self.assertRaises(runner.Refused):
                    native.release('owned-fixture')
            self.assertEqual(len(calls), 2)

    def test_readback_never_renews_proof_and_revoked_row_refuses(self):
        native = self.native(deadline=20)
        row = dict(native.release_scope, share='full', exempt_pid=123,
                   heartbeat_at=dt.datetime.now(dt.timezone.utc).isoformat(),
                   lease_ttl_sec=120, status='active')
        lease = runner.ProvenLease(row, 0, 'm1', 'lane', 'finite-work', None, False)
        original_deadline = lease.deadline
        with mock.patch.object(runner.subprocess, 'run', return_value=mock.Mock(stdout=json.dumps([row]))):
            observed = native.lookup('m1')
            lease.check(observed)
        self.assertEqual(lease.deadline, original_deadline)
        with self.assertRaises(runner.Refused):
            lease.check(dict(row, status='released'))
        self.assertEqual(lease.deadline, original_deadline)


if __name__ == '__main__':
    unittest.main()
