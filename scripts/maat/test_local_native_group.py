"""Darwin group ABI/error controls and a bounded own-child no-signal check."""
import ctypes
import errno
import importlib.util
import os
from pathlib import Path
import subprocess
import sys
from types import SimpleNamespace
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('native_group_runner', Path(__file__).with_name('admitted-run.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class NativeGroupTests(unittest.TestCase):
    def list_call(self, groups):
        def call(kind, group, buffer, size):
            self.assertEqual(kind, 2)
            if buffer is None:
                self.assertEqual(size, 0)
                return 128
            pids = groups(group)
            for index, pid in enumerate(pids):
                buffer[index] = pid
            return len(pids) * 4
        return call

    def metadata(self, pid, include_zombies=False):
        if pid == os.getpid():
            self.assertFalse(include_zombies)
            return SimpleNamespace(pid=pid, pgid=os.getpgrp(), uid=os.getuid(), ruid=os.getuid(), status=2)
        self.assertTrue(include_zombies)
        return SimpleNamespace(pid=pid, ppid=os.getpid(), pgid=24680, uid=os.getuid(), status=5)

    def snapshot(self, groups, metadata=None):
        library = mock.Mock()
        library.proc_listpids.side_effect = self.list_call(groups)
        with mock.patch.object(runner.sys, 'platform', 'darwin'), \
                mock.patch.object(runner.ctypes, 'CDLL', return_value=library), \
                mock.patch.object(runner, 'native_process_info', side_effect=metadata or self.metadata), \
                mock.patch.object(runner.subprocess, 'run', side_effect=PermissionError('ps forbidden')) as ps:
            result = runner.local_group_snapshot(24680)
            ps.assert_not_called()
            return result

    def test_positive_group_and_unreaped_zombie_without_ps(self):
        result = self.snapshot(lambda group: [os.getpid()] if group == os.getpgrp() else [24680])
        self.assertEqual(result, [dict(pid=24680, ppid=os.getpid(), pgid=24680, uid=os.getuid(), status='Z')])

    def test_empty_target_requires_positive_same_executor_control(self):
        self.assertEqual(self.snapshot(lambda group: [os.getpid()] if group == os.getpgrp() else []), [])
        with self.assertRaisesRegex(runner.Refused, 'positive control'):
            self.snapshot(lambda group: [])

    def test_denied_zero_return_never_means_absence(self):
        def denied(*_args):
            ctypes.set_errno(errno.EPERM)
            return 0
        for phase in ('size', 'read'):
            def call(*args):
                return 128 if phase == 'read' and args[2] is None else denied()
            with self.subTest(phase=phase), self.assertRaises(runner.Refused):
                runner._native_group_pids(24680, call)

    def test_stale_errno_cleared_but_malformed_or_truncated_reads_refuse(self):
        ctypes.set_errno(errno.EPERM)
        self.assertEqual(runner._native_group_pids(24680, self.list_call(lambda _: [])), [])
        for size, read in ((0, 0), (-1, 0), (3, 0), (1 << 20, 0), (128, -1), (128, 3), (128, 192), (128, 196)):
            def call(_kind, _group, buffer, capacity):
                return size if buffer is None else read
            with self.subTest(size=size, read=read), self.assertRaises(runner.Refused):
                runner._native_group_pids(24680, call)
        for pids in ([0], [-1], [24680, 24680]):
            with self.subTest(pids=pids), self.assertRaises(runner.Refused):
                runner._native_group_pids(24680, self.list_call(lambda _: pids))

    def test_member_metadata_denial_drift_or_unknown_state_does_not_drop_member(self):
        for change in ('denied', 'group', 'state'):
            def metadata(pid, include_zombies=False):
                row = self.metadata(pid, include_zombies)
                if pid == 24680:
                    if change == 'denied':
                        raise runner.Refused('fixture metadata unavailable')
                    if change == 'group':
                        row.pgid += 1
                    else:
                        row.status = 99
                return row
            with self.subTest(change=change), self.assertRaises(runner.Refused):
                self.snapshot(lambda group: [os.getpid()] if group == os.getpgrp() else [24680], metadata)
        target_calls = []
        def drifting(group):
            if group == os.getpgrp():
                return [os.getpid()]
            target_calls.append(group)
            return [24680] if len(target_calls) == 1 else []
        with self.assertRaisesRegex(runner.Refused, 'membership changed'):
            self.snapshot(drifting)

    def test_failed_native_observation_never_falls_back_to_ps(self):
        with mock.patch.object(runner.sys, 'platform', 'darwin'), \
                mock.patch.object(runner, 'native_group_snapshot', side_effect=runner.Refused('denied')), \
                mock.patch.object(runner, 'ps_group_snapshot') as fallback:
            with self.assertRaises(runner.Refused):
                runner.local_group_snapshot(24680)
            fallback.assert_not_called()

    def test_bsd_info_group_read_explicitly_includes_zombies_only_when_requested(self):
        library = mock.Mock()
        def lookup(pid, flavor, arg, pointer, size):
            self.assertEqual((pid, flavor, size), (24680, 3, 136))
            row = ctypes.cast(pointer, ctypes.POINTER(runner._BSDInfo)).contents
            row.pid, row.start_sec = pid, 1
            return size
        library.proc_pidinfo.side_effect = lookup
        with mock.patch.object(runner.sys, 'platform', 'darwin'), mock.patch.object(runner.ctypes, 'CDLL', return_value=library):
            runner.native_process_info(24680)
            runner.native_process_info(24680, include_zombies=True)
        self.assertEqual([call.args[2] for call in library.proc_pidinfo.call_args_list], [0, 1])

    def test_native_denial_cannot_signal_or_reap_owned_handle(self):
        child = mock.Mock(pid=24680, returncode=None)
        with mock.patch.object(runner, 'local_group_snapshot', side_effect=runner.Refused('group observation denied')), \
                mock.patch.object(runner.os, 'killpg') as signals:
            with self.assertRaises(runner.Refused):
                runner.local_stop_owned(child, 'retained-start')
            signals.assert_not_called()
            child.wait.assert_not_called()

    @unittest.skipUnless(sys.platform == 'darwin', 'actual public Darwin observation only')
    def test_actual_owned_child_exit_observed_and_reaped_without_ps_or_signals(self):
        # Executed only inside the caller's normal admitted test invocation.
        # Own finite child exits naturally; no foreign PID, signal or service.
        child = subprocess.Popen([sys.executable, '-c', 'import time;time.sleep(0.3)'], start_new_session=True)
        try:
            _, birth = runner.process_identity(child.pid)
            runner.local_bind_owned_child(child, birth)
            runner.local_wait_terminal(child, 3)
            with mock.patch.object(runner.subprocess, 'run', side_effect=PermissionError('ps forbidden')), \
                    mock.patch.object(runner.os, 'killpg', side_effect=AssertionError('no signal for naturally exited child')):
                runner.local_stop_owned(child, birth)
                self.assertEqual(runner.local_group_snapshot(child.pid), [])
            self.assertEqual(child.returncode, 0)
        finally:
            child.wait(timeout=3)


if __name__ == '__main__':
    unittest.main()
