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

    def test_native_cpu_request_reaches_reservation(self):
        native = runner.Native()
        native.cpu_cores = 1
        native.call = mock.Mock(return_value={})
        native.reserve('m5', 'lane', 'finite-cpu')
        args = native.call.call_args.args
        self.assertIn('--cores', args)
        self.assertEqual(args[args.index('--cores') + 1], '1')

    def test_explicit_quiet_request_does_not_claim_cpu_only_semantics(self):
        native = runner.Native()
        native.cpu_cores = 1
        native.requested_regime = 'quiet'
        native.call = mock.Mock(return_value={})
        with self.assertRaises(runner.Refused):
            native.reserve('m5', 'lane', 'quiet-measurement')
        native.call.assert_not_called()

    def test_explicit_regime_defaults_and_cpu_floor_separation(self):
        self.assertEqual(runner.execution_regime(), 'build')
        self.assertEqual(runner.execution_regime(1), 'build')
        self.assertEqual(runner.execution_regime(None, 'quiet'), 'quiet')
        self.assertEqual(runner.execution_regime(None, 'build'), 'build')
        self.assertEqual(runner.execution_regime(None, 'loaded'), 'loaded')
        self.assertEqual(runner.execution_cores(), 1)
        self.assertEqual(runner.execution_cores(3), 3)
        self.assertIsNone(runner.execution_cores(None, 'quiet'))
        self.assertIsNone(runner.execution_cores(None, 'loaded'))
        for cores, regime in [(1, 'quiet'), (1, 'loaded'), (None, 'unknown')]:
            with self.subTest(cores=cores, regime=regime), self.assertRaises(runner.Refused):
                runner.execution_regime(cores, regime)

    def test_native_reserve_sends_real_quiet_request_not_loaded_label(self):
        native = runner.Native()
        native.cpu_cores = None
        native.requested_regime = 'quiet'
        with mock.patch.object(native, 'call', return_value={}) as call:
            native.reserve('m5', 'lane', 'work')
        argv = call.call_args.args
        self.assertEqual(argv[argv.index('--regime') + 1], 'quiet')
        self.assertEqual(argv[argv.index('--exempt-pid') + 1], str(os.getpid()))

    def test_quiet_never_borrows_existing_loaded_or_relabels_reply(self):
        for reply_regime, share in [('quiet', 'full'), ('loaded', 'full'), ('quiet', 'floor')]:
            with self.subTest(regime=reply_regime, share=share):
                class QuietBackend(Backend):
                    requested_regime = 'quiet'
                    def reserve(self, resource, agent, work):
                        self.reserve_calls += 1
                        self.row = admission(id='fresh-quiet', regime=reply_regime, share=share, cores=4)
                        return dict(granted=True, reservation=self.row)
                backend = QuietBackend(admission(id='old-loaded', regime='loaded'))
                record = {}
                with mock.patch.object(runner.subprocess, 'Popen', wraps=runner.subprocess.Popen) as spawn:
                    code = runner.supervise(['/usr/bin/true'], 'm5', 'lane', 'thread', 'work', 10, backend, record)
                self.assertEqual(backend.reserve_calls, 1)
                self.assertEqual(backend.released, ['fresh-quiet'])
                self.assertEqual(record['requested_regime'], 'quiet')
                if reply_regime == 'quiet' and share == 'full':
                    self.assertEqual(code, 0, record)
                    self.assertEqual(record['admission'], 'fresh-quiet')
                    self.assertFalse(record['borrowed'])
                    spawn.assert_called_once()
                else:
                    self.assertEqual(code, 78)
                    spawn.assert_not_called()

    def test_quiet_renewal_cannot_change_regime(self):
        row = admission(regime='quiet')
        lease = runner.ProvenLease(row, runner.time.monotonic(), 'm5', 'lane', 'work', None, row['id'])
        with self.assertRaises(runner.Refused):
            lease.check(dict(row, regime='loaded'))

    def test_quiet_cli_rejects_missing_executor_before_any_launch(self):
        result = subprocess.run([sys.executable, str(Path(runner.__file__)), '--agent', 'lane',
                                 '--work', 'work', '--regime', 'quiet', '--', '/usr/bin/true'],
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 2)
        self.assertIn('Quiet qualification requires --register-executor', result.stderr)

    def test_execution_fact_is_private_exact_and_passed_only_to_owned_child(self):
        with tempfile.TemporaryDirectory() as directory:
            backend = Backend(admission())
            backend.execution_receipt_root = Path(directory)
            command = [sys.executable, '-c',
                       'import os,json; p=os.environ["SIRSI_MAAT_EXECUTION_RECEIPT"]; '
                       'r=json.load(open(p)); assert r["supervisor_pid"]==os.getppid(); '
                       'assert r["work"]=="work"; assert os.stat(p).st_mode & 0o777 == 0o600']
            record = {}
            with mock.patch.object(runner, 'process_identity', return_value=(1, 'actual-parent-birth')):
                code = runner.supervise(command, 'm5', 'lane', 'thread', 'work', 10, backend, record)
            self.assertEqual(code, 0, record)
            files = list(Path(directory).glob('execution-*.json'))
            self.assertEqual(len(files), 1)
            data = json.loads(files[0].read_text())
            self.assertEqual(data['admission_id'], 'fixture-id')
            self.assertEqual(data['thread_id'], 'thread')
            self.assertEqual(data['budget_seconds'], 10)
            self.assertEqual(data['supervisor_start'], 'actual-parent-birth')
            self.assertEqual(data['argv_sha256'], runner.hashlib.sha256(b'\0'.join(os.fsencode(x) for x in command)).hexdigest())
            self.assertEqual(files[0].read_text(), json.dumps(data, sort_keys=True, separators=(',', ':')))
    def test_partial_registration_prune_conflict_uses_exact_readback_not_reregister(self):
        native = runner.Native()
        calls = []
        def run(argv, **kwargs):
            calls.append(argv)
            if argv[1:3] == ['thread', 'register']:
                raise subprocess.CalledProcessError(1, argv, stderr='thread "thr-old" prune lost lifecycle fence')
            identity = argv[argv.index('--thread')+1]
            return mock.Mock(stdout=json.dumps(dict(agent_id='lane', pid=os.getpid(), status='active', thread_id=identity)))
        with mock.patch.object(runner.subprocess, 'run', side_effect=run):
            identity = native.register_executor('lane')
        self.assertEqual([x[1:3] for x in calls], [['thread', 'register'], ['thread', 'heartbeat']])
        self.assertEqual(calls[0][calls[0].index('--thread')+1], identity)
        self.assertEqual(calls[1][calls[1].index('--thread')+1], identity)

    def test_partial_registration_classifier_never_relaxes_denial_or_other_calls(self):
        for operation, detail in [
            (('thread', 'heartbeat'), 'thread "thr-old" prune lost lifecycle fence'),
            (('thread', 'register'), '403 denied; thread "thr-old" prune lost lifecycle fence'),
            (('thread', 'register'), 'thread "thr-old" mutation lost lifecycle fence')]:
            with mock.patch.object(runner.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, 'sirsi', stderr=detail)):
                with self.assertRaises(runner.Refused) as error:
                    runner.Native().call(*operation)
            self.assertNotIsInstance(error.exception, runner.TransientTransport)

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
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance):
            self.assertEqual(native.registration_observation('own'), {'thread_id': 'own'})
        self.assertEqual(native.call.call_count, 2)
        self.assertEqual(native.deadline, 75)
        native.deadline = None
        native.call = mock.Mock(side_effect=runner.Refused('403 denied'))
        with self.assertRaises(runner.Refused):
            native.registration_observation('own')
        self.assertEqual(native.call.call_count, 1)

    def test_uncertain_registration_waits_only_for_the_same_pending_identity(self):
        native = runner.Native()
        clock = Clock()
        calls = []
        def call(*args, **kwargs):
            calls.append(args)
            identity = args[args.index('--thread') + 1]
            if args[:2] == ('thread', 'register'):
                raise runner.TransientTransport('HTTP 502 OUTCOME UNKNOWN Bearer secret-do-not-retain')
            if len(calls) == 2:
                raise runner.Refused("Control call ('thread', 'heartbeat') failed: thread \"" + identity +
                                     '\" not registered: heartbeat registration unavailable')
            return dict(agent_id='lane', pid=os.getpid(), status='active', thread_id=identity)
        native.call = call
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance):
            identity = native.register_executor('lane')
        self.assertEqual([c[:2] for c in calls], [('thread', 'register'), ('thread', 'heartbeat'), ('thread', 'heartbeat')])
        self.assertTrue(all(c[c.index('--thread') + 1] == identity for c in calls))
        self.assertEqual(clock.now, 1)

    def test_uncertain_registration_retains_safe_initial_cause_on_refusal(self):
        native = runner.Native()
        native.call = mock.Mock(side_effect=[
            runner.TransientTransport('HTTP 503 OUTCOME UNKNOWN Bearer secret-do-not-retain'),
            runner.Refused('403 denied')])
        with self.assertRaises(runner.Refused) as caught:
            native.register_executor('lane')
        self.assertIn('http_503,relay_outcome_unknown', str(caught.exception))
        self.assertIn('403 denied', str(caught.exception))
        self.assertNotIn('secret-do-not-retain', str(caught.exception))
        self.assertEqual(native.call.call_count, 2)

    def test_missing_registration_is_not_retried_outside_pending_recovery(self):
        native = runner.Native()
        native.call = mock.Mock(side_effect=runner.Refused(
            "Control call ('thread', 'heartbeat') failed: thread \"own\" not registered: heartbeat registration unavailable"))
        with self.assertRaises(runner.Refused):
            native.registration_observation('own')
        self.assertEqual(native.call.call_count, 1)

    def test_pending_missing_registration_respects_prior_deadline(self):
        native = runner.Native()
        native.deadline = 3
        native.call = mock.Mock(side_effect=runner.Refused(
            "Control call ('thread', 'heartbeat') failed: thread \"own\" not registered: heartbeat registration unavailable"))
        clock = Clock()
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance), \
                self.assertRaises(runner.TransientTransport):
            native.registration_observation('own', pending_registration=True)
        self.assertEqual(clock.now, 3)
        self.assertEqual(native.deadline, 3)
        self.assertEqual(native.call.call_count, 2)

    def test_uncertain_registration_never_retries_denial_terminal_or_foreign_missing(self):
        for message in (
                '403 denied',
                'thread "own" is closed and cannot be revived by heartbeat',
                'thread "own" belongs to a different native runtime',
                "Control call ('thread', 'heartbeat') failed: thread \"foreign\" not registered: heartbeat registration unavailable",
                "Control call ('thread', 'heartbeat') failed: 403 denied; thread \"own\" not registered: heartbeat registration unavailable",
                "Control call ('thread', 'heartbeat') failed: unauthorized; thread \"own\" not registered: heartbeat registration unavailable",
                "Control call ('thread', 'heartbeat') failed: unauthorised; thread \"own\" not registered: heartbeat registration unavailable",
                "Control call ('thread', 'heartbeat') failed: released; thread \"own\" not registered: heartbeat registration unavailable"):
            with self.subTest(message=message):
                native = runner.Native()
                native.deadline = 3
                native.call = mock.Mock(side_effect=runner.Refused(message))
                clock = Clock()
                with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                        mock.patch.object(runner.time, 'sleep', side_effect=clock.advance) as sleep, \
                        self.assertRaises(runner.Refused):
                    native.registration_observation('own', pending_registration=True)
                self.assertEqual(native.call.call_count, 1)
                sleep.assert_not_called()

    def test_registration_observation_stops_at_bounded_budget(self):
        native = runner.Native()
        native.call = mock.Mock(side_effect=runner.TransientTransport('HTTP 502'))
        clock = Clock()
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance), \
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
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance):
            with self.assertRaises(runner.TransientTransport):
                native.registered('lane','thread')
            process.assert_not_called()


class SupervisorTests(unittest.TestCase):
    def test_registered_executor_gets_own_loaded_grant_despite_foreign_primary_holder(self):
        backend = runner.Native()
        backend.executor_thread = 'thr-actual-executor'
        backend.registered = mock.Mock()
        backend.lookup = mock.Mock(side_effect=lambda resource: admission() if backend.owned_admission
                                   else admission(holder='other', work='training'))
        backend.reserve = mock.Mock(return_value={'granted': True, 'reservation': admission()})
        backend.release = mock.Mock()
        record = {}
        clock = Clock()
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=FakeChild(clock, 0)), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                mock.patch.object(runner, 'release_fabric_child'), \
                mock.patch.object(runner, 'fabric_child_terminal', side_effect=lambda child: child.terminal_status()), \
                mock.patch.object(runner, 'stop'):
            code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 30, backend, record)
        self.assertEqual(code, 0)
        self.assertEqual(backend.reserve.call_count, 1)
        self.assertFalse(record['borrowed'])
        self.assertEqual(backend.lookup.call_count, 2)

    def test_registered_executor_never_borrows_old_same_lane_work_grant(self):
        backend = runner.Native()
        backend.executor_thread = 'thr-actual-executor'
        backend.registered = mock.Mock()
        backend.lookup = mock.Mock(side_effect=lambda resource: admission() if backend.owned_admission
                                   else admission(exempt_pid=1))
        backend.reserve = mock.Mock(return_value={'granted': True, 'reservation': admission()})
        backend.release = mock.Mock()
        record = {}
        clock = Clock()
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=FakeChild(clock, 0)), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                mock.patch.object(runner, 'release_fabric_child'), \
                mock.patch.object(runner, 'fabric_child_terminal', side_effect=lambda child: child.terminal_status()), \
                mock.patch.object(runner, 'stop'):
            code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 30, backend, record)
        self.assertEqual(code, 0)
        backend.reserve.assert_called_once()
        self.assertFalse(record['borrowed'])
        self.assertEqual(backend.lookup.call_count, 2)

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
        self.assertEqual(code, 0, record)
        self.assertEqual(record['share'],'floor')

    def test_owned_lookup_uses_exact_id_query_without_resource(self):
        native = runner.Native()
        native.owned_admission = 'own-grant'
        response = mock.Mock(stdout=json.dumps([admission(id='own-grant')]))
        with mock.patch.object(runner.subprocess, 'run', return_value=response) as run:
            self.assertEqual(native.lookup('m5')['id'], 'own-grant')
        self.assertEqual(run.call_args.args[0],
                         [native.cli, 'maat', 'status', '--id', 'own-grant', '--json'])

    def run_case(self, backend, command=None, timeout=2, agent='lane', thread='thread'):
        record = {}
        code = runner.supervise(command or [sys.executable, '-c', 'pass'], 'm5', agent, thread,
                                'work', timeout, backend, record)
        return code, record

    def test_borrowed_admission_success_does_not_release_others_hold(self):
        backend = Backend(admission())
        code, record = self.run_case(backend)
        self.assertEqual(code, 0, record)
        self.assertTrue(record['borrowed'])
        self.assertEqual(backend.released, [])

    def test_own_admission_is_acquired_and_released(self):
        backend = Backend(None)
        code, record = self.run_case(backend)
        self.assertEqual(code, 0, record)
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
        with self.assertRaises(ProcessLookupError, msg=f"owned timeout child was not reaped: {record}"):
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
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                mock.patch.object(runner, 'release_fabric_child'), \
                mock.patch.object(runner, 'fabric_child_terminal', side_effect=lambda target: target.terminal_status()), \
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
            self.assertTrue(record['reservation_outcome_unconfirmed'])
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
    def advance(self, seconds):
        # Keep repeated subsecond poll steps on a deterministic nanosecond grid.
        self.now = round(self.now + seconds, 9)


class FakeChild:
    pid = 424242
    def __init__(self, clock, finish, exit_code=0):
        self.clock, self.finish = clock, finish
        self.exit_code = exit_code
        self.returncode = None
        self.wait_timeouts = []
        self.poll_calls = 0
        self.terminal_calls = 0
    def _record_reap(self):
        self.returncode = self.exit_code
        identity = getattr(self, '_sirsi_fabric_identity', None)
        if identity is not None:
            identity.reaped = True
    def poll(self):
        self.poll_calls += 1
        if self.returncode is not None:
            return self.returncode
        if self.clock.now >= self.finish:
            self._record_reap()
            return self.returncode
        return None
    def terminal_status(self):
        self.terminal_calls += 1
        return self.exit_code if self.clock.now >= self.finish else None
    def wait(self, timeout=None):
        self.wait_timeouts.append(timeout)
        if self.returncode is not None:
            return self.returncode
        if timeout is not None and self.clock.now + timeout < self.finish:
            self.clock.advance(timeout)
            raise subprocess.TimeoutExpired('fixture', timeout)
        self.clock.now = max(self.clock.now, self.finish)
        self._record_reap()
        return self.returncode


def bind_fake_fabric_child(child):
    identity = runner._FabricChildIdentity(child, child.pid, os.getpid(), os.getuid(),
                                           os.getuid(), child.pid, 'fixture')
    child._sirsi_fabric_identity = identity
    return identity


class FakeChildReapModelTests(unittest.TestCase):
    def test_poll_and_wait_consume_only_when_child_is_terminal(self):
        clock = Clock()
        child = FakeChild(clock, finish=2, exit_code=17)
        identity = bind_fake_fabric_child(child)
        self.assertIsNone(child.poll())
        with self.assertRaises(subprocess.TimeoutExpired):
            child.wait(timeout=1)
        self.assertIsNone(child.returncode)
        self.assertFalse(identity.reaped)
        clock.advance(1)
        self.assertEqual(child.poll(), 17)
        self.assertEqual(child.returncode, 17)
        self.assertTrue(identity.reaped)

        clock = Clock()
        child = FakeChild(clock, finish=0, exit_code=23)
        identity = bind_fake_fabric_child(child)
        self.assertEqual(child.wait(), 23)
        self.assertEqual(child.returncode, 23)
        self.assertTrue(identity.reaped)


class FabricLaunchGateTests(unittest.TestCase):
    def invoke_gate(self, ready, token, environment, exec_error=None):
        with mock.patch.object(runner.sys, 'argv', ['-c', '41', 'payload', 'arg']), \
                mock.patch.dict(runner.os.environ, environment, clear=True), \
                mock.patch.object(runner.os, 'fstat', return_value=mock.Mock(st_mode=runner.stat.S_IFIFO)), \
                mock.patch('select.select', return_value=(ready, [], [])), \
                mock.patch.object(runner.os, 'read', return_value=token) as read, \
                mock.patch.object(runner.os, 'close') as close, \
                mock.patch.object(runner.os, 'execvpe', side_effect=exec_error) as execute:
            with self.assertRaises(SystemExit) as exited:
                exec(runner._FABRIC_GATE_PROGRAM, {})
        self.assertEqual(exited.exception.code, 78)
        close.assert_called_once_with(41)
        return read, execute

    def test_parent_death_eof_and_missing_release_timeout_never_exec(self):
        for ready, token in [([41], b''), ([], b'')]:
            with self.subTest(ready=ready):
                read, execute = self.invoke_gate(ready, token, runner.fabric_gate_environment({}))
                if ready:
                    read.assert_called_once_with(41, 1)  # EOF after the parent's writer closes.
                else:
                    read.assert_not_called()
                execute.assert_not_called()

    def test_isolated_gate_restores_payload_loader_environment_after_release(self):
        original = {'DYLD_INSERT_LIBRARIES': 'fixture-dylib', 'LD_PRELOAD': 'fixture-so',
                    'PYTHONPATH': 'fixture-path', runner._FABRIC_GATE_ENV: 'prior-value'}
        environment = runner.fabric_gate_environment(original)
        self.assertNotIn('DYLD_INSERT_LIBRARIES', environment)
        self.assertNotIn('LD_PRELOAD', environment)
        read, execute = self.invoke_gate([41], b'1', environment)
        read.assert_called_once_with(41, 1)
        execute.assert_called_once_with('payload', ['payload', 'arg'], original)

    def test_payload_exec_failure_refuses_without_a_second_exec(self):
        environment = runner.fabric_gate_environment({})
        _, execute = self.invoke_gate([41], b'1', environment,
                                      FileNotFoundError('fixture missing payload'))
        execute.assert_called_once_with('payload', ['payload', 'arg'], {})

    def run_bind_failure(self, wait_result, release_error=None, close_error=None):
        child = mock.Mock(pid=424242, returncode=None)
        backend = Backend(None)
        events = []
        def wait(timeout):
            events.append(('wait', timeout))
            if isinstance(wait_result, BaseException):
                raise wait_result
            child.returncode = wait_result
            return wait_result
        child.wait.side_effect = wait
        def release_reservation(identity):
            events.append(('release', identity))
            if release_error:
                raise release_error
        backend.release = mock.Mock(side_effect=release_reservation)
        record = {}
        def close(fd):
            events.append(('close', fd))
            if fd == 42 and close_error:
                raise close_error
        with mock.patch.object(runner.os, 'pipe', return_value=(41, 42)), \
                mock.patch.object(runner.os, 'close', side_effect=close), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child) as spawn, \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=runner.Refused('fixture bind refusal')), \
                mock.patch.object(runner, 'release_fabric_child') as release, \
                mock.patch.object(runner.os, 'killpg') as signal_group:
            code = runner.supervise(['payload', 'arg'], 'm5', 'lane', 'thread', 'work', 30,
                                    backend, record)

        self.assertEqual(code, 78)
        argv = spawn.call_args.args[0]
        self.assertEqual(argv[1:5], ['-I', '-S', '-c', runner._FABRIC_GATE_PROGRAM])
        self.assertEqual(argv[5], '41')
        self.assertEqual(argv[-2:], ['payload', 'arg'])
        self.assertEqual(spawn.call_args.kwargs['pass_fds'], (41,))
        release.assert_not_called()
        signal_group.assert_not_called()
        self.assertLess(events.index(('close', 42)), events.index(('wait', 6)))
        return record, backend, events

    def test_bind_failure_releases_only_after_refusal_exit_is_reaped(self):
        record, backend, events = self.run_bind_failure(78)
        self.assertLess(events.index(('wait', 6)), events.index(('release', 'fixture-id')))
        backend.release.assert_called_once_with('fixture-id')
        self.assertNotIn('cleanup_debt', record)

    def test_bind_failure_retains_grant_on_timeout_echild_or_unexpected_status(self):
        for result in (subprocess.TimeoutExpired('gate', 6), ChildProcessError('ECHILD'), 0):
            with self.subTest(result=result):
                record, backend, _ = self.run_bind_failure(result)
                backend.release.assert_not_called()
                self.assertTrue(record['child_cleanup_unconfirmed'])
                self.assertIn('Owned reservation release deferred', record['cleanup_debt'])
                reservation = record['unresolved_reservation']
                confirmation = reservation['last_confirmed']
                self.assertEqual(reservation['scope']['id'], 'fixture-id')
                self.assertTrue(reservation['owned'])
                self.assertFalse(reservation['expiry_is_current_authority'])
                self.assertEqual(reservation['explicit_release'], 'deferred')
                self.assertTrue(reservation['automatic_expiry_possible'])
                stamp = dt.datetime.fromisoformat(confirmation['heartbeat_at'].replace('Z', '+00:00'))
                self.assertEqual(confirmation['heartbeat_plus_ttl'],
                                 (stamp + dt.timedelta(seconds=confirmation['lease_ttl_sec'])).isoformat())
                custody = record['unresolved_child_custody']
                self.assertEqual(custody['popen_pid'], 424242)
                self.assertIsNone(custody['bound_identity'])
                self.assertFalse(custody['release_attempted'])
                self.assertEqual(custody['release_outcome'], 'not_attempted')
                self.assertFalse(custody['termination_confirmed'])
                self.assertFalse(custody['handle_transferable'])

    def test_pipe_close_failure_does_not_substitute_for_exit_receipt(self):
        failure = OSError('fixture close failure')
        record, backend, _ = self.run_bind_failure(subprocess.TimeoutExpired('gate', 6), close_error=failure)
        backend.release.assert_not_called()
        self.assertIn('Fabric launch gate close unconfirmed', record['cleanup_debt'])
        self.assertTrue(record['child_cleanup_unconfirmed'])
        record, backend, _ = self.run_bind_failure(78, close_error=failure)
        backend.release.assert_called_once_with('fixture-id')  # Exact exit receipt, not close, permits release.
        self.assertIn('Fabric launch gate close unconfirmed', record['cleanup_debt'])

    def test_previously_reaped_helper_status_is_not_a_fresh_receipt(self):
        child = mock.Mock(pid=424242, returncode=78)
        with self.assertRaisesRegex(runner.Refused, 'wait custody unavailable'):
            runner.wait_unreleased_fabric_child(child)
        child.wait.assert_not_called()

    def test_reaped_helper_release_failure_records_debt(self):
        record, backend, events = self.run_bind_failure(78, runner.Refused('release outcome unknown'))
        self.assertLess(events.index(('wait', 6)), events.index(('release', 'fixture-id')))
        backend.release.assert_called_once_with('fixture-id')
        self.assertIn('Reservation release unconfirmed: release outcome unknown', record['cleanup_debt'])
        backend.close_executor = mock.Mock()
        runner.close_fabric_executor(backend, record)
        backend.close_executor.assert_not_called()
        self.assertIn('Executor close deferred: reservation outcome unconfirmed', record['cleanup_debt'])

    def test_deadline_expired_during_bind_keeps_gate_closed(self):
        clock = Clock()
        child = FakeChild(clock, 0, exit_code=78)
        backend = Backend(None)
        record = {}
        def bind(target):
            bind_fake_fabric_child(target)
            clock.now = 30
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.os, 'pipe', return_value=(41, 42)), \
                mock.patch.object(runner.os, 'close'), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind), \
                mock.patch.object(runner, 'release_fabric_child') as release, \
                mock.patch.object(runner, 'stop') as stop:
            self.assertEqual(runner.supervise(['payload'], 'm5', 'lane', 'thread', 'work', 30,
                                              backend, record), 78)
        release.assert_not_called()
        stop.assert_not_called()
        self.assertEqual(child.wait_timeouts, [6])
        self.assertEqual(backend.released, ['fixture-id'])
        self.assertEqual(record['stop_cause'], 'execution_timeout')

    def test_success_binds_before_release_and_cleans_before_grant_release(self):
        clock = Clock()
        child = FakeChild(clock, 0)
        backend = Backend(None)
        events = []
        backend.release = mock.Mock(side_effect=lambda identity: events.append('grant release'))
        backend.close_executor = mock.Mock(side_effect=lambda: events.append('executor close'))
        def bind(target):
            events.append('bind')
            return bind_fake_fabric_child(target)
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.os, 'pipe', return_value=(41, 42)), \
                mock.patch.object(runner.os, 'close'), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind), \
                mock.patch.object(runner, 'release_fabric_child', side_effect=lambda fd: events.append('gate release')), \
                mock.patch.object(runner, 'fabric_child_terminal', return_value=0), \
                mock.patch.object(runner, 'stop', side_effect=lambda target: events.append('cleanup')):
            record = {}
            self.assertEqual(runner.supervise(['payload'], 'm5', 'lane', 'thread', 'work', 30,
                                              backend, record), 0)
            runner.close_fabric_executor(backend, record)
        self.assertEqual(events, ['bind', 'gate release', 'cleanup', 'grant release', 'executor close'])

    def test_payload_exec_failure_is_reaped_before_grant_release(self):
        clock = Clock()
        child = FakeChild(clock, 0, exit_code=78)
        child.wait = mock.Mock(wraps=child.wait)
        backend = Backend(None)
        backend.release = mock.Mock(side_effect=lambda identity: self.assertEqual(child.returncode, 78))
        record = {}
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.os, 'pipe', return_value=(41, 42)), \
                mock.patch.object(runner.os, 'close'), \
                mock.patch.object(runner.os, 'killpg') as kill_group, \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                mock.patch.object(runner, 'release_fabric_child'), \
                mock.patch.object(runner, 'fabric_child_terminal', return_value=78), \
                mock.patch.object(runner, 'local_group_snapshot',
                                  side_effect=lambda group: [dict(pid=child.pid, pgid=group, status='Z')]):
            self.assertEqual(runner.supervise(['missing-payload'], 'm5', 'lane', 'thread', 'work', 30,
                                              backend, record), 78)
        child.wait.assert_called_once_with(timeout=1)
        kill_group.assert_not_called()
        backend.release.assert_called_once_with('fixture-id')
        self.assertNotIn('cleanup_debt', record)

    def test_uncertain_release_and_failed_bound_cleanup_retain_grant(self):
        child = FakeChild(Clock(), 0)
        backend = Backend(None)
        backend.close_executor = mock.Mock()
        record = {}
        with mock.patch.object(runner.os, 'pipe', return_value=(41, 42)), \
                mock.patch.object(runner.os, 'close'), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                mock.patch.object(runner, 'release_fabric_child', side_effect=OSError('write uncertain')), \
                mock.patch.object(runner, 'stop', side_effect=runner.Refused('custody uncertain')) as stop:
            self.assertEqual(runner.supervise(['payload'], 'm5', 'lane', 'thread', 'work', 30,
                                              backend, record), 78)
        stop.assert_called_once_with(child)
        self.assertEqual(backend.released, [])
        runner.close_fabric_executor(backend, record)
        backend.close_executor.assert_not_called()
        self.assertIn('custody uncertain', record['cleanup_debt'])
        self.assertIn('Owned reservation release deferred', record['cleanup_debt'])
        self.assertIn('Executor close deferred', record['cleanup_debt'])
        self.assertTrue(record['unresolved_child_custody']['release_attempted'])
        self.assertEqual(record['unresolved_child_custody']['release_outcome'], 'unknown')
        self.assertEqual(record['unresolved_child_custody']['bound_identity']['pid'], child.pid)
        self.assertEqual(record['unresolved_reservation']['scope']['id'], 'fixture-id')

    def test_gate_short_write_or_interruption_never_implies_safe_handback(self):
        for result in (0, InterruptedError('fixture write interrupted')):
            with self.subTest(result=result):
                child = FakeChild(Clock(), 0)
                backend = Backend(None)
                record = {}
                write = mock.Mock(side_effect=result) if isinstance(result, BaseException) else mock.Mock(return_value=result)
                with mock.patch.object(runner.os, 'pipe', return_value=(41, 42)), \
                        mock.patch.object(runner.os, 'close'), \
                        mock.patch.object(runner.os, 'write', write), \
                        mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                        mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                        mock.patch.object(runner, 'stop', side_effect=runner.Refused('cleanup uncertain')) as stop:
                    self.assertEqual(runner.supervise(['payload'], 'm5', 'lane', 'thread', 'work', 30,
                                                      backend, record), 78)
                write.assert_called_once_with(42, b'1')
                stop.assert_called_once_with(child)
                self.assertEqual(backend.released, [])
                self.assertTrue(record['unresolved_child_custody']['release_attempted'])
                self.assertEqual(record['unresolved_child_custody']['release_outcome'], 'unknown')
                self.assertFalse(record['unresolved_child_custody']['termination_confirmed'])

    def test_unknown_renewal_keeps_only_last_confirmed_expiry(self):
        clock = Clock()
        child = FakeChild(clock, 1000)
        backend = Backend(None)
        backend.heartbeat = mock.Mock(side_effect=runner.TransientTransport('fixture renewal outcome unknown'))
        record = {}
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                mock.patch.object(runner, 'release_fabric_child'), \
                mock.patch.object(runner, 'fabric_child_terminal', return_value=None), \
                mock.patch.object(runner, 'stop', side_effect=runner.Refused('cleanup uncertain')):
            self.assertEqual(runner.supervise(['payload'], 'm5', 'lane', 'thread', 'work', 31,
                                              backend, record), 78)
        backend.heartbeat.assert_called_once_with('fixture-id')
        self.assertEqual(backend.released, [])
        reservation = record['unresolved_reservation']
        self.assertEqual(reservation['scope']['id'], 'fixture-id')
        self.assertEqual(reservation['renewal_outcome'], 'unknown')
        self.assertFalse(reservation['expiry_is_current_authority'])
        self.assertIsNotNone(reservation['last_confirmed']['heartbeat_plus_ttl'])
        self.assertEqual(record['unresolved_child_custody']['release_outcome'], 'delivered')

    def test_executor_close_is_deferred_when_child_cleanup_is_unknown(self):
        backend = mock.Mock()
        record = {'child_cleanup_unconfirmed': True, 'cleanup_debt': 'child uncertain'}
        runner.close_fabric_executor(backend, record)
        backend.close_executor.assert_not_called()
        self.assertIn('child uncertain', record['cleanup_debt'])
        self.assertIn('Executor close deferred', record['cleanup_debt'])


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
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance), \
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

    def test_recurring_propagates_unknown_child_cleanup(self):
        for field in ('child_cleanup_unconfirmed', 'reservation_outcome_unconfirmed'):
            with self.subTest(field=field):
                backend = mock.Mock(executor_thread='thr-real')
                record = {}
                def iteration(*args):
                    args[-1][field] = True
                    return 78
                with mock.patch.object(runner, 'supervise', side_effect=iteration):
                    self.assertEqual(runner.supervise_iterations(['fixture'], 'm5', 'lane', 'thread',
                                                                'work', 30, backend, record, 2, 0, 120), 78)
                self.assertTrue(record[field])

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
                mock.patch.object(runner.time, 'sleep', side_effect=clock.advance), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                mock.patch.object(runner, 'release_fabric_child'), \
                mock.patch.object(runner, 'fabric_child_terminal', side_effect=lambda target: target.terminal_status()), \
                mock.patch.object(runner, 'stop') as stop:
            code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', timeout, backend, record)
            stop.assert_called_once_with(child)
        self.assertEqual(backend.reserve_calls, 1)
        self.assertEqual(backend.released, ['fixture-id'])
        return code, record, backend, clock, events

    def test_healthy_run_bounds_remote_reads_without_slowing_child_monitoring(self):
        code, record, backend, clock, events = self.run_case(finish=95)
        self.assertEqual(code, 0)
        self.assertEqual(clock.now, 95)
        reads = [at for kind,at in events if kind == 'read']
        renewals = [at for kind,at in events if kind == 'renew']
        # Initial observation + pre-spawn + three healthy checks + terminal.
        self.assertEqual(reads, [0, 0, 30, 60, 90, 95])
        self.assertEqual(renewals, [30, 60, 90])
        self.assertEqual(backend.reserve_calls, 1)
        self.assertGreater(backend.child.terminal_calls, 40)
        self.assertEqual(backend.child.poll_calls, 0)
        self.assertEqual(backend.child.wait_timeouts, [])

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
                    mock.patch.object(runner.time, 'sleep', side_effect=clock.advance), \
                    mock.patch.object(runner.subprocess, 'Popen', return_value=FakeChild(clock, 1000)), \
                    mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                    mock.patch.object(runner, 'release_fabric_child'), \
                    mock.patch.object(runner, 'fabric_child_terminal', side_effect=lambda child: child.terminal_status()), \
                    mock.patch.object(runner, 'stop') as stop:
                code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 300, backend, record)
            self.assertEqual(code, 78)
            self.assertEqual(clock.now, 31)
            backend.heartbeat.assert_not_called()
            self.assertEqual(backend.released, ['fixture-id'])
            stop.assert_called_once()

    def test_fabric_stop_signals_before_the_only_reap_and_never_after_pgid_reuse(self):
        clock = Clock()
        child = FakeChild(clock, 0, exit_code=17)
        child.returncode = None
        identity = bind_fake_fabric_child(child)
        events = []
        killed = False
        pgid_reassigned = False

        def observe(target):
            self.assertIs(target, child)
            events.append(('observe', identity.reaped, pgid_reassigned))
            return 17

        def snapshot(group):
            self.assertEqual(group, child.pid)
            events.append(('snapshot', identity.reaped, pgid_reassigned))
            rows = [dict(pid=child.pid, pgid=group, status='Z')]
            if not killed:
                rows.append(dict(pid=424243, pgid=group, status='S'))
            return rows

        def send_group(group, sig):
            nonlocal killed
            self.assertEqual(group, child.pid)
            self.assertFalse(identity.reaped, 'group signal after direct-child reap')
            self.assertFalse(pgid_reassigned, 'reaped PID/PGID may now name another group')
            self.assertFalse(identity.signals_closed)
            events.append(('signal', sig, identity.reaped, pgid_reassigned))
            if sig == runner.signal.SIGTERM:
                clock.now = 2
            else:
                killed = True

        def reap(target):
            nonlocal pgid_reassigned
            self.assertIs(target, child)
            self.assertEqual([event[0:2] for event in events if event[0] == 'signal'][-1],
                             ('signal', runner.signal.SIGKILL))
            events.append(('reap',))
            target._record_reap()
            pgid_reassigned = True  # Model immediate reuse after wait() consumes the leader.
            return 17

        runner.stop(child, observe=observe, send_group=send_group, reap=reap,
                    monotonic=clock.monotonic, sleep=clock.advance,
                    snapshot_group=snapshot)
        self.assertEqual(child.returncode, 17)
        self.assertEqual([event[0:2] for event in events if event[0] in ('signal', 'reap')],
                         [('signal', runner.signal.SIGTERM), ('signal', runner.signal.SIGKILL), ('reap',)])
        self.assertTrue(identity.reaped)
        self.assertTrue(identity.signals_closed)
        with self.assertRaises(runner.Refused):
            runner._signal_fabric_group(child, runner.signal.SIGTERM, observe, send_group)
        self.assertEqual(len([event for event in events if event[0] == 'signal']), 2)

    def test_fabric_terminal_singleton_zombie_reaps_without_signaling(self):
        child = FakeChild(Clock(), 0, exit_code=17)
        identity = bind_fake_fabric_child(child)
        events = []
        def observe(target):
            events.append(('observe', identity.reaped))
            return 17
        def snapshot(group):
            events.append(('snapshot', identity.reaped))
            return [dict(pid=child.pid, pgid=group, status='Z')]
        def send_group(*args):
            self.fail('terminal-only group must not be signaled')
        def reap(target):
            events.append(('reap', identity.reaped, identity.signals_closed))
            target._record_reap()
            return 17
        runner.stop(child, observe=observe, send_group=send_group, reap=reap,
                    snapshot_group=snapshot)
        self.assertEqual(child.returncode, 17)
        self.assertEqual(events[-1], ('reap', False, True))
        self.assertTrue(identity.reaped)
        self.assertTrue(identity.signals_closed)

    def test_fabric_exit_between_wait_and_initial_snapshot_is_reobserved(self):
        child = FakeChild(Clock(), 0, exit_code=17)
        identity = bind_fake_fabric_child(child)
        observations = iter([None, 17])
        send = mock.Mock()
        def reap(target):
            target._record_reap()
            return 17
        runner.stop(child, observe=lambda target: next(observations), send_group=send, reap=reap,
                    snapshot_group=lambda group: [dict(pid=child.pid, pgid=group, status='Z')])
        self.assertEqual(child.returncode, 17)
        self.assertTrue(identity.reaped)
        send.assert_not_called()

    def test_fabric_zombie_without_terminal_wait_receipt_stays_debt(self):
        child = FakeChild(Clock(), 0)
        identity = bind_fake_fabric_child(child)
        send, reap = mock.Mock(), mock.Mock()
        with self.assertRaisesRegex(runner.Refused, 'Live wait status conflicts'):
            runner.stop(child, observe=lambda target: None, send_group=send, reap=reap,
                        snapshot_group=lambda group: [dict(pid=child.pid, pgid=group, status='Z')])
        self.assertTrue(identity.signals_closed)
        self.assertFalse(identity.reaped)
        send.assert_not_called()
        reap.assert_not_called()

    def test_fabric_exit_between_wait_and_post_term_snapshot_is_reobserved(self):
        child = FakeChild(Clock(), 0)
        identity = bind_fake_fabric_child(child)
        observations = iter([None, None, None, 0, 0])
        signals = []
        def snapshot(group):
            return [dict(pid=child.pid, pgid=group, status='Z' if signals else 'S')]
        def send(group, sig):
            self.assertFalse(identity.reaped)
            signals.append(sig)
        def reap(target):
            target._record_reap()
            return 0
        runner.stop(child, observe=lambda target: next(observations), send_group=send, reap=reap,
                    snapshot_group=snapshot)
        self.assertEqual(signals, [runner.signal.SIGTERM])
        self.assertTrue(identity.reaped)

    def test_fabric_drifting_snapshots_after_term_escalate_with_retained_custody(self):
        for failure, recovers in (('drift', True), ('drift', False), ('denied', False), ('timeout', False)):
            with self.subTest(failure=failure, recovery_after_kill=recovers):
                clock = Clock()
                child = FakeChild(clock, 0)
                identity = bind_fake_fabric_child(child)
                signals = []
                killed = False
                def observe(target):
                    self.assertFalse(identity.reaped)
                    return 0 if killed else None
                def snapshot(group):
                    if not signals:
                        return [dict(pid=child.pid, pgid=group, status='S')]
                    if killed and recovers:
                        return [dict(pid=child.pid, pgid=group, status='Z')]
                    if failure == 'denied':
                        raise PermissionError(runner.errno.EPERM, 'group observation denied')
                    if failure == 'timeout':
                        raise subprocess.TimeoutExpired('ps', 2)
                    raise runner.Refused('Native process-group membership changed during observation')
                def send(group, sig):
                    nonlocal killed
                    self.assertEqual(group, child.pid)
                    self.assertFalse(identity.reaped)
                    self.assertFalse(identity.signals_closed)
                    signals.append((sig, clock.now))
                    if sig == runner.signal.SIGKILL:
                        killed = True
                def reap(target):
                    target._record_reap()
                    return 0
                reap_call = mock.Mock(side_effect=reap)
                if recovers:
                    runner.stop(child, observe=observe, send_group=send, reap=reap_call,
                                monotonic=clock.monotonic, sleep=clock.advance, snapshot_group=snapshot)
                    self.assertTrue(identity.reaped)
                    reap_call.assert_called_once_with(child)
                else:
                    with self.assertRaisesRegex(runner.Refused, 'observation uncertain after KILL'):
                        runner.stop(child, observe=observe, send_group=send, reap=reap_call,
                                    monotonic=clock.monotonic, sleep=clock.advance, snapshot_group=snapshot)
                    self.assertFalse(identity.reaped)
                    reap_call.assert_not_called()
                    self.assertGreaterEqual(clock.now, 4)
                    self.assertLess(clock.now, 4.001)
                self.assertEqual([sig for sig, _ in signals], [runner.signal.SIGTERM, runner.signal.SIGKILL])
                self.assertGreaterEqual(signals[1][1], 2)
                self.assertLess(signals[1][1], 2.001)
                self.assertTrue(identity.signals_closed)
                if recovers:
                    with self.assertRaises(runner.Refused):
                        runner._signal_fabric_group(child, runner.signal.SIGKILL, observe, send, snapshot)
                    self.assertEqual(len(signals), 2)

    def test_fabric_drift_during_final_confirmation_still_escalates(self):
        clock = Clock()
        child = FakeChild(clock, 0)
        identity = bind_fake_fabric_child(child)
        signals = []
        post_term_reads = 0
        killed = False
        def observe(target):
            return 0 if signals else None
        def snapshot(group):
            nonlocal post_term_reads
            leader = dict(pid=child.pid, pgid=group, status='Z' if signals else 'S')
            if not signals or killed:
                return [leader]
            post_term_reads += 1
            if post_term_reads == 1:
                return [leader]
            if post_term_reads == 2:
                raise runner.Refused('Native process-group membership changed during observation')
            return [leader, dict(pid=424243, pgid=group, status='S')]
        def send(group, sig):
            nonlocal killed
            self.assertFalse(identity.reaped)
            signals.append(sig)
            if sig == runner.signal.SIGKILL:
                killed = True
        def reap(target):
            target._record_reap()
            return 0
        runner.stop(child, observe=observe, send_group=send, reap=reap,
                    monotonic=clock.monotonic, sleep=clock.advance, snapshot_group=snapshot)
        self.assertEqual(signals, [runner.signal.SIGTERM, runner.signal.SIGKILL])
        self.assertTrue(identity.reaped)

    def test_fabric_drift_with_uncertain_wait_custody_never_escalates_or_reaps(self):
        clock = Clock()
        child = FakeChild(clock, 0)
        identity = bind_fake_fabric_child(child)
        signals = []
        def observe(target):
            if clock.now >= 2:
                raise runner.Refused('wait custody unavailable')
            return None
        def snapshot(group):
            if not signals:
                return [dict(pid=child.pid, pgid=group, status='S')]
            raise runner.Refused('membership changed during observation')
        def send(group, sig):
            signals.append(sig)
        reap = mock.Mock()
        with self.assertRaisesRegex(runner.Refused, 'wait custody unavailable'):
            runner.stop(child, observe=observe, send_group=send, reap=reap,
                        monotonic=clock.monotonic, sleep=clock.advance, snapshot_group=snapshot)
        self.assertEqual(signals, [runner.signal.SIGTERM])
        self.assertTrue(identity.signals_closed)
        self.assertFalse(identity.reaped)
        reap.assert_not_called()

    def test_fabric_group_snapshot_errors_keep_live_or_unknown_group_as_debt(self):
        for failure in [PermissionError(1, 'operation not permitted'),
                        ProcessLookupError(3, 'no such process'),
                        runner.Refused('membership unknown')]:
            with self.subTest(error=type(failure).__name__):
                child = FakeChild(Clock(), 0)
                identity = bind_fake_fabric_child(child)
                send = mock.Mock()
                reap = mock.Mock()
                with self.assertRaises(type(failure)):
                    runner.stop(child, observe=lambda target: 0, send_group=send,
                                reap=reap, snapshot_group=mock.Mock(side_effect=failure))
                self.assertTrue(identity.signals_closed)
                self.assertFalse(identity.reaped)
                self.assertIsNone(child.returncode)
                send.assert_not_called()
                reap.assert_not_called()

    def test_fabric_signals_close_on_initial_observer_and_checked_reap_errors(self):
        child = FakeChild(Clock(), 0)
        identity = bind_fake_fabric_child(child)
        reap = mock.Mock()
        with self.assertRaisesRegex(runner.Refused, 'initial wait unavailable'):
            runner.stop(child, observe=mock.Mock(side_effect=runner.Refused('initial wait unavailable')),
                        snapshot_group=mock.Mock(), reap=reap)
        self.assertTrue(identity.signals_closed)
        self.assertFalse(identity.reap_started)
        reap.assert_not_called()

        child = FakeChild(Clock(), 0)
        identity = bind_fake_fabric_child(child)
        with self.assertRaisesRegex(TimeoutError, 'checked reap unavailable'):
            runner.stop(child, observe=lambda target: 4,
                        snapshot_group=lambda group: [dict(pid=child.pid, pgid=group, status='Z')],
                        reap=mock.Mock(side_effect=TimeoutError('checked reap unavailable')))
        self.assertTrue(identity.signals_closed)
        self.assertTrue(identity.reap_started)
        self.assertFalse(identity.reaped)

    def test_fabric_terminal_leader_with_live_member_gets_term_observation_then_kill(self):
        child = FakeChild(Clock(), 0, exit_code=9)
        identity = bind_fake_fabric_child(child)
        clock = Clock()
        events = []
        killed = False
        def observe(target):
            self.assertFalse(identity.reaped)
            return 9
        def snapshot(group):
            events.append(('snapshot', clock.now))
            rows = [dict(pid=child.pid, pgid=group, status='Z')]
            if not killed:
                rows.append(dict(pid=424243, pgid=group, status='S'))
            return rows
        def send_group(group, sig):
            nonlocal killed
            self.assertEqual(group, child.pid)
            self.assertFalse(identity.reaped)
            events.append(('signal', sig, clock.now))
            if sig == runner.signal.SIGTERM:
                clock.now = 2
            else:
                killed = True
        def reap(target):
            self.assertTrue(identity.signals_closed)
            events.append(('reap', clock.now))
            target._record_reap()
            return 9
        runner.stop(child, observe=observe, send_group=send_group, reap=reap,
                    monotonic=clock.monotonic, sleep=clock.advance,
                    snapshot_group=snapshot)
        self.assertEqual([event[1] for event in events if event[0] == 'signal'],
                         [runner.signal.SIGTERM, runner.signal.SIGKILL])
        self.assertEqual(events[-1][0], 'reap')
        self.assertTrue(identity.reaped)

    def test_fabric_additional_zombie_after_kill_remains_cleanup_debt(self):
        child = FakeChild(Clock(), 0, exit_code=6)
        identity = bind_fake_fabric_child(child)
        clock = Clock()
        events = []
        killed = False
        def observe(target):
            return 6
        def snapshot(group):
            status = 'Z' if killed else 'S'
            return [dict(pid=child.pid, pgid=group, status='Z'),
                    dict(pid=424243, pgid=group, status=status)]
        def send_group(group, sig):
            nonlocal killed
            events.append(sig)
            if sig == runner.signal.SIGTERM:
                clock.now = 2
            else:
                killed = True
        reap = mock.Mock()
        with self.assertRaisesRegex(runner.Refused, 'Additional group zombies remain'):
            runner.stop(child, observe=observe, send_group=send_group, reap=reap,
                        monotonic=clock.monotonic, sleep=clock.advance,
                        snapshot_group=snapshot)
        self.assertEqual(events, [runner.signal.SIGTERM, runner.signal.SIGKILL])
        self.assertTrue(identity.signals_closed)
        self.assertFalse(identity.reaped)
        reap.assert_not_called()

    def test_fabric_eperm_and_esrch_require_independent_singleton_reobservation(self):
        for error in [PermissionError(1, 'operation not permitted'),
                      ProcessLookupError(3, 'no such process')]:
            with self.subTest(error=type(error).__name__):
                child = FakeChild(Clock(), 0, exit_code=12)
                identity = bind_fake_fabric_child(child)
                observations = iter([None, None, 12, 12])
                snapshots = iter([
                    [dict(pid=child.pid, pgid=child.pid, status='S')],
                    [dict(pid=child.pid, pgid=child.pid, status='Z')],
                    [dict(pid=child.pid, pgid=child.pid, status='Z')],
                ])
                def observe(target):
                    return next(observations)
                def snapshot(group):
                    return next(snapshots)
                def send_group(group, sig):
                    raise error
                def reap(target):
                    target._record_reap()
                    return 12
                runner.stop(child, observe=observe, send_group=send_group, reap=reap,
                            snapshot_group=snapshot)
                self.assertTrue(identity.reaped)
                self.assertTrue(identity.signals_closed)

    def test_fabric_eperm_or_esrch_without_independent_singleton_proof_is_debt(self):
        scenarios = [
            (PermissionError(runner.errno.EPERM, 'operation not permitted'), 'live', PermissionError),
            (ProcessLookupError(runner.errno.ESRCH, 'no such process'), 'unknown', runner.Refused),
        ]
        for error, second_state, expected in scenarios:
            with self.subTest(error=type(error).__name__):
                child = FakeChild(Clock(), 0)
                identity = bind_fake_fabric_child(child)
                initial = True
                def snapshot(group):
                    nonlocal initial
                    if initial:
                        initial = False
                        return [dict(pid=child.pid, pgid=group, status='S')]
                    if second_state == 'live':
                        return [dict(pid=child.pid, pgid=group, status='S')]
                    raise runner.Refused('independent snapshot unknown')
                def send_group(group, sig):
                    raise error
                reap = mock.Mock()
                with self.assertRaises(expected):
                    runner.stop(child, observe=lambda target: None, send_group=send_group,
                                reap=reap, snapshot_group=snapshot)
                self.assertTrue(identity.signals_closed)
                self.assertFalse(identity.reaped)
                reap.assert_not_called()

    @unittest.skipUnless(os.name == 'posix', 'POSIX process-group integration')
    def test_naturally_short_lived_real_child_reaps_singleton_zombie_without_signal(self):
        child = subprocess.Popen([sys.executable, '-c', 'pass'], start_new_session=True,
                                 stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                 stderr=subprocess.DEVNULL)
        try:
            identity = runner.fabric_bind_child(child)
            deadline = runner.time.monotonic() + 5
            while runner.fabric_child_terminal(child) is None:
                if runner.time.monotonic() >= deadline:
                    self.fail('short-lived child did not reach a WNOWAIT terminal state')
                runner.time.sleep(0.01)
            signals = mock.Mock(side_effect=AssertionError('terminal-only group was signaled'))
            runner.stop(child, send_group=signals)
            self.assertTrue(identity.reaped)
            self.assertTrue(identity.signals_closed)
            self.assertEqual(child.returncode, 0)
            signals.assert_not_called()
        finally:
            if child.returncode is None:
                child.kill()
                child.wait(timeout=2)

    def test_fabric_signal_failure_retains_unreaped_custody(self):
        child = FakeChild(Clock(), 0)
        identity = bind_fake_fabric_child(child)
        sent = []
        def send_group(group, sig):
            sent.append((group, sig))
            raise PermissionError('fixture signal refusal')
        reap = mock.Mock()
        with self.assertRaisesRegex(PermissionError, 'fixture signal refusal'):
            runner.stop(child, observe=lambda target: None, send_group=send_group, reap=reap,
                        snapshot_group=lambda group: [dict(pid=child.pid, pgid=group, status='S')])
        self.assertEqual(sent, [(child.pid, runner.signal.SIGTERM)])
        self.assertTrue(identity.signals_closed)
        self.assertFalse(identity.reaped)
        self.assertIsNone(child.returncode)
        reap.assert_not_called()

    def test_fabric_final_terminal_uncertainty_never_reaps(self):
        child = FakeChild(Clock(), 0)
        identity = bind_fake_fabric_child(child)
        clock = Clock()
        observations = iter([None, None, runner.Refused('terminal receipt unavailable')])
        def observe(target):
            result = next(observations)
            if isinstance(result, Exception):
                raise result
            return result
        reap = mock.Mock()
        def send_group(group, sig):
            clock.now = 2
        with self.assertRaisesRegex(runner.Refused, 'terminal receipt unavailable'):
            runner.stop(child, observe=observe, send_group=send_group, reap=reap,
                        monotonic=clock.monotonic, sleep=clock.advance,
                        snapshot_group=lambda group: [dict(pid=child.pid, pgid=group, status='S')])
        self.assertTrue(identity.signals_closed)
        self.assertFalse(identity.reaped)
        reap.assert_not_called()

    def test_fabric_grace_expiry_escalates_before_final_reap(self):
        child = FakeChild(Clock(), 0)
        identity = bind_fake_fabric_child(child)
        clock = Clock()
        events = []
        killed = False
        def observe(target):
            self.assertIs(target, child)
            return 0 if killed else None
        def snapshot(group):
            status = 'Z' if killed else 'S'
            return [dict(pid=child.pid, pgid=group, status=status)]
        def send_group(group, sig):
            nonlocal killed
            self.assertEqual(group, child.pid)
            self.assertFalse(identity.reaped)
            events.append(('signal', sig, clock.now))
            if sig == runner.signal.SIGKILL:
                killed = True
        def reap(target):
            self.assertGreaterEqual(clock.now, 2)
            self.assertLess(clock.now, 2.001)
            self.assertEqual(events[-1][0:2], ('signal', runner.signal.SIGKILL))
            events.append(('reap', clock.now))
            target._record_reap()
            return target.returncode
        runner.stop(child, observe=observe, send_group=send_group, reap=reap,
                    monotonic=clock.monotonic, snapshot_group=snapshot,
                    sleep=clock.advance)
        self.assertEqual(events[0][0:2], ('signal', runner.signal.SIGTERM))
        self.assertEqual(events[-1][0], 'reap')
        self.assertGreaterEqual(events[-1][1], 2)
        self.assertLess(events[-1][1], 2.001)

    def test_fabric_echild_is_not_inferred_as_terminal_or_safe_custody(self):
        child = FakeChild(Clock(), 0)
        bind_fake_fabric_child(child)
        wait = mock.Mock(side_effect=ChildProcessError('fixture ECHILD'))
        with mock.patch.object(runner.sys, 'platform', 'linux'), \
                mock.patch.object(runner.os, 'P_PID', 1, create=True), \
                mock.patch.object(runner.os, 'WEXITED', 4, create=True), \
                mock.patch.object(runner.os, 'WNOHANG', 1, create=True), \
                mock.patch.object(runner.os, 'WNOWAIT', 0x01000000, create=True):
            with self.assertRaises(runner.LocalWaitRefused):
                runner.fabric_child_terminal(child, wait)
        self.assertIsNone(child.returncode)
        self.assertEqual(child.poll_calls, 0)
        self.assertEqual(child.wait_timeouts, [])

    def test_nonzero_payload_status_survives_fabric_cleanup(self):
        clock = Clock()
        backend = Backend(admission())
        child = FakeChild(clock, 0, exit_code=17)
        record = {}
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                mock.patch.object(runner, 'fabric_bind_child', side_effect=bind_fake_fabric_child), \
                mock.patch.object(runner, 'release_fabric_child'), \
                mock.patch.object(runner.os, 'killpg') as kill_group, \
                mock.patch.object(runner, 'fabric_child_terminal', side_effect=lambda target: target.terminal_status()), \
                mock.patch.object(runner, 'local_group_snapshot',
                                  side_effect=lambda group: [dict(pid=child.pid, pgid=group, status='Z')]):
            code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 30, backend, record)
        self.assertEqual(code, 17)
        self.assertEqual(record['status'], 'child_failed')
        self.assertEqual(backend.released, [])
        self.assertEqual(child.returncode, 17)
        kill_group.assert_not_called()

    def test_fabric_terminal_observation_uses_nonreaping_waitid(self):
        child = FakeChild(Clock(), 0, exit_code=17)
        child.returncode = None
        bind_fake_fabric_child(child)
        info = mock.Mock(si_pid=child.pid, si_uid=os.getuid(), si_signo=runner.signal.SIGCHLD,
                         si_code=1, si_status=17)
        wait = mock.Mock(return_value=info)
        with mock.patch.object(runner.sys, 'platform', 'linux'), \
                mock.patch.object(runner.os, 'P_PID', 1, create=True), \
                mock.patch.object(runner.os, 'WEXITED', 4, create=True), \
                mock.patch.object(runner.os, 'WNOHANG', 1, create=True), \
                mock.patch.object(runner.os, 'WNOWAIT', 0x01000000, create=True):
            self.assertEqual(runner.fabric_child_terminal(child, wait), 17)
        wait.assert_called_once_with(1, child.pid, 4 | 1 | 0x01000000)
        self.assertEqual(child.poll_calls, 0)
        self.assertEqual(child.wait_timeouts, [])

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

    def test_confirmed_renewal_replaces_last_recorded_expiry_but_readback_does_not(self):
        clock = Clock()
        initial = admission(lease_ttl_sec=120)
        renewed = admission(lease_ttl_sec=240)
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic):
            lease = runner.ProvenLease(initial, 0, 'm5', 'lane', 'work', None, True)
            lease.renewal_outcome = 'unknown'
            clock.now = 1
            lease.confirm(renewed, 1)
            last = dict(lease.last_confirmation)
            lease.check(admission(lease_ttl_sec=300))
        self.assertEqual(lease.renewal_outcome, 'confirmed')
        self.assertEqual(last['heartbeat_at'], renewed['heartbeat_at'])
        self.assertEqual(last['lease_ttl_sec'], 240)
        self.assertEqual(lease.last_confirmation, last)
        stamp = dt.datetime.fromisoformat(renewed['heartbeat_at'].replace('Z', '+00:00'))
        self.assertEqual(last['heartbeat_plus_ttl'], (stamp + dt.timedelta(seconds=240)).isoformat())

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
        for payload in ('', '{}', '[]', json.dumps([admission(id='foreign')]),
                        json.dumps([admission(), admission()]),
                        json.dumps([admission(), admission(id='foreign')]),
                        json.dumps([None]), '{malformed'):
            with self.subTest(payload=payload):
                response = mock.Mock(stdout=payload)
                with mock.patch.object(runner.subprocess, 'run', return_value=response) as run:
                    with self.assertRaises(runner.Refused):
                        native.lookup('m5')
                self.assertEqual(run.call_args.args[0],
                                 [native.cli, 'maat', 'status', '--id', 'fixture-id', '--json'])
        failure = subprocess.CalledProcessError(1, 'sirsi', stderr='unknown reservation')
        with mock.patch.object(runner.subprocess, 'run', side_effect=failure):
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
