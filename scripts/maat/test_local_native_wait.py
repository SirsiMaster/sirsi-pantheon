"""Public Darwin wait ABI and ps grammar, with inert per-call syscall fixtures."""
import ctypes
import errno
import importlib.util
import os
from pathlib import Path
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('native_wait_runner', Path(__file__).with_name('admitted-run.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class NativeWaitTests(unittest.TestCase):
    def test_ps_accepts_signed_foreign_uid_but_keeps_selected_group_custody(self):
        own = f'24680 {os.getpid()} 24680 {os.getuid()} Z\n'
        with mock.patch.object(runner.subprocess, 'run', return_value=mock.Mock(returncode=0, stdout='\n644 1 644 -2 S\n'+own)) as probe:
            rows = runner.local_group_snapshot(24680)
            self.assertEqual(rows, [dict(pid=24680, ppid=os.getpid(), pgid=24680, uid=os.getuid(), status='Z')])
            self.assertEqual(probe.call_args.args[0], ['/bin/ps', '-axo', 'pid=,ppid=,pgid=,uid=,stat='])
        with mock.patch.object(runner.subprocess, 'run', return_value=mock.Mock(returncode=0, stdout='24680 1 24680 -2 Z\n')):
            self.assertEqual(runner.local_group_snapshot(24680)[0]['uid'], -2)
        for raw in ('', '\n', 'bad row\n', '-1 1 42 501 S\n', '1 -1 42 501 S\n', '1 1 -42 501 S\n', '1 1 42 unknown S\n'):
            with self.subTest(raw=raw), mock.patch.object(runner.subprocess, 'run', return_value=mock.Mock(returncode=0, stdout=raw)):
                with self.assertRaises(runner.Refused): runner.local_group_snapshot(24680)
        child = mock.Mock(pid=24680)
        with mock.patch.object(runner, 'local_group_snapshot', return_value=[dict(pid=24680, uid=-2, status='Z')]), \
                mock.patch.object(runner, 'process_identity', return_value=(os.getpid(), 'start')), mock.patch.object(runner.os, 'killpg') as signals:
            with self.assertRaises(runner.Refused): runner.local_stop_owned(child, 'start')
            signals.assert_not_called(); child.wait.assert_not_called()

    def test_waitid_exact_public_abi_status_and_nonreaping_options(self):
        child = mock.Mock(pid=24680)
        cases = [
            ('alive', {}, None),
            ('exit-zero', {'code': 1, 'status': 0}, 0),
            ('exit-seven', {'code': 1, 'status': 7}, 7),
            ('signal', {'code': 2, 'status': 9}, -9),
            ('core', {'code': 3, 'status': 11}, -11),
        ]
        for name, values, expected in cases:
            with self.subTest(name=name), mock.patch.object(runner.sys, 'platform', 'darwin'):
                def wait(kind, pid, pointer, options):
                    self.assertEqual((kind, pid, options), (1, child.pid, 0x25))
                    self.assertEqual(ctypes.sizeof(runner._Siginfo), 104)
                    self.assertEqual(runner._Siginfo.pid.offset, 12)
                    self.assertEqual(runner._Siginfo.status.offset, 20)
                    info = ctypes.cast(pointer, ctypes.POINTER(runner._Siginfo)).contents
                    if values:
                        info.pid = child.pid; info.uid = os.getuid(); info.signo = runner.signal.SIGCHLD
                        for key, value in values.items(): setattr(info, key, value)
                    return 0
                self.assertEqual(runner.local_child_terminal(child, wait=wait), expected)
        child.wait.assert_not_called()

    def test_waitid_foreign_malformed_and_echild_never_infer_exit_or_absence(self):
        child = mock.Mock(pid=24680)
        cases = [{'pid': 24681}, {'uid': os.getuid()+1}, {'signo': 0}, {'code': 6},
                 {'error': 1}, {'status': 256}, {'pid': 0, 'uid': os.getuid()}]
        for overrides in cases:
            with self.subTest(overrides=overrides), mock.patch.object(runner.sys, 'platform', 'darwin'):
                def wait(_kind, _pid, pointer, _options):
                    info = ctypes.cast(pointer, ctypes.POINTER(runner._Siginfo)).contents
                    info.pid = child.pid; info.uid = os.getuid(); info.signo = runner.signal.SIGCHLD; info.code = 1
                    for key, value in overrides.items(): setattr(info, key, value)
                    return 0
                with self.assertRaises(runner.LocalWaitRefused) as failure: runner.local_child_terminal(child, wait=wait)
                observed=failure.exception.observation
                self.assertEqual(observed['expected_pid'],child.pid)
                self.assertEqual(observed['expected_uid'],os.getuid())
                self.assertEqual(observed['abi_bytes'],104)
                self.assertEqual(set(observed),{'api','result','errno','expected_pid','expected_uid','expected_signal',
                                               'pid','uid','signo','error','code','status','abi_bytes','pid_offset','status_offset'})
                for key,value in overrides.items():self.assertEqual(observed[key],value)
        previous = ctypes.get_errno()
        try:
            def echild(*_args): ctypes.set_errno(errno.ECHILD); return -1
            with mock.patch.object(runner.sys, 'platform', 'darwin'):
                with self.assertRaisesRegex(runner.Refused, 'no exit or absence inferred'):
                    runner.local_child_terminal(child, wait=echild)
        finally: ctypes.set_errno(previous)
        child.wait.assert_not_called()

    def test_libproc_omission_requires_positive_exact_child_wait_not_missing_read(self):
        child = mock.Mock(pid=24680)
        for terminal in (None, 0):
            with self.subTest(terminal=terminal), mock.patch.object(runner, 'process_identity', side_effect=runner.Refused('unavailable')), \
                    mock.patch.object(runner, 'local_child_terminal', return_value=terminal) as wait:
                self.assertEqual(runner.local_anchor_matches(child, 'retained-before-release'), terminal is not None)
                wait.assert_called_once_with(child)
        with mock.patch.object(runner, 'process_identity', return_value=(os.getpid(), 'different-start')), \
                mock.patch.object(runner, 'local_child_terminal') as wait:
            self.assertFalse(runner.local_anchor_matches(child, 'retained-before-release'))
            wait.assert_not_called()
        with mock.patch.object(runner, 'process_identity', side_effect=runner.Refused('unavailable')), \
                mock.patch.object(runner, 'local_child_terminal', side_effect=runner.Refused('ECHILD')):
            with self.assertRaises(runner.Refused): runner.local_anchor_matches(child, 'retained-before-release')

    def test_positive_owned_zombie_receipt_allows_reap_only_after_quiescence(self):
        child = mock.Mock(pid=24680); reaped = []
        def snapshot(_group):
            return [] if reaped else [dict(pid=child.pid, uid=os.getuid(), status='Z')]
        child.wait.side_effect = lambda timeout: reaped.append(timeout)
        with mock.patch.object(runner, 'local_group_snapshot', side_effect=snapshot), \
                mock.patch.object(runner, 'process_identity', side_effect=runner.Refused('libproc omitted zombie')), \
                mock.patch.object(runner, 'local_child_terminal', return_value=0) as wait, \
                mock.patch.object(runner.os, 'killpg') as signals:
            runner.local_stop_owned(child, 'retained-before-release')
            wait.assert_called_once_with(child)
            signals.assert_not_called(); child.wait.assert_called_once_with(timeout=1)


if __name__ == '__main__': unittest.main()
