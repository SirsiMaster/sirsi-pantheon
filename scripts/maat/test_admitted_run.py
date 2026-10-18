import datetime as dt
import importlib.util
import json
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
               share='full', exempt_pid=os.getpid(), heartbeat_at=dt.datetime.now(dt.timezone.utc).isoformat(), lease_ttl_sec=120)
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
    def test_initial_registration_blip_recovers_the_same_identity_without_reregister(self):
        native = runner.Native()
        calls = []
        def call(*args, **kwargs):
            calls.append(args)
            if args[:2] == ('thread', 'register'):
                raise runner.TransientTransport('HTTP 502 OUTCOME UNKNOWN')
            return dict(agent_id='lane', pid=os.getpid(), status='active',
                        thread_id=args[args.index('--thread')+1])
        native.call = call
        identity = native.register_executor('lane')
        self.assertEqual([x[:2] for x in calls], [('thread', 'register'), ('thread', 'heartbeat')])
        self.assertEqual(calls[0][calls[0].index('--thread')+1], identity)
        self.assertEqual(calls[1][calls[1].index('--thread')+1], identity)

    def test_registration_observation_retries_only_transient_and_keeps_prior_bound(self):
        native = runner.Native()
        native.deadline = 75
        native.call = mock.Mock(side_effect=[runner.TransientTransport('HTTP 502'), {'thread_id': 'own'}])
        clock = Clock()
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.time, 'sleep', side_effect=lambda seconds: setattr(clock, 'now', clock.now + seconds)):
            self.assertEqual(native.registration_observation('own'), {'thread_id': 'own'})
        self.assertEqual(native.call.call_count, 2)
        self.assertEqual(native.deadline, 75)
        native.deadline = None
        native.call = mock.Mock(side_effect=runner.Refused('403 denied'))
        with self.assertRaises(runner.Refused):
            native.registration_observation('own')
        self.assertEqual(native.call.call_count, 1)

    def test_registration_observation_stops_at_bounded_budget(self):
        native = runner.Native()
        native.call = mock.Mock(side_effect=runner.TransientTransport('HTTP 502'))
        clock = Clock()
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.time, 'sleep', side_effect=lambda seconds: setattr(clock, 'now', clock.now + seconds)), \
                self.assertRaises(runner.TransientTransport):
            native.registration_observation('own')
        self.assertEqual(clock.now, 120)
        self.assertIsNone(native.deadline)

    def release_native(self):
        native = runner.Native()
        native.owned_admission = 'fixture-id'
        native.release_scope = {key: admission()[key] for key in
                                ('id', 'resource', 'holder', 'work', 'exempt_pid')}
        return native

    def test_unknown_release_committed_is_reconciled_without_duplicate_mutation(self):
        native = self.release_native()
        native.call = mock.Mock(side_effect=runner.TransientTransport('HTTP 502'))
        native.lookup = mock.Mock(return_value=admission(status='released'))
        self.assertEqual(native.release('fixture-id')['status'], 'released')
        self.assertEqual(native.call.call_count, 1)
        self.assertIsNone(native.deadline)

    def test_unknown_release_still_active_is_read_before_retry(self):
        native = self.release_native()
        events = []
        def call(*args, **kwargs):
            events.append('release')
            if len(events) == 1:
                raise runner.TransientTransport('HTTP 502')
            return admission(status='released')
        native.call = call
        native.lookup = lambda resource: events.append('read') or admission()
        self.assertEqual(native.release('fixture-id')['status'], 'released')
        self.assertEqual(events, ['release', 'read', 'release'])

    def test_unknown_release_does_not_touch_replacement_or_foreign_holder(self):
        for change in (dict(id='different'), dict(holder='foreign'), dict(exempt_pid=1)):
            native = self.release_native()
            native.call = mock.Mock(side_effect=runner.TransientTransport('HTTP 502'))
            native.lookup = mock.Mock(return_value=admission(**change))
            with self.assertRaisesRegex(runner.Refused, 'changed exact'):
                native.release('fixture-id')
            self.assertEqual(native.call.call_count, 1)
            self.assertIsNone(native.deadline)

    def test_unknown_release_unavailable_has_bounded_retries_and_remains_debt(self):
        native = self.release_native()
        native.call = mock.Mock(side_effect=runner.TransientTransport('HTTP 502'))
        native.lookup = mock.Mock(side_effect=runner.TransientTransport('HTTP 502'))
        with mock.patch.object(runner.time, 'sleep'), self.assertRaises(runner.TransientTransport):
            native.release('fixture-id')
        self.assertEqual(native.lookup.call_count, 3)
        self.assertEqual(native.call.call_count, 1)

    def test_release_explicit_denial_is_not_retried_or_reconciled(self):
        native = self.release_native()
        native.call = mock.Mock(side_effect=runner.Refused('HTTP 403 denied'))
        native.lookup = mock.Mock()
        with self.assertRaises(runner.Refused):
            native.release('fixture-id')
        native.lookup.assert_not_called()

    def test_executor_controls_cannot_publish_or_remove_parent_session_markers(self):
        inherited = dict(CLAUDE_CODE_SESSION_ID='parent-claude',
                         CODEX_SESSION_ID='parent-codex', CODEX_THREAD_ID='parent-codex',
                         SIRSI_THREAD_ID='thr-parent', SIRSI_AGENT_ID='lane')
        controls = []
        def control(argv, **kwargs):
            env = kwargs['env']
            controls.append((argv, dict(env)))
            if argv[1:3] == ['thread', 'register']:
                identity = argv[argv.index('--thread')+1]
                return mock.Mock(stdout=json.dumps(dict(agent_id='lane', pid=os.getpid(),
                                 status='active', thread_id=identity)))
            return mock.Mock(stdout='{}')
        with mock.patch.dict(os.environ, inherited), \
                mock.patch.object(runner.subprocess, 'run', side_effect=control):
            native = runner.Native()
            identity = native.register_executor('lane')
            native.close_executor()
            for key, value in inherited.items():
                self.assertEqual(os.environ[key], value)
        self.assertEqual(len(controls), 2)
        for argv, env in controls:
            for key in ('CLAUDE_CODE_SESSION_ID', 'CODEX_SESSION_ID', 'CODEX_THREAD_ID'):
                self.assertNotIn(key, env)
            self.assertEqual(env['SIRSI_AGENT_ID'], 'lane')
        self.assertNotIn('SIRSI_THREAD_ID', controls[0][1])
        self.assertEqual(controls[1][1]['SIRSI_THREAD_ID'], identity)

    def test_non_executor_control_keeps_original_session_resolution(self):
        with mock.patch.object(runner.subprocess, 'run', return_value=mock.Mock(stdout='{}')) as run:
            runner.Native().call('thread', 'heartbeat', '--thread', 'thr-parent')
        self.assertIsNone(run.call_args.kwargs['env'])

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
        native.executor_identity = dict(agent_id='lane', thread_id='thr-exec')
        native.verify_runtime = mock.Mock()
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
        with mock.patch.object(runner, 'process_identity', return_value=(1,'fixture')):
            with self.assertRaises(runner.Refused):
                native.registered('lane','thread')
        native.call.assert_not_called()

    def test_changed_start_time_refuses_before_heartbeat(self):
        native = runner.Native()
        native.executor_identity = dict(thread_id='thread',agent_id='lane',status='active',pid=os.getpid(),start_time='old')
        native.call = mock.Mock()
        with mock.patch.object(runner, 'process_identity', return_value=(1,'new')):
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
        clock = Clock()
        with mock.patch.object(runner.subprocess, 'run', side_effect=subprocess.TimeoutExpired('sirsi',30)), \
                mock.patch.object(runner.subprocess, 'check_output') as process, \
                mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.time, 'sleep', side_effect=lambda seconds: setattr(clock, 'now', clock.now + seconds)):
            with self.assertRaises(runner.TransientTransport):
                native.registered('lane','thread')
            process.assert_not_called()


class SupervisorTests(unittest.TestCase):
    def test_floor_requires_explicit_sufficient_cpu_build_mask(self):
        row = admission(share='floor', cores=4, regime='build')
        self.assertFalse(runner.valid(row,'m5','lane','work'))
        self.assertTrue(runner.valid(row,'m5','lane','work',2))
        self.assertFalse(runner.valid(row,'m5','lane','work',5))
        self.assertFalse(runner.valid(admission(share='floor',cores=4,regime='loaded'),'m5','lane','work',2))
        self.assertFalse(runner.valid(admission(share='floor',cores=True,regime='build'),'m5','lane','work',1))

    def test_cpu_build_floor_applies_explicit_child_environment(self):
        backend = Backend(admission(share='floor',cores=4,regime='build'))
        backend.cpu_cores = 2
        code, record = self.run_case(backend,command=[sys.executable,'-c',
            'import os; assert os.environ["GOMAXPROCS"] == "2"; assert os.environ["OMP_NUM_THREADS"] == "2"'])
        self.assertEqual(code,0)
        self.assertEqual(record['share'],'floor')

    def test_owned_lookup_reads_exact_grant_not_foreign_first_holder(self):
        native=runner.Native()
        native.owned_admission='own-grant'
        native.call=mock.Mock(return_value=[admission(id='foreign'), admission(id='own-grant')])
        self.assertEqual(native.lookup('m5')['id'], 'own-grant')
        native.call.assert_called_once_with('maat','status','m5','--json')

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

    def test_admission_loss_is_observed_at_bounded_remote_tick(self):
        backend = Backend(admission())
        clock = Clock()
        calls = []
        def lookup(resource):
            calls.append(clock.now)
            return backend.row if len(calls) < 3 else None
        backend.lookup = lookup
        record = {}
        child = FakeChild(clock, 1000)
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'stop') as stop:
            code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 120, backend, record)
        self.assertEqual(code, 78)
        self.assertEqual(calls, [0, 0, 30])
        self.assertIn('scope/executor lost', record['reason'])
        stop.assert_called_once_with(child)

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
        with mock.patch.object(runner, 'process_identity', return_value=(1, 'fixture')):
            with self.assertRaises(runner.Refused):
                native.registered('lane', 'thread')


class KernelIdentityTests(unittest.TestCase):
    def test_short_and_mismatched_kernel_reads_refuse(self):
        ctypes_size = runner.ctypes.sizeof(runner._BSDInfo)
        for count, observed_pid in [(0, 42), (1, 42), (ctypes_size, 43)]:
            def lookup(pid, flavor, arg, pointer, size):
                self.assertEqual((pid, flavor, arg, size), (42, 3, 0, ctypes_size))
                pointer._obj.pid = observed_pid
                pointer._obj.start_sec = 100
                return count
            library = mock.Mock()
            library.proc_pidinfo.side_effect = lookup
            with mock.patch.object(runner.sys, 'platform', 'darwin'), \
                    mock.patch.object(runner.ctypes, 'CDLL', return_value=library):
                with self.assertRaises(runner.Refused):
                    runner.process_identity(42)

    def row(self, **changes):
        row = dict(thread_id='thread', agent_id='lane', status='active',
                   pid=os.getpid(), start_time='fixture')
        row.update(changes)
        return row

    def test_self_and_real_ancestor_need_no_ps(self):
        for anchor in [os.getpid(), 42]:
            def lookup(pid):
                return (42, 'fixture') if pid == os.getpid() else (1, 'fixture')
            with mock.patch.object(runner, 'process_identity', side_effect=lookup), \
                    mock.patch.object(runner.subprocess, 'check_output', side_effect=PermissionError('ps denied')) as ps:
                runner.Native().verify_runtime(self.row(pid=anchor), 'lane', 'thread')
                ps.assert_not_called()

    def test_identity_failures_and_cycles_never_authorize(self):
        for lookup in [mock.Mock(side_effect=runner.Refused('kernel denied')),
                       mock.Mock(return_value=(os.getpid(), 'fixture')),
                       mock.Mock(return_value=(1, 'reused'))]:
            with mock.patch.object(runner, 'process_identity', lookup):
                with self.assertRaises(runner.Refused):
                    runner.Native().verify_runtime(self.row(pid=42), 'lane', 'thread')

    @unittest.skipUnless(sys.platform == 'darwin', 'Darwin kernel integration')
    def test_live_kernel_matches_parent_and_refuses_wrong_start(self):
        parent, start = runner.process_identity(os.getpid())
        self.assertEqual(parent, os.getppid())
        self.assertTrue(start)
        runner.Native().verify_runtime(self.row(start_time=start), 'lane', 'thread')
        with self.assertRaises(runner.Refused):
            runner.Native().verify_runtime(self.row(start_time='reused'), 'lane', 'thread')




class Clock:
    def __init__(self):
        self.now = 0
    def monotonic(self):
        return self.now


class FakeChild:
    pid = 424242
    def __init__(self, clock, finish):
        self.clock, self.finish = clock, finish
        self.wait_timeouts = []
    def poll(self):
        return 0 if self.clock.now >= self.finish else None
    def wait(self, timeout=None):
        self.wait_timeouts.append(timeout)
        if timeout is not None and self.clock.now + timeout < self.finish:
            self.clock.now += timeout
            raise subprocess.TimeoutExpired('fixture', timeout)
        self.clock.now = max(self.clock.now, self.finish)
        return 0


class RecurringTests(unittest.TestCase):
    def run_iterations(self, results, interval=60, lifetime=500, heartbeat_error=None):
        clock = Clock()
        backend = mock.Mock(executor_thread='thr-real')
        backend.heartbeat_executor.side_effect = heartbeat_error
        snapshots = []
        record = {}
        def iteration(*args):
            self.assertIsNone(backend.owned_admission)
            self.assertIsNone(backend.release_scope)
            current = args[-1]
            code = results.pop(0)
            current.update(status='success' if code == 0 else 'refused_or_stopped', exit_code=code)
            backend.owned_admission = 'previous-terminal-id'
            clock.now += 5
            return code
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.time, 'sleep', side_effect=lambda seconds: setattr(clock, 'now', clock.now + seconds)), \
                mock.patch.object(runner, 'supervise', side_effect=iteration) as supervise:
            code = runner.supervise_iterations(['fixture'], 'm5', 'lane', 'thr-real',
                                               'work', 60, backend, record, 3, interval, lifetime,
                                               lambda: snapshots.append(json.loads(json.dumps(record))))
        return code, record, backend, supervise, clock, snapshots

    def test_three_iterations_get_independent_grants_and_durable_results(self):
        code, record, backend, supervise, clock, snapshots = self.run_iterations([0, 0, 0])
        self.assertEqual(code, 0)
        self.assertEqual(supervise.call_count, 3)
        self.assertEqual(clock.now, 135)
        self.assertEqual(record['status'], 'success')
        self.assertEqual(len(snapshots), 3)
        self.assertEqual([x['iteration'] for x in record['iterations']], [1, 2, 3])
        self.assertEqual(backend.heartbeat_executor.call_count, 4)

    def test_first_failure_never_relaunches_failed_payload(self):
        code, record, backend, supervise, clock, snapshots = self.run_iterations([0, 78])
        self.assertEqual(code, 78)
        self.assertEqual(supervise.call_count, 2)
        self.assertEqual(record['status'], 'refused_or_stopped')

    def test_idle_job_budget_expires_without_next_admission(self):
        code, record, backend, supervise, clock, snapshots = self.run_iterations([0], lifetime=20)
        self.assertEqual(code, 78)
        self.assertEqual(supervise.call_count, 1)
        self.assertEqual(clock.now, 20)
        self.assertEqual(record['stop_cause'], 'job_timeout')

    def test_idle_transient_does_not_borrow_authority_or_prevent_fresh_admission(self):
        code, record, backend, supervise, clock, snapshots = self.run_iterations(
            [0, 0, 0], heartbeat_error=runner.TransientTransport('HTTP 502'))
        self.assertEqual(code, 0)
        self.assertIn('502', record['idle_transport_error'])
        self.assertEqual(supervise.call_count, 3)

    def test_idle_denial_stops_without_next_iteration(self):
        with self.assertRaisesRegex(runner.Refused, 'denied'):
            self.run_iterations([0], heartbeat_error=runner.Refused('HTTP 403 denied'))

    def test_recurring_requires_own_executor(self):
        with self.assertRaisesRegex(runner.Refused, 'own registered executor'):
            runner.supervise_iterations(['fixture'], 'm5', 'lane', 'thread', 'work', 60,
                                        Backend(None), {}, 2, 30, 120)

    def test_job_budget_includes_iteration_setup_before_child_spawn(self):
        backend = Backend(None)
        backend.job_deadline = 10
        clock = Clock()
        original = backend.registered
        def registered(*args):
            original(*args)
            clock.now += 6
        backend.registered = registered
        record = {}
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.subprocess, 'Popen') as spawn:
            code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 60, backend, record)
        self.assertEqual(code, 78)
        self.assertEqual(record['stop_cause'], 'job_timeout')
        spawn.assert_not_called()
        self.assertEqual(backend.released, ['fixture-id'])

    def test_job_limit_is_distinguished_from_per_child_timeout(self):
        lease = mock.Mock(deadline=240)
        record = {'execution_bound': 'job_timeout'}
        with mock.patch.object(runner.time, 'monotonic', return_value=20), \
                self.assertRaisesRegex(runner.Refused, 'total lifetime'):
            runner.check_deadlines(20, lease, 5, 60, record)
        self.assertEqual(record['stop_cause'], 'job_timeout')


class RecoveryTests(unittest.TestCase):
    def run_case(self, *, failure=None, restore=34, finish=45, renewal=None, ttl=120, timeout=300):
        clock = Clock()
        backend = Backend(None)
        backend.heartbeat_calls = 0
        events = []
        base = admission(lease_ttl_sec=ttl)
        def reserve(*args):
            backend.reserve_calls += 1
            backend.row = dict(base)
            events.append(('reserve', clock.now))
            return dict(granted=True, reservation=backend.row)
        def lookup(*args):
            events.append(('read', clock.now))
            if clock.now >= 30 and clock.now < restore and failure:
                raise failure
            return backend.row
        def heartbeat(identity):
            events.append(('renew', clock.now))
            backend.heartbeat_calls += 1
            if renewal:
                return renewal(backend, clock)
            return dict(backend.row)
        backend.reserve, backend.lookup, backend.heartbeat = reserve, lookup, heartbeat
        child = FakeChild(clock, finish)
        backend.child = child
        record = {}
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'stop') as stop:
            code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', timeout, backend, record)
            stop.assert_called_once_with(child)
        self.assertEqual(backend.reserve_calls, 1)
        self.assertEqual(backend.released, ['fixture-id'])
        return code, record, backend, clock, events

    def test_healthy_run_bounds_remote_reads_without_slowing_child_monitoring(self):
        code, record, backend, clock, events = self.run_case(finish=95)
        self.assertEqual(code, 0)
        reads = [at for kind,at in events if kind == 'read']
        renewals = [at for kind,at in events if kind == 'renew']
        # Initial observation + pre-spawn + three healthy checks + terminal.
        self.assertEqual(reads, [0, 0, 30, 60, 90, 95])
        self.assertEqual(renewals, [30, 60, 90])
        self.assertEqual(backend.reserve_calls, 1)
        self.assertGreater(len(backend.child.wait_timeouts), 40)
        self.assertTrue(all(0 < wait <= 2 for wait in backend.child.wait_timeouts))

    def test_three_minute_outage_recovers_only_with_sufficient_confirmed_grant(self):
        code, record, backend, clock, events = self.run_case(
            failure=runner.TransientTransport('MaatRead HTTP 502'),
            restore=210, finish=250, ttl=240)
        self.assertEqual(code, 0)
        self.assertGreater(record['transport_retries'], 10)
        self.assertEqual(backend.reserve_calls, 1)

    def test_long_transfer_renews_without_confusing_ttl_with_total_run_budget(self):
        code, record, backend, clock, events = self.run_case(
            finish=7000, timeout=7200, ttl=240)
        self.assertEqual(code, 0)
        self.assertGreater(backend.heartbeat_calls, 200)
        self.assertEqual(record['execution_timeout_sec'], 7200)
        self.assertEqual(record['execution_elapsed_sec'], 7000)
        self.assertNotIn('stop_cause', record)

    def test_configured_copy_timeouts_are_not_reported_as_expired_lease(self):
        for timeout in (900, 1700):
            with self.subTest(timeout=timeout):
                code, record, backend, clock, events = self.run_case(
                    finish=5000, timeout=timeout, ttl=240)
                self.assertEqual(code, 78)
                self.assertEqual(clock.now, timeout)
                self.assertEqual(record['stop_cause'], 'execution_timeout')
                self.assertGreater(record['proven_lease_remaining_sec'], 0)
                self.assertIn('--timeout '+str(timeout), record['reason'])

    def test_expired_proven_lease_has_its_own_terminal_cause(self):
        code, record, backend, clock, events = self.run_case(
            failure=runner.TransientTransport('HTTP 502'), restore=1000, finish=1000)
        self.assertEqual(code, 78)
        self.assertEqual(record['stop_cause'], 'proven_lease_expired')
        self.assertEqual(record['proven_lease_remaining_sec'], 0)

    def test_denied_and_insufficient_grants_preserve_actual_blocker_identity(self):
        for granted in (False, True):
            backend = Backend(None)
            conflict = admission(id='actual-blocker', holder='other', work='training')
            own = admission(share='floor', cores=1, regime='build')
            backend.reserve = lambda *args: dict(granted=granted, reservation=own, conflict=conflict,
                                                pending_cedes=['cede-123'])
            record = {}
            with mock.patch.object(runner.subprocess, 'Popen') as spawn:
                code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 30, backend, record)
            self.assertEqual(code, 78)
            spawn.assert_not_called()
            self.assertEqual(record['admission_decision']['conflict']['id'], 'actual-blocker')
            self.assertIn('actual-blocker', record['reason'])
            self.assertEqual(record['admission_decision']['pending_cedes'], ['cede-123'])

    def test_native_requests_explicit_recovery_ttl_without_inventing_a_grant(self):
        native = runner.Native()
        native.call = mock.Mock(return_value={'granted': False})
        self.assertEqual(native.reserve('m5', 'lane', 'work'), {'granted': False})
        args = native.call.call_args.args
        self.assertEqual(args[args.index('--lease-ttl')+1], '240')

    def test_transient_read_failures_recover_with_one_reservation(self):
        for reason in ['MaatRead HTTP 502', 'TLS handshake timeout', 'relay OUTCOME UNKNOWN']:
            with self.subTest(reason=reason):
                code, record, backend, clock, events = self.run_case(failure=runner.TransientTransport(reason))
                self.assertEqual(code, 0)
                self.assertGreater(record['transport_retries'], 0)
                self.assertGreater(backend.heartbeat_calls, 0)
                for i, event in enumerate(events):
                    if event[0] == 'renew':
                        self.assertEqual(events[i-1][0], 'read')

    def test_uncertain_mutation_is_not_replayed_until_readback(self):
        def renewal(backend, clock):
            if backend.heartbeat_calls == 1:
                raise runner.TransientTransport('MaatMutate HTTP 502 OUTCOME UNKNOWN')
            return dict(backend.row)
        code, record, backend, clock, events = self.run_case(renewal=renewal)
        self.assertEqual(code, 0)
        self.assertEqual(backend.heartbeat_calls, 2)
        first, second = [i for i,e in enumerate(events) if e[0]=='renew']
        self.assertTrue(any(e[0]=='read' for e in events[first+1:second]))
        self.assertGreater(events[second][1], events[first][1])

    def test_readback_outage_exhausts_original_ttl_without_renewal(self):
        code, record, backend, clock, events = self.run_case(
            failure=runner.TransientTransport('TLS handshake timeout'), restore=1000, finish=1000)
        self.assertEqual(code, 78)
        self.assertLessEqual(clock.now, 120)
        self.assertEqual(backend.heartbeat_calls, 0)

    def test_failed_renewal_never_extends_proven_deadline(self):
        def renewal(backend, clock):
            # Server MAY have renewed; no usable response means no extra time.
            raise runner.TransientTransport('relay OUTCOME UNKNOWN')
        code, record, backend, clock, events = self.run_case(renewal=renewal, finish=1000)
        self.assertEqual(code, 78)
        self.assertLessEqual(clock.now, 120)

    def test_denial_or_unclassified_failure_stops_immediately(self):
        for reason in ['HTTP 403 scope denied', 'released', 'revoked', 'unclassified failure']:
            code, record, backend, clock, events = self.run_case(failure=runner.Refused(reason), restore=1000, finish=1000)
            self.assertEqual(code, 78)
            self.assertEqual(clock.now, 30)
            self.assertEqual(backend.heartbeat_calls, 0)

    def test_mismatched_recovery_or_renewal_never_promotes(self):
        for changes in [dict(id='other'), dict(resource='studio'), dict(holder='other'),
                        dict(work='other'), dict(exempt_pid=1), dict(status='released'),
                        dict(status='expired'), dict(share='floor', cores=1, regime='build')]:
            def renewal(backend, clock):
                return dict(backend.row, **changes)
            code, record, backend, clock, events = self.run_case(renewal=renewal, finish=1000)
            self.assertEqual(code, 78)
            self.assertEqual(clock.now, 30)

    def test_recovery_readback_mismatch_stops_before_mutation(self):
        for changes in [dict(exempt_pid=1), dict(id='other'), dict(share='floor', cores=1, regime='build'),
                        dict(status='released'), dict(holder='other'), dict(work='other')]:
            clock = Clock()
            backend = Backend(None)
            events = []
            def reserve(*args):
                backend.row = admission()
                return dict(granted=True, reservation=backend.row)
            def lookup(*args):
                events.append(('read', clock.now))
                if clock.now == 30:
                    raise runner.TransientTransport('HTTP 502')
                return dict(backend.row, **changes) if clock.now > 30 else backend.row
            backend.reserve, backend.lookup = reserve, lookup
            backend.heartbeat = mock.Mock()
            record = {}
            with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                    mock.patch.object(runner.subprocess, 'Popen', return_value=FakeChild(clock, 1000)), \
                    mock.patch.object(runner, 'stop') as stop:
                code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 300, backend, record)
            self.assertEqual(code, 78)
            self.assertEqual(clock.now, 31)
            backend.heartbeat.assert_not_called()
            self.assertEqual(backend.released, ['fixture-id'])
            stop.assert_called_once()

    def test_stop_terminates_group_escalates_and_reaps(self):
        child = mock.Mock(pid=424242)
        child.wait.side_effect = [subprocess.TimeoutExpired('fixture', 2), 0]
        with mock.patch.object(runner.os, 'killpg') as kill:
            runner.stop(child)
        self.assertEqual(kill.call_args_list, [mock.call(child.pid, runner.signal.SIGTERM),
                                              mock.call(child.pid, runner.signal.SIGKILL)])
        self.assertEqual(child.wait.call_args_list, [mock.call(timeout=2), mock.call()])

    def test_actual_accepted_ttl_and_request_start_bound(self):
        row = admission(lease_ttl_sec=7)
        clock = Clock()
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic):
            clock.now = 3  # RPC took three seconds; TTL starts before request.
            lease = runner.ProvenLease(row, 0, 'm5', 'lane', 'work', None, True)
            self.assertLessEqual(lease.deadline, 7)
            prior = lease.deadline
            clock.now = 6
            lease.check(admission(lease_ttl_sec=120))
            self.assertEqual(lease.deadline, prior)  # readback cannot extend
            clock.now = 8
            with self.assertRaises(runner.Refused):
                lease.confirm(row, 0)

    def test_slow_success_after_old_deadline_is_refused(self):
        def renewal(backend, clock):
            clock.now = 121
            return dict(backend.row)
        code, record, backend, clock, events = self.run_case(renewal=renewal, finish=1000)
        self.assertEqual(code, 78)
        self.assertIn('after proven deadline', record['reason'])

    def test_missing_owned_executor_never_spawns(self):
        backend = Backend(None)
        backend.reserve = lambda *args: dict(granted=True, reservation=admission(exempt_pid=1))
        with mock.patch.object(runner.subprocess, 'Popen') as spawn:
            code, record = SupervisorTests().run_case(backend)
        self.assertEqual(code, 78)
        spawn.assert_not_called()
        self.assertEqual(backend.released, ['fixture-id'])


class TransportTests(unittest.TestCase):
    def test_transport_allowlist_and_denial_precedence(self):
        for text, transient in [('MaatMutate HTTP 502', True), ('TLS handshake timeout', True),
                                ('relay restart OUTCOME UNKNOWN', True),
                                ('HTTP 403 scope denied HTTP 502', False),
                                ('reservation expired', False), ('something failed', False)]:
            with self.subTest(text=text):
                error = subprocess.CalledProcessError(1, 'sirsi', stderr=text)
                with mock.patch.object(runner.subprocess, 'run', side_effect=error):
                    with self.assertRaises(runner.TransientTransport if transient else runner.Refused) as caught:
                        runner.Native().call('maat', 'heartbeat', 'fixture')
                    self.assertEqual(isinstance(caught.exception, runner.TransientTransport), transient)

    def test_rpc_timeout_is_clamped_to_proven_deadline(self):
        native = runner.Native()
        native.deadline = 10
        with mock.patch.object(runner.time, 'monotonic', return_value=7), \
                mock.patch.object(runner.subprocess, 'run', return_value=mock.Mock(stdout='{}')) as run:
            native.call('maat', 'status', 'm5', '--json')
            self.assertEqual(run.call_args.kwargs['timeout'], 3)
        with mock.patch.object(runner.time, 'monotonic', return_value=10), \
                mock.patch.object(runner.subprocess, 'run') as run:
            with self.assertRaises(runner.Refused):
                native.call('maat', 'heartbeat', 'fixture')
            run.assert_not_called()

    def test_readback_requires_single_exact_id(self):
        native = runner.Native()
        native.owned_admission = 'fixture-id'
        for rows in [None, {}, [], [admission(id='foreign')], [admission(), admission()]]:
            native.call = mock.Mock(return_value=rows)
            with self.assertRaises(runner.Refused):
                native.lookup('m5')

    def test_renewal_verifies_runtime_and_returns_receipt(self):
        native = runner.Native()
        native.executor_identity = dict(agent_id='lane', thread_id='thread')
        native.verify_runtime = mock.Mock(side_effect=runner.Refused('changed ancestry'))
        native.call = mock.Mock()
        with self.assertRaises(runner.Refused):
            native.heartbeat('fixture-id')
        native.call.assert_not_called()


if __name__ == '__main__':
    unittest.main()
