import datetime as dt
import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('runner', Path(__file__).with_name('admitted-run.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


def admission(**updates):
    row = dict(id='fixture-id', resource='m5', holder='lane', work='work', status='active',
               share='full', heartbeat_at=dt.datetime.now(dt.timezone.utc).isoformat(), lease_ttl_sec=120)
    row.update(updates)
    return row


class Backend:
    def __init__(self, row):
        self.row = row
        self.released = []
        self.reserve_calls = 0
    def registered(self, agent, thread):
        if (agent, thread) != ('lane', 'thread'):
            raise runner.Refused('Unregistered fixture')
    def lookup(self, resource):
        return self.row
    def reserve(self, resource, agent, work):
        self.reserve_calls += 1
        self.row = admission()
        return dict(granted=True, reservation=self.row)
    def heartbeat(self, identity):
        pass
    def release(self, identity):
        self.released.append(identity)


class RegistryDeadlineTests(unittest.TestCase):
    def test_executor_registration_is_bound_to_this_process_not_origin_pid(self):
        for agent in ['lane-from-m5', 'lane-from-m1', 'lane-from-studio']:
            native = runner.Native()
            native.call = mock.Mock(side_effect=lambda *args, **kwargs: dict(agent_id=agent, pid=os.getpid(),
                                    status='active', thread_id=args[args.index('--thread')+1]))
            self.assertTrue(native.register_executor(agent).startswith('thr-'))
            args = native.call.call_args.args
            self.assertEqual(args[args.index('--anchor-pid')+1], str(os.getpid()))
            self.assertNotIn('--consumer-capable', args)
            native.close_executor()
            self.assertEqual(native.call.call_args.args,
                             ('thread','close','--thread',native.executor_thread,'--json'))

    def test_executor_registration_rejects_wrong_pid_lane_and_skipped(self):
        for row in [dict(agent_id='other',pid=os.getpid(),status='active',thread_id='thr-exec'),
                    dict(agent_id='lane',pid=1,status='active',thread_id='thr-exec'),
                    dict(agent_id='lane',pid=os.getpid(),status='closed',thread_id='thr-exec'),
                    dict(skipped='one-shot')]:
            native = runner.Native()
            native.call = mock.Mock(return_value=row)
            with self.assertRaises(runner.Refused):
                native.register_executor('lane')

    def test_executor_heartbeat_does_not_renew_borrowed_admission(self):
        native = runner.Native()
        native.executor_thread = 'thr-exec'
        native.call = mock.Mock()
        native.heartbeat_executor()
        native.call.assert_called_once_with('thread','heartbeat','--thread','thr-exec','--json',timeout=30,json_result=False)

    def test_metadata_reads_have_a_bounded_deadline(self):
        native = runner.Native()
        with mock.patch.object(runner.subprocess, 'run', return_value=mock.Mock(stdout='{}')) as run:
            with self.assertRaises(runner.Refused):
                native.registered('lane', 'thread')
            self.assertEqual(run.call_args.kwargs['timeout'], 30)
            self.assertEqual(run.call_args.args[0][1:], ['thread', 'heartbeat', '--thread', 'thread', '--json'])
            native.lookup('m5')
            self.assertEqual(run.call_args.kwargs['timeout'], 60)

    def test_destination_identity_is_checked_before_heartbeat(self):
        native = runner.Native()
        native.executor_identity = dict(thread_id='thread', agent_id='lane', status='active',pid=2,start_time='fixture')
        native.call = mock.Mock()
        with mock.patch.object(runner.subprocess, 'check_output', side_effect=['1','fixture']):
            with self.assertRaises(runner.Refused):
                native.registered('lane','thread')
        native.call.assert_not_called()

    def test_changed_start_time_refuses_before_heartbeat(self):
        native = runner.Native()
        native.executor_identity = dict(thread_id='thread',agent_id='lane',status='active',pid=os.getpid(),start_time='old')
        native.call = mock.Mock()
        with mock.patch.object(runner.subprocess, 'check_output', side_effect=['1','new']):
            with self.assertRaises(runner.Refused):
                native.registered('lane','thread')
        native.call.assert_not_called()

    def test_plain_close_reply_is_not_json_parsed(self):
        native = runner.Native()
        native.executor_thread='thr-exec'
        with mock.patch.object(runner.subprocess,'run',return_value=mock.Mock(stdout='Closed thread thr-exec')):
            native.close_executor()

    def test_registry_timeout_never_authorizes_or_uses_a_cache(self):
        native = runner.Native()
        with mock.patch.object(runner.subprocess, 'run', side_effect=subprocess.TimeoutExpired('sirsi',30)), \
                mock.patch.object(runner.subprocess, 'check_output') as process:
            with self.assertRaises(subprocess.TimeoutExpired):
                native.registered('lane','thread')
            process.assert_not_called()


class SupervisorTests(unittest.TestCase):
    def run_case(self, backend, command=None, timeout=2, agent='lane', thread='thread'):
        record = {}
        code = runner.supervise(command or [sys.executable, '-c', 'pass'], 'm5', agent, thread,
                                'work', timeout, backend, record)
        return code, record

    def test_borrowed_admission_success_does_not_release_others_hold(self):
        backend = Backend(admission())
        code, record = self.run_case(backend)
        self.assertEqual(code, 0)
        self.assertTrue(record['borrowed'])
        self.assertEqual(backend.released, [])

    def test_own_admission_is_acquired_and_released(self):
        backend = Backend(None)
        code, record = self.run_case(backend)
        self.assertEqual(code, 0)
        self.assertFalse(record['borrowed'])
        self.assertEqual(backend.released, ['fixture-id'])

    def test_wrong_or_expired_or_error_receipt_never_spawns(self):
        for row in [None, 'Error: no reservation', {'error':'not granted'},
                    admission(holder='foreign'), admission(resource='studio'),
                    admission(work='different'), admission(status='released'), admission(share='floor'),
                    admission(lease_ttl_sec=True), admission(heartbeat_at='invalid'),
                    admission(heartbeat_at=(dt.datetime.now(dt.timezone.utc)-dt.timedelta(hours=1)).isoformat())]:
            if row is None:
                continue
            with self.subTest(row=row), mock.patch.object(runner.subprocess, 'Popen') as spawn:
                self.assertEqual(self.run_case(Backend(row))[0], 78)
                spawn.assert_not_called()

    def test_registration_required_before_spawn_or_reserve(self):
        backend = Backend(None)
        with mock.patch.object(runner.subprocess, 'Popen') as spawn:
            self.assertEqual(self.run_case(backend, agent='other')[0], 78)
            spawn.assert_not_called()
        self.assertEqual(backend.reserve_calls, 0)

    def test_invalid_floor_grant_is_released_without_spawn(self):
        backend = Backend(None)
        def reserve(*args):
            backend.row = admission(share='floor')
            return dict(granted=True, reservation=backend.row)
        backend.reserve = reserve
        with mock.patch.object(runner.subprocess, 'Popen') as spawn:
            self.assertEqual(self.run_case(backend)[0], 78)
            spawn.assert_not_called()
        self.assertEqual(backend.released, ['fixture-id'])

    def test_explicit_denial_does_not_become_admission(self):
        backend = Backend(None)
        backend.reserve = lambda *args: {'granted':False, 'error':'fixture'}
        with mock.patch.object(runner.subprocess, 'Popen') as spawn:
            self.assertEqual(self.run_case(backend)[0], 78)
            spawn.assert_not_called()

    def test_timeout_reaps_only_owned_harmless_child(self):
        code, record = self.run_case(Backend(admission()), [sys.executable, '-c', 'import time;time.sleep(60)'], timeout=.05)
        self.assertEqual(code, 78)
        with self.assertRaises(ProcessLookupError):
            os.kill(record['pid'], 0)

    def test_admission_loss_stops_child(self):
        backend = Backend(admission())
        calls = []
        def lookup(resource):
            calls.append(resource)
            return backend.row if len(calls) < 3 else None
        backend.lookup = lookup
        code, record = self.run_case(backend, [sys.executable, '-c', 'import time;time.sleep(60)'])
        self.assertEqual(code, 78)
        with self.assertRaises(ProcessLookupError):
            os.kill(record['pid'], 0)

    def test_release_failure_has_explicit_debt(self):
        backend = Backend(None)
        backend.release = mock.Mock(side_effect=RuntimeError('fixture offline'))
        code, record = self.run_case(backend)
        self.assertEqual(code, 78)
        self.assertIn('release unconfirmed', record['cleanup_debt'])

    def test_exec_failure_releases_own_hold(self):
        backend = Backend(None)
        code, record = self.run_case(backend, ['/nonexistent/fixture-program'])
        self.assertEqual(code, 78)
        self.assertEqual(backend.released, ['fixture-id'])

    def test_ambiguous_reserve_failure_records_debt_without_spawn(self):
        backend = Backend(None)
        backend.reserve = mock.Mock(side_effect=subprocess.TimeoutExpired('fixture', 8))
        with mock.patch.object(runner.subprocess, 'Popen') as spawn:
            code, record = self.run_case(backend)
            self.assertEqual(code, 78)
            self.assertIn('outcome unknown', record['cleanup_debt'])
            spawn.assert_not_called()

    def test_registered_foreign_thread_anchor_is_refused(self):
        native = runner.Native()
        native.call = lambda *args, **kwargs: dict(thread_id='thread', agent_id='lane', status='active',pid=2,start_time='fixture')
        with mock.patch.object(runner.subprocess, 'check_output', side_effect=['1', 'fixture']):
            with self.assertRaises(runner.Refused):
                native.registered('lane', 'thread')


if __name__ == '__main__':
    unittest.main()
