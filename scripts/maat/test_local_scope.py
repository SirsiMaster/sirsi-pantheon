"""Owned-file/fake-child tests. No CLI, router, service, model or network calls."""
import contextlib
import fcntl
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('local_scope_runner', Path(__file__).with_name('admitted-run.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


def bind_child(child, birth):
    child.returncode=None
    child._sirsi_local_identity=runner._LocalChildIdentity(child,child.pid,os.getpid(),os.getuid(),os.getuid(),child.pid,birth)


def owned_child(birth='old'):
    child=mock.Mock(pid=24680)
    bind_child(child,birth)
    return child


class LocalScopeTests(unittest.TestCase):
    def setUp(self):
        identity = mock.patch.object(runner, 'process_identity', return_value=(os.getpid(), 'fixture-start'))
        wait = mock.patch.object(runner, 'local_wait_terminal', side_effect=lambda child, timeout: child.wait(timeout=timeout))
        identity.start(); wait.start()
        self.addCleanup(identity.stop); self.addCleanup(wait.stop)
        binding=mock.patch.object(runner,'local_bind_owned_child',side_effect=bind_child)
        binding.start();self.addCleanup(binding.stop)
        release=mock.patch.object(runner,'release_local_child')
        self.release=release.start();self.addCleanup(release.stop)

    def main(self, home, flags=(), child=None):
        child = child or mock.Mock(pid=12345)
        child.wait.return_value = 0
        with mock.patch.dict(os.environ, {'HOME': str(home), 'SIRSI_ROUTER_URL': 'https://unreachable.invalid',
                                         'SIRSI_MAAT_EXECUTION_RECEIPT': '/not-this-execution'}, clear=True), \
                mock.patch.object(runner.sys, 'argv', ['sirsi-maat-run', *flags, '--work', 'owned-fixture', '--', '/owned/fake-work']), \
                mock.patch.object(runner.signal, 'signal'), \
                mock.patch.object(runner.socket, 'gethostname', return_value='new-unenrolled-mac'), \
                mock.patch.object(runner.socket, 'create_connection', side_effect=AssertionError('network attempted')), \
                mock.patch.object(runner, 'Native', side_effect=AssertionError('fabric client constructed')), \
                mock.patch.object(runner, 'local_resource', side_effect=AssertionError('enrollment mapping attempted')), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=child) as spawn, \
                mock.patch.object(runner, 'local_stop_owned') as stop, contextlib.redirect_stderr(io.StringIO()):
            result = runner.main()
        return result, spawn, stop

    def test_actual_entry_offline_local_without_registry_or_enrolled_hostname(self):
        with tempfile.TemporaryDirectory() as raw:
            home = Path(raw)
            result, spawn, stop = self.main(home, ['--scope', 'local'])
            self.assertEqual(result, 0)
            spawn.assert_called_once()
            stop.assert_called_once_with(spawn.return_value, 'fixture-start')
            env = spawn.call_args.kwargs['env']
            self.assertNotIn('SIRSI_MAAT_EXECUTION_RECEIPT', env)
            self.assertNotIn('SIRSI_THREAD_ID', env)
            self.assertNotIn('SIRSI_ROUTER_DB', env)
            self.assertEqual(env['SIRSI_ROUTER_URL'], 'https://unreachable.invalid')
            self.assertEqual(env['GOMAXPROCS'], '1')
            self.assertEqual(len(spawn.call_args.kwargs['pass_fds']), 2)
            paths = list((home/'.sirsi/evidence/maat-runs').glob('*/terminal.json'))
            self.assertEqual(len(paths), 1)
            value = json.loads(paths[0].read_text())
            self.assertEqual(value['status'], 'success')
            self.assertEqual(value['scope'], 'local')
            self.assertEqual(value['authority'], 'local-user-process-lock')
            self.assertEqual(value['fabric_delivery'], 'not_requested')
            self.assertEqual(value['uid'], os.getuid())
            self.assertEqual(value['resource'], 'local-user-cpu')
            self.assertNotIn('admission', value)  # No fabricated fabric reservation ID.
            self.assertNotIn('executor_thread', value)

    def test_local_options_cannot_select_fabric_or_model_authority(self):
        for extra in (['--regime', 'quiet'], ['--regime', 'loaded'], ['--cpu-cores', '2'],
                      ['--register-executor'], ['--iterations', '2'], ['--job-timeout', '30']):
            with self.subTest(extra=extra), tempfile.TemporaryDirectory() as raw:
                with self.assertRaises(SystemExit) as refused:
                    self.main(Path(raw), ['--scope', 'local', *extra])
                self.assertEqual(refused.exception.code, 2)
                self.assertFalse((Path(raw)/'.sirsi').exists())

    def test_default_fabric_transport_failure_never_selects_local(self):
        with tempfile.TemporaryDirectory() as raw, \
                mock.patch.dict(os.environ, {'HOME': raw}, clear=True), \
                mock.patch.object(runner.sys, 'argv', ['sirsi-maat-run', '--agent', 'fixture', '--thread', 'fixture-thread', '--work', 'work', '--', '/owned/fake-work']), \
                mock.patch.object(runner.signal, 'signal'), \
                mock.patch.object(runner, 'local_resource', return_value='fixture-host'), \
                mock.patch.object(runner, 'Native') as native, \
                mock.patch.object(runner, 'supervise_iterations', side_effect=runner.Refused('canonical service unreachable')) as fabric, \
                mock.patch.object(runner, 'supervise_local', side_effect=AssertionError('silent local fallback')), \
                contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(runner.main(), 78)
            native.assert_called_once()
            fabric.assert_called_once()
            native.return_value.close_executor.assert_called_once()

    def local_fixture(self, home):
        root = home/'runs'
        root.mkdir(mode=0o700)
        folder = root/'owned-execution'
        folder.mkdir(mode=0o700)
        return root, folder

    def test_lock_excludes_second_descriptor_and_intent_is_durable_before_spawn(self):
        with tempfile.TemporaryDirectory() as raw:
            root, folder = self.local_fixture(Path(raw))
            record, persisted = {}, []
            child = mock.Mock(pid=12345)
            child.wait.return_value = 0

            def spawn(*args, **kwargs):
                self.assertEqual(persisted[-1]['status'], 'admitted')
                self.assertEqual(persisted[-1]['local_lock']['inode'], os.fstat(kwargs['pass_fds'][0]).st_ino)
                with (root/'.local-cpu.lock').open('r+b') as competing:
                    with self.assertRaises(BlockingIOError):
                        fcntl.flock(competing.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                return child

            with mock.patch.object(runner.subprocess, 'Popen', side_effect=spawn), mock.patch.object(runner, 'local_stop_owned'):
                self.assertEqual(runner.supervise_local(['/owned/fake'], 10, record, folder, lambda: persisted.append(dict(record))), 0)
            with (root/'.local-cpu.lock').open('r+b') as released:
                fcntl.flock(released.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            self.assertEqual(persisted[-1]['exit_code'], 0)

    def test_busy_local_lock_refuses_without_spawn_or_remote_lookup(self):
        with tempfile.TemporaryDirectory() as raw:
            root, folder = self.local_fixture(Path(raw))
            lock = os.open(root/'.local-cpu.lock', os.O_CREAT | os.O_RDWR, 0o600)
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                record = {}
                with mock.patch.object(runner.subprocess, 'Popen') as spawn:
                    self.assertEqual(runner.supervise_local(['/owned/fake'], 10, record, folder, lambda: None), 78)
                    spawn.assert_not_called()
                self.assertIn('Another same-user local CPU job', record['reason'])
            finally:
                os.close(lock)

    def test_local_lock_nonregular_link_and_public_mode_refuse_before_spawn(self):
        for kind in ('symlink', 'hardlink', 'public', 'fifo'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as raw:
                root, folder = self.local_fixture(Path(raw))
                other = root/'other'
                other.write_bytes(b'owned fixture')
                other.chmod(0o600)
                leaf = root/'.local-cpu.lock'
                if kind == 'symlink':
                    leaf.symlink_to(other)
                elif kind == 'hardlink':
                    os.link(other, leaf)
                elif kind == 'fifo':
                    os.mkfifo(leaf, 0o600)
                else:
                    leaf.write_bytes(b'')
                    leaf.chmod(0o644)
                record = {}
                with mock.patch.object(runner.subprocess, 'Popen') as spawn:
                    self.assertEqual(runner.supervise_local(['/owned/fake'], 10, record, folder, lambda: None), 78)
                    spawn.assert_not_called()
                self.assertEqual(other.read_bytes(), b'owned fixture')

    def test_failed_intent_persistence_cannot_launch(self):
        with tempfile.TemporaryDirectory() as raw:
            _, folder = self.local_fixture(Path(raw))
            record, calls = {}, []

            def persist():
                calls.append(record['status'])
                if len(calls) == 1:
                    raise OSError('inert persistence refusal')

            with mock.patch.object(runner.subprocess, 'Popen') as spawn:
                self.assertEqual(runner.supervise_local(['/owned/fake'], 10, record, folder, persist), 78)
                spawn.assert_not_called()
            self.assertEqual(calls, ['admitted', 'refused_or_stopped'])

    def test_timeout_and_cleanup_debt_cannot_report_local_success(self):
        with tempfile.TemporaryDirectory() as raw:
            _, folder = self.local_fixture(Path(raw))
            child = mock.Mock(pid=12345)
            child.wait.side_effect = subprocess.TimeoutExpired('/owned/fake', 1)
            record = {}
            with mock.patch.object(runner.subprocess, 'Popen', return_value=child), \
                    mock.patch.object(runner, 'local_stop_owned', side_effect=OSError('inert cleanup unavailable')) as stop:
                self.assertEqual(runner.supervise_local(['/owned/fake'], 1, record, folder, lambda: None), 78)
                stop.assert_called_once_with(child, 'fixture-start')
            self.assertEqual(record['status'], 'refused_or_stopped')
            self.assertIn('cleanup unconfirmed', record['cleanup_debt'])

    def test_child_failure_preserves_exact_code_and_cleans_only_owned_child(self):
        with tempfile.TemporaryDirectory() as raw:
            _, folder = self.local_fixture(Path(raw))
            child = mock.Mock(pid=12345)
            child.wait.return_value = 7
            record = {}
            with mock.patch.object(runner.subprocess, 'Popen', return_value=child), mock.patch.object(runner, 'local_stop_owned') as stop:
                self.assertEqual(runner.supervise_local(['/owned/fake'], 10, record, folder, lambda: None), 7)
                stop.assert_called_once_with(child, 'fixture-start')
            self.assertEqual(record['child_exit_code'], 7)
            self.assertEqual(record['status'], 'child_failed')


class LocalContinuationTests(unittest.TestCase):
    setUp = LocalScopeTests.setUp
    main = LocalScopeTests.main
    def test_elevated_local_refuses_before_any_evidence_or_payload(self):
        for uid,euid in ((0,0),(501,0),(501,502)):
            with self.subTest(uid=uid,euid=euid), tempfile.TemporaryDirectory() as raw, \
                    mock.patch.object(runner.os,'getuid',return_value=uid), mock.patch.object(runner.os,'geteuid',return_value=euid):
                with self.assertRaises(SystemExit):self.main(Path(raw),['--scope','local'])
                self.assertFalse((Path(raw)/'.sirsi').exists())

    def test_auto_qualified_tier1_is_local_but_remote_and_old_capability_are_fabric(self):
        args=mock.Mock(scope='auto',register_executor=False,iterations=1,cpu_cores=1,job_timeout=None)
        with tempfile.TemporaryDirectory() as raw:
            cli=Path(raw)/'Pantheon.app/Contents/MacOS/sirsi';cli.parent.mkdir(parents=True);cli.write_bytes(b'fixture');cli.chmod(0o755)
            with mock.patch.object(runner.subprocess,'run',return_value=mock.Mock(returncode=0,stdout=json.dumps({'local_admission_schema':runner.LOCAL_RECEIPT_SCHEMA}))) as probe:
                self.assertEqual(runner.select_scope(args,[str(cli),'vitals','--json'],'build'),'local')
                self.assertEqual(probe.call_args.args[0],[str(cli),'version','--json'])
                for command in (['router','task','claim'],['maat','reserve','studio'],['ask','model'],['launchd','refresh']):
                    probe.reset_mock();self.assertEqual(runner.select_scope(args,[str(cli),*command],'build'),'fabric');probe.assert_not_called()
            with mock.patch.object(runner.subprocess,'run',return_value=mock.Mock(returncode=0,stdout='{}')):
                self.assertEqual(runner.select_scope(args,[str(cli),'vitals'],'build'),'fabric')

    def test_actual_auto_entry_publishes_exact_private_local_fact_before_core_launch(self):
        with tempfile.TemporaryDirectory() as raw:
            home=Path(raw);cli=home/'Pantheon.app/Contents/MacOS/sirsi';cli.parent.mkdir(parents=True);cli.write_bytes(b'fixture');cli.chmod(0o755)
            command=[str(cli),'vitals','--json'];child=mock.Mock(pid=24680);child.wait.return_value=0
            def spawn(argv,**kw):
                self.assertEqual(argv[-len(command):],command)
                self.assertIn('--owned-local-child',argv)
                fact_path=Path(kw['env']['SIRSI_MAAT_LOCAL_EXECUTION_RECEIPT']);fact=json.loads(fact_path.read_text())
                self.assertEqual(fact['schema'],runner.LOCAL_RECEIPT_SCHEMA)
                self.assertEqual(fact['supervisor_pid'],os.getpid())
                self.assertEqual(fact['supervisor_start'],'fixture-start')
                self.assertEqual(fact['argv_sha256'],runner.hashlib.sha256(b'\0'.join(os.fsencode(x) for x in command)).hexdigest())
                self.assertEqual(fact_path.stat().st_mode&0o777,0o600)
                self.assertNotIn('SIRSI_MAAT_EXECUTION_RECEIPT',kw['env'])
                return child
            with mock.patch.dict(os.environ,{'HOME':raw,'SIRSI_ROUTER_URL':'https://unreachable.invalid'},clear=True), \
                    mock.patch.object(runner.sys,'argv',['sirsi-maat-run','--work','work','--',*command]),mock.patch.object(runner.signal,'signal'), \
                    mock.patch.object(runner,'Native',side_effect=AssertionError('fabric client constructed')),mock.patch.object(runner,'local_resource',side_effect=AssertionError('remote enrollment mapping')), \
                    mock.patch.object(runner.subprocess,'run',return_value=mock.Mock(returncode=0,stdout=json.dumps({'local_admission_schema':runner.LOCAL_RECEIPT_SCHEMA}))), \
                    mock.patch.object(runner.subprocess,'Popen',side_effect=spawn) as start,mock.patch.object(runner,'local_stop_owned'),contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(runner.main(),0);start.assert_called_once()

    def test_after_spawn_partial_running_write_keeps_intent_and_never_reports_success(self):
        real=runner.local_transition;retained=[]
        def transition(folder,record):
            if record['status']=='running':
                prior=(folder/'intent.json').read_bytes()
                def partial(file,data):file.write(data[:5]);raise OSError('inert partial running write')
                try:real(folder,record,write=partial)
                finally:retained.append(((folder/'intent.json').read_bytes(),prior))
            else:real(folder,record)
        with tempfile.TemporaryDirectory() as raw,mock.patch.object(runner,'local_transition',side_effect=transition):
            result,spawn,_=self.main(Path(raw),['--scope','local'])
            self.assertEqual(result,78);spawn.assert_called_once()
            self.assertEqual(len(retained),1);self.assertEqual(*retained[0])
            self.release.assert_not_called()


class LocalCustodyTests(unittest.TestCase):
    def test_reaped_or_reused_anchor_never_signals(self):
        child=owned_child()
        row=dict(pid=child.pid,uid=os.getuid(),status='S')
        for rows,birth in (([],'old'),([row],'new')):
            with self.subTest(rows=rows),mock.patch.object(runner,'local_group_snapshot',return_value=rows), \
                    mock.patch.object(runner,'process_identity',return_value=(os.getpid(),birth)),mock.patch.object(runner.os,'killpg') as kill:
                with self.assertRaises(runner.Refused):runner.local_stop_owned(child,'old')
                kill.assert_not_called();child.wait.assert_not_called()

    def test_delayed_group_quiescence_precedes_reap_and_no_unnecessary_signal(self):
        child=owned_child()
        leader=dict(pid=child.pid,uid=os.getuid(),status='Z')
        member=dict(pid=24681,uid=os.getuid(),status='S')
        snapshots=[[leader,member],[leader,member],[leader],[leader],[]]
        with mock.patch.object(runner,'local_group_snapshot',side_effect=snapshots), \
                mock.patch.object(runner,'process_identity',return_value=(os.getpid(),'old')), \
                mock.patch.object(runner.time,'sleep'),mock.patch.object(runner.os,'killpg') as kill:
            runner.local_stop_owned(child,'old')
            child.wait.assert_called_once_with(timeout=1);kill.assert_not_called()

    def test_timeout_cleanup_signals_only_revalidated_owned_anchor(self):
        child=owned_child();state={'zombie':False,'reaped':False};clock=iter(range(100))
        def snapshot(group):
            return [] if state['reaped'] else [dict(pid=child.pid,uid=os.getuid(),status='Z' if state['zombie'] else 'S')]
        def kill(group,sig):
            self.assertEqual(group,child.pid);self.assertEqual(sig,runner.signal.SIGTERM);state['zombie']=True
        child.wait.side_effect=lambda timeout:state.update(reaped=True)
        with mock.patch.object(runner,'local_group_snapshot',side_effect=snapshot),mock.patch.object(runner,'process_identity',return_value=(os.getpid(),'old')) as identity, \
                mock.patch.object(runner.os,'killpg',side_effect=kill) as signal_group,mock.patch.object(runner.time,'sleep'),mock.patch.object(runner.time,'monotonic',side_effect=lambda:next(clock)/10):
            runner.local_stop_owned(child,'old')
            signal_group.assert_called_once_with(child.pid,runner.signal.SIGTERM)
            self.assertGreaterEqual(identity.call_count,2)

    def test_cleanup_debt_blocks_next_job_until_exact_quiescent_reconciliation(self):
        with tempfile.TemporaryDirectory() as raw,mock.patch.dict(os.environ,{'HOME':raw},clear=True):
            root=Path(raw)/'.sirsi/evidence/maat-runs';root.mkdir(parents=True,mode=0o700)
            folder=root/('a'*32);folder.mkdir(mode=0o700)
            child=mock.Mock(pid=24680);record={}
            with mock.patch.object(runner.subprocess,'Popen',return_value=child),mock.patch.object(runner,'process_identity',return_value=(os.getpid(),'old')), \
                    mock.patch.object(runner,'local_wait_terminal',return_value=0),mock.patch.object(runner,'local_bind_owned_child',side_effect=bind_child),mock.patch.object(runner,'release_local_child'),mock.patch.object(runner,'local_stop_owned',side_effect=runner.Refused('unknown group')):
                self.assertEqual(runner.supervise_local(['/owned/fake'],10,record,folder,lambda:None),78)
            next_folder=root/('b'*32);next_folder.mkdir(mode=0o700)
            with mock.patch.object(runner.subprocess,'Popen') as spawn:
                self.assertEqual(runner.supervise_local(['/owned/fake'],10,{},next_folder,lambda:None),78);spawn.assert_not_called()
            with mock.patch.object(runner,'local_group_snapshot',return_value=[{'pid':24680}]),mock.patch.object(runner.os,'killpg') as kill:
                with self.assertRaises(runner.Refused):runner.reconcile_local_debt('a'*32)
                kill.assert_not_called()
            with mock.patch.object(runner,'local_group_snapshot',return_value=[]):runner.reconcile_local_debt('a'*32)
            self.assertFalse((root/'.local-cpu.debt.json').exists())
            self.assertTrue((folder/'cleanup-reconciled.json').exists())
            self.assertEqual((root/'.local-cpu.lock').read_bytes(),b'')

    def test_immutable_intent_survives_partial_sync_and_publication_failures(self):
        for phase in ('partial','sync','publish'):
            with self.subTest(phase=phase),tempfile.TemporaryDirectory() as raw:
                folder=Path(raw);runner.local_transition(folder,{'status':'admitted','exact':'original'})
                prior=(folder/'intent.json').read_bytes()
                def partial(file,data):file.write(data[:4]);raise OSError('partial write')
                def fail(*args):raise OSError('inert boundary failure')
                kwargs={'write':partial} if phase=='partial' else {'sync':fail} if phase=='sync' else {'publish':fail}
                with self.assertRaises(OSError):runner.local_transition(folder,{'status':'running','pid':24680},**kwargs)
                self.assertEqual((folder/'intent.json').read_bytes(),prior)
                self.assertFalse((folder/'running.json').exists())
                self.assertEqual((folder/'intent.json').stat().st_mode&0o777,0o600)

    def test_child_handshake_eof_never_executes_and_release_executes_exact_payload(self):
        for token in (b'',b'1'):
            read,write=os.pipe()
            try:
                if token:os.write(write,token)
                os.close(write);write=None
                with mock.patch.object(runner.os,'execvpe') as execute:
                    self.assertEqual(runner.owned_local_child(read,['/owned/fake','arg']),78)
                    read=None
                    if token:execute.assert_called_once();self.assertEqual(execute.call_args.args[:2],('/owned/fake',['/owned/fake','arg']))
                    else:execute.assert_not_called()
            finally:
                for fd in (read,write):
                    if fd is not None:os.close(fd)

    def test_unreleased_durable_protocol_intent_can_reconcile_without_replay(self):
        with tempfile.TemporaryDirectory() as raw,mock.patch.dict(os.environ,{'HOME':raw},clear=True):
            root=Path(raw)/'.sirsi/evidence/maat-runs';root.mkdir(parents=True,mode=0o700)
            folder=root/('c'*32);folder.mkdir(mode=0o700)
            lock=root/'.local-cpu.lock';lock.write_text('c'*32+'\n');lock.chmod(0o600)
            runner.local_transition(folder,dict(status='admitted',scope='local',uid=os.getuid(),execution='c'*32,spawn_protocol='pipe-after-durable-running-v1'))
            with mock.patch.object(runner.subprocess,'Popen') as spawn,mock.patch.object(runner.os,'killpg') as signal_group:
                runner.reconcile_local_debt('c'*32)
                spawn.assert_not_called();signal_group.assert_not_called()
            self.assertEqual(json.loads((folder/'cleanup-reconciled.json').read_text())['resolution'],'payload_not_released_by_protocol')
            self.assertEqual(lock.read_bytes(),b'')


if __name__ == '__main__':
    unittest.main()
