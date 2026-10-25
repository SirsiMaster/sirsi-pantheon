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


class LocalScopeTests(unittest.TestCase):
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
                mock.patch.object(runner, 'stop') as stop, contextlib.redirect_stderr(io.StringIO()):
            result = runner.main()
        return result, spawn, stop

    def test_actual_entry_offline_local_without_registry_or_enrolled_hostname(self):
        with tempfile.TemporaryDirectory() as raw:
            home = Path(raw)
            result, spawn, stop = self.main(home, ['--scope', 'local'])
            self.assertEqual(result, 0)
            spawn.assert_called_once()
            stop.assert_called_once_with(spawn.return_value)
            env = spawn.call_args.kwargs['env']
            self.assertNotIn('SIRSI_MAAT_EXECUTION_RECEIPT', env)
            self.assertNotIn('SIRSI_THREAD_ID', env)
            self.assertNotIn('SIRSI_ROUTER_DB', env)
            self.assertEqual(env['SIRSI_ROUTER_URL'], 'https://unreachable.invalid')
            self.assertEqual(env['GOMAXPROCS'], '1')
            self.assertEqual(len(spawn.call_args.kwargs['pass_fds']), 1)
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

            with mock.patch.object(runner.subprocess, 'Popen', side_effect=spawn), mock.patch.object(runner, 'stop'):
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
                    mock.patch.object(runner, 'stop', side_effect=OSError('inert cleanup unavailable')) as stop:
                self.assertEqual(runner.supervise_local(['/owned/fake'], 1, record, folder, lambda: None), 78)
                stop.assert_called_once_with(child)
            self.assertEqual(record['status'], 'refused_or_stopped')
            self.assertIn('cleanup unconfirmed', record['cleanup_debt'])

    def test_child_failure_preserves_exact_code_and_cleans_only_owned_child(self):
        with tempfile.TemporaryDirectory() as raw:
            _, folder = self.local_fixture(Path(raw))
            child = mock.Mock(pid=12345)
            child.wait.return_value = 7
            record = {}
            with mock.patch.object(runner.subprocess, 'Popen', return_value=child), mock.patch.object(runner, 'stop') as stop:
                self.assertEqual(runner.supervise_local(['/owned/fake'], 10, record, folder, lambda: None), 7)
                stop.assert_called_once_with(child)
            self.assertEqual(record['child_exit_code'], 7)
            self.assertEqual(record['status'], 'child_failed')


if __name__ == '__main__':
    unittest.main()
