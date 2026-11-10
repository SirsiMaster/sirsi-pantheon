"""SOURCE ONLY / ALL UNRUN: c406 recovery + bcaf observed-outcome join.

All control calls/children/descriptors/signals are mocked. This is a NEW tuple,
not qualified by the prior c406 ten tests or native8829 model-control receipt.
"""
from contextlib import ExitStack
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import types
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('observed_union', Path(__file__).with_name('admitted-run.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


def consumed(method='MaatMutate', cause='context deadline exceeded'):
    return ('routerstore: remote ' + method + ': Post "http://spool/v1/call/' + method +
            '": spool: ' + method + ' id 1794259726844-1-2f176a4a: OUTCOME UNKNOWN — '
            'a relay consumed the request but no response arrived (' + cause +
            '); re-query before retrying a mutation')


class Clock:
    def __init__(self):
        self.now = 0

    def monotonic(self):
        return self.now

    def sleep(self, seconds):
        self.now += seconds


class ObservedOutcomeJoinControls(unittest.TestCase):
    def setUp(self):
        self.clock = Clock()
        for patch in (
                mock.patch.object(runner.time, 'monotonic', self.clock.monotonic),
                mock.patch.object(runner.time, 'sleep', self.clock.sleep),
                mock.patch.object(runner.subprocess, 'Popen', side_effect=AssertionError('no real child')),
                mock.patch.object(runner.subprocess, 'run', side_effect=AssertionError('no real CLI'))):
            patch.start()
            self.addCleanup(patch.stop)

    def native(self):
        native = runner.Native()
        native.owned_admission = 'owned-union-fixture'
        native.release_scope = dict(id=native.owned_admission, resource='m5', holder='lane', work='work')
        native.deadline = 20
        return native

    def row(self, native, status='released', **changes):
        return dict(native.release_scope, status=status, **changes)

    def failure(self, text=None):
        return subprocess.CalledProcessError(1, 'inert-fixture', stderr=consumed() if text is None else text)

    def test_literal_owned_release_vectors_only(self):
        native = self.native()
        for outcome in ('0', '17', '-9', str(-(1 << 63)), str((1 << 63) - 1)):
            self.assertTrue(native.exact_owned_release_control(
                ('maat', 'release', native.owned_admission, '--exit-code', outcome, '--json')))
        self.assertTrue(native.exact_owned_release_control(('maat', 'release', native.owned_admission, '--json')))
        invalid = (
            ('maat', 'release', 'foreign', '--exit-code', '0', '--json'),
            ('maat', 'release', native.owned_admission, '--exit-code', '0', '--json', 'extra'),
            ('maat', 'release', native.owned_admission, '--json', '--exit-code', '0'),
            ('maat', 'release', native.owned_admission, '--exit-code=0', '--json'),
        )
        for vector in invalid:
            self.assertFalse(native.exact_owned_release_control(vector))
        for value in ('', '00', '-0', '+1', '1.0', 'true', ' 0', '0\n', str(1 << 63), str(-(1 << 63) - 1)):
            self.assertFalse(native.exact_owned_release_control(
                ('maat', 'release', native.owned_admission, '--exit-code', value, '--json')))
        native.release_scope['id'] = 'different'
        self.assertFalse(native.exact_owned_release_control(('maat', 'release', native.owned_admission, '--json')))

    def test_observed_zero_and_nonzero_first_release_preserve_exact_exit(self):
        for outcome in (0, 17, -9):
            native = self.native()
            with self.subTest(outcome=outcome), mock.patch.object(runner.subprocess, 'run', return_value=
                    mock.Mock(stdout=json.dumps(self.row(native, exit_code=outcome)))) as call:
                result = native.release(native.owned_admission, exit_code=outcome)
            self.assertEqual(result['exit_code'], outcome)
            self.assertEqual(call.call_count, 1)
            self.assertEqual(call.call_args.args[0], [native.cli, 'maat', 'release', native.owned_admission,
                                                     '--exit-code', str(outcome), '--json'])

    def test_missing_mismatched_bool_or_wrong_terminal_exit_never_backfilled(self):
        for changes in ({}, {'exit_code': None}, {'exit_code': False}, {'exit_code': '0'},
                        {'exit_code': 17}, {'exit_code': 0, 'status': 'expired'},
                        {'exit_code': 0, 'id': 'foreign'}):
            native = self.native()
            row = self.row(native)
            row.update(changes)
            with self.subTest(changes=changes), mock.patch.object(runner.subprocess, 'run',
                    return_value=mock.Mock(stdout=json.dumps(row))) as call:
                with self.assertRaisesRegex(runner.Refused, 'child exit outcome unconfirmed'):
                    native.release(native.owned_admission, exit_code=0)
            self.assertEqual(call.call_count, 1)
            self.assertEqual(row.get('exit_code'), changes.get('exit_code'))

    def test_noninteger_observed_exit_refuses_before_any_call(self):
        native = self.native()
        for value in (False, '0', 0.0, []):
            with self.subTest(value=value), mock.patch.object(runner.subprocess, 'run') as call:
                with self.assertRaisesRegex(runner.Refused, 'observed integer child exit'):
                    native.release(native.owned_admission, exit_code=value)
                call.assert_not_called()

    def test_observed_consumed_or_outer_timeout_active_then_released_one_write(self):
        for outcome in (0, 17):
            for cause in ('consumed', 'outer-timeout'):
                self.clock.now = 0
                native, calls = self.native(), []
                statuses = iter(('active', 'released'))
                def control(argv, **kwargs):
                    operation = tuple(argv[1:])
                    calls.append(operation)
                    if operation[:2] == ('maat', 'release'):
                        self.assertEqual(len(calls), 1, 'second mutation attempted')
                        self.assertEqual(operation, ('maat', 'release', native.owned_admission,
                                                     '--exit-code', str(outcome), '--json'))
                        if cause == 'outer-timeout':
                            raise subprocess.TimeoutExpired('inert-cli', 60)
                        raise self.failure()
                    self.assertEqual(operation, ('maat', 'status', '--id', native.owned_admission, '--json'))
                    status = next(statuses)
                    row = self.row(native, status)
                    if status == 'released':
                        row['exit_code'] = outcome
                    return mock.Mock(stdout=json.dumps([row]))
                with self.subTest(outcome=outcome, cause=cause), mock.patch.object(
                        runner.subprocess, 'run', side_effect=control):
                    result = native.release(native.owned_admission, exit_code=outcome)
                self.assertEqual(result['exit_code'], outcome)
                self.assertEqual(len(calls), 3)
                self.assertEqual(sum(op[:2] == ('maat', 'release') for op in calls), 1)
                self.assertEqual(native.deadline, 20)

    def test_uncertain_observed_terminal_requires_matching_exit_no_second_write(self):
        for terminal in ({}, {'exit_code': None}, {'exit_code': False}, {'exit_code': 3},
                         {'exit_code': 0, 'status': 'invalidated'}):
            native = self.native()
            row = self.row(native)
            row.update(terminal)
            with self.subTest(terminal=terminal), mock.patch.object(runner.subprocess, 'run', side_effect=[
                    self.failure(), mock.Mock(stdout=json.dumps([row]))]) as call:
                with self.assertRaisesRegex(runner.Refused, 'child exit outcome unconfirmed'):
                    native.release(native.owned_admission, exit_code=0)
            self.assertEqual(call.call_count, 2)
            self.assertEqual(tuple(call.call_args_list[-1].args[0][1:2]), ('maat',))
            self.assertEqual(call.call_args_list[-1].args[0][2], 'status')

    def test_observed_denial_cancel_or_malformed_vector_does_not_gain_recovery(self):
        for kind in ('denied', 'truncated-denial', 'canceled', 'foreign', 'malformed-exit'):
            native = self.native()
            text = consumed()
            args = ('maat', 'release', native.owned_admission, '--exit-code', '0', '--json')
            if kind == 'denied': text = 'HTTP403 scope denied\n' + text
            if kind == 'truncated-denial': text = "caller's session does not own this lease\n" + 'x'*1800 + text
            if kind == 'canceled': text = consumed(cause='context canceled')
            if kind == 'foreign': args = ('maat', 'release', 'foreign', '--exit-code', '0', '--json')
            if kind == 'malformed-exit': args = ('maat', 'release', native.owned_admission, '--exit-code', '00', '--json')
            with self.subTest(kind=kind), mock.patch.object(runner.subprocess, 'run', side_effect=self.failure(text)) as call:
                with self.assertRaises(runner.Refused) as caught:
                    native.call(*args)
                self.assertNotIsInstance(caught.exception, runner.TransientTransport)
            self.assertEqual(call.call_count, 1)

    def test_observed_late_release_cannot_extend_bound_or_enter_recovery(self):
        native = self.native()
        native.deadline = 2
        def late(*args, **kwargs):
            self.clock.now = 3
            raise self.failure()
        with mock.patch.object(runner.subprocess, 'run', side_effect=late) as call:
            with self.assertRaisesRegex(runner.Refused, 'Proven admission deadline exceeded') as caught:
                native.release(native.owned_admission, exit_code=0)
        self.assertNotIsInstance(caught.exception, runner.TransientTransport)
        self.assertEqual(call.call_count, 1)
        self.assertEqual(native.deadline, 2)

    def supervise_fixture(self, child_exit=0, cleanup_exit=0, final_valid=True, fail_before_spawn=False):
        """Actual Native class + actual supervisor/lease/settlement logic; all edges inert."""
        native = runner.Native()
        native.executor_thread = 'fixture-not-a-live-thread'
        native.cpu_cores, native.requested_regime = 1, 'build'
        row = dict(id='owned-supervisor-fixture', resource='m5', holder='lane', work='work',
                   status='active', share='full', regime='build', cores=1,
                   exempt_pid=os.getpid(), heartbeat_at=dt.datetime.now(dt.timezone.utc).isoformat(),
                   lease_ttl_sec=120)
        child = types.SimpleNamespace(pid=424242, returncode=None)
        record, releases = {}, []
        observations = iter((row, row if final_valid else dict(row, status='released')))
        def control(argv, **kwargs):
            self.assertEqual(argv[1:3], ['maat', 'release'])
            releases.append(tuple(argv[1:]))
            reply = dict(row, status='released')
            if '--exit-code' in argv:
                reply['exit_code'] = int(argv[argv.index('--exit-code') + 1])
            return mock.Mock(stdout=json.dumps(reply))
        def cleanup(_child):
            _child.returncode = cleanup_exit
        with ExitStack() as patches:
            for obj, name, values in (
                    (native, 'registered', {'side_effect': [None, runner.Refused('inert prespawn refusal')]
                                                           if fail_before_spawn else None}),
                    (native, 'reserve', {'return_value': dict(granted=True, reservation=row)}),
                    (native, 'lookup', {'side_effect': lambda _resource: next(observations)}),
                    (runner.os, 'pipe', {'return_value': (80, 81)}),
                    (runner.os, 'close', {}),
                    (runner.subprocess, 'Popen', {'return_value': child}),
                    (runner.subprocess, 'run', {'side_effect': control}),
                    (runner, 'fabric_gate_environment', {'return_value': {}}),
                    (runner, 'fabric_bind_child', {}),
                    (runner, 'release_fabric_child', {}),
                    (runner, 'fabric_wait_terminal', {'return_value': child_exit}),
                    (runner, 'stop', {'side_effect': cleanup}),
                    (runner, 'wait_unreleased_fabric_child', {'side_effect': cleanup}),
                    (runner, 'retain_fabric_cleanup_evidence', {})):
                patches.enter_context(mock.patch.object(obj, name, **values))
            result = runner.supervise(['INERT-payload'], 'm5', 'lane', 'fixture-not-a-live-thread',
                                      'work', 30, native, record)
        return result, record, releases

    def test_supervisor_observed_zero_nonzero_final_cleanup_matching_exit(self):
        for outcome in (0, 17):
            with self.subTest(outcome=outcome):
                result, record, releases = self.supervise_fixture(outcome, outcome)
            self.assertEqual(result, outcome)
            self.assertEqual(record['status'], 'success' if outcome == 0 else 'child_failed')
            self.assertEqual(releases, [('maat', 'release', 'owned-supervisor-fixture',
                                         '--exit-code', str(outcome), '--json')])

    def test_supervisor_cleanup_mismatch_withholds_release_and_keeps_debt(self):
        result, record, releases = self.supervise_fixture(0, 17)
        self.assertEqual(result, 78)
        self.assertTrue(record['child_cleanup_unconfirmed'])
        self.assertIn('Child cleanup exit receipt differs', record['cleanup_debt'])
        self.assertEqual(releases, [])

    def test_supervisor_final_admission_or_prespawn_failure_never_sends_exit_zero(self):
        for options in ({'final_valid': False}, {'fail_before_spawn': True}, {'child_exit': None}):
            with self.subTest(options=options):
                result, record, releases = self.supervise_fixture(**options)
            self.assertEqual(result, 78)
            self.assertEqual(record['status'], 'refused_or_stopped')
            self.assertEqual(releases, [('maat', 'release', 'owned-supervisor-fixture', '--json')])


if __name__ == '__main__':
    unittest.main()
