"""Exact LFS01/02/03 prefixes; owned files and inert kernel/process seams only."""
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

spec=importlib.util.spec_from_file_location('recovery_runner',Path(__file__).with_name('admitted-run.py'))
runner=importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class RecoveryPrefixTests(unittest.TestCase):
    def fixture(self,home):
        execution='a'*32
        root=Path(home)/'.sirsi/evidence/maat-runs';root.mkdir(parents=True,mode=0o700)
        folder=root/execution;folder.mkdir(mode=0o700)
        lock=root/'.local-cpu.lock';lock.write_text(execution+'\n');lock.chmod(0o600)
        debt=dict(schema=runner.LOCAL_RECEIPT_SCHEMA,execution=execution,uid=os.getuid(),pid=24680,start='exact-start')
        path=root/'.local-cpu.debt.json';path.write_text(json.dumps(debt));path.chmod(0o600)
        return execution,root,folder,lock,path

    def test_sigterm_ignored_sigkill_quiescence_completes_without_debt(self):
        child=mock.Mock(pid=24680);state={'killed':False,'reaped':False};clock=iter(range(200))
        child.returncode=None
        child._sirsi_local_identity=runner._LocalChildIdentity(child,child.pid,os.getpid(),os.getuid(),os.getuid(),child.pid,'exact-start')
        def snapshot(group):
            self.assertEqual(group,child.pid)
            return [] if state['reaped'] else [dict(pid=child.pid,uid=os.getuid(),status='Z' if state['killed'] else 'S')]
        def signal_group(group,sig):
            self.assertEqual(group,child.pid)
            if sig==runner.signal.SIGKILL:state['killed']=True
        def reap(timeout):
            self.assertTrue(state['killed']);state['reaped']=True
        child.wait.side_effect=reap
        with mock.patch.object(runner,'local_group_snapshot',side_effect=snapshot),mock.patch.object(runner,'process_identity',return_value=(os.getpid(),'exact-start')) as identity, \
                mock.patch.object(runner.os,'killpg',side_effect=signal_group) as signal_call,mock.patch.object(runner.time,'sleep'),mock.patch.object(runner.time,'monotonic',side_effect=lambda:next(clock)/10):
            runner.local_stop_owned(child,'exact-start')
        self.assertEqual(signal_call.call_args_list,[mock.call(child.pid,runner.signal.SIGTERM),mock.call(child.pid,runner.signal.SIGKILL)])
        self.assertGreaterEqual(identity.call_count,4)
        child.wait.assert_called_once_with(timeout=1)

    def test_intent_is_durable_before_fence_and_crash_prefix_reconciles(self):
        with tempfile.TemporaryDirectory() as home,mock.patch.dict(os.environ,{'HOME':home},clear=True):
            root=Path(home)/'.sirsi/evidence/maat-runs';root.mkdir(parents=True,mode=0o700)
            folder=root/('b'*32);folder.mkdir(mode=0o700)
            lock=root/'.local-cpu.lock';lock.write_bytes(b'');lock.chmod(0o600);inode=lock.stat().st_ino
            record={};observed=[];real_sync=os.fsync
            def persist():runner.local_transition(folder,record)
            def sync(fd):
                info=os.fstat(fd)
                if info.st_ino==inode and info.st_size:
                    intent=json.loads((folder/'intent.json').read_text())
                    self.assertEqual(intent['execution'],folder.name)
                    self.assertEqual(intent['spawn_protocol'],'pipe-after-durable-running-v1')
                    observed.append(intent)
                real_sync(fd)
            with mock.patch.object(runner.os,'fsync',side_effect=sync),mock.patch.object(runner.subprocess,'Popen',side_effect=runner.Refused('inert launch boundary')):
                self.assertEqual(runner.supervise_local(['/owned/fake'],10,record,folder,persist),78)
            self.assertTrue(observed,'durable fence must observe the complete typed intent first')
            # Exact possible post-fence/pre-spawn crash prefix, using the real intent.
            lock.write_text(folder.name+'\n')
            with mock.patch.object(runner.subprocess,'Popen') as spawn,mock.patch.object(runner.os,'killpg') as signals:
                runner.reconcile_local_debt(folder.name)
                spawn.assert_not_called();signals.assert_not_called()
            self.assertEqual(json.loads((folder/'cleanup-reconciled.json').read_text())['resolution'],'payload_not_released_by_protocol')
            self.assertEqual(lock.read_bytes(),b'')

    def test_same_resolution_resumes_each_marker_archive_and_fence_prefix(self):
        for boundary in ('marker_committed','before_archive','archive_linked','before_debt_unlink','debt_unlinked','before_fence_clear','fence_cleared'):
            with self.subTest(boundary=boundary),tempfile.TemporaryDirectory() as home,mock.patch.dict(os.environ,{'HOME':home},clear=True):
                execution,root,folder,lock,debt=self.fixture(home);original=debt.read_bytes()
                def step(name):
                    if name==boundary:raise OSError('inert interruption after '+boundary)
                with mock.patch.object(runner,'local_group_snapshot',return_value=[]),mock.patch.object(runner.subprocess,'Popen') as spawn,mock.patch.object(runner.os,'killpg') as signals:
                    with self.assertRaises(OSError):runner.reconcile_local_debt(execution,step=step)
                    marker=(folder/'cleanup-reconciled.json').read_bytes()
                    runner.reconcile_local_debt(execution)
                    runner.reconcile_local_debt(execution)  # Exact complete retry remains idempotent.
                    spawn.assert_not_called();signals.assert_not_called()
                self.assertEqual((folder/'cleanup-reconciled.json').read_bytes(),marker)
                self.assertEqual((root/('.local-cpu.resolved-'+execution+'.json')).read_bytes(),original)
                self.assertFalse(debt.exists());self.assertEqual(lock.read_bytes(),b'')

    def test_partial_marker_stage_can_retry_without_overwriting_original_debt(self):
        with tempfile.TemporaryDirectory() as home,mock.patch.dict(os.environ,{'HOME':home},clear=True):
            execution,root,folder,lock,debt=self.fixture(home);original=debt.read_bytes()
            with mock.patch.object(runner,'local_group_snapshot',return_value=[]):
                with self.assertRaises(OSError):runner.reconcile_local_debt(execution,resolution_write=lambda f,b:f.write(b[:4]))
                self.assertFalse((folder/'cleanup-reconciled.json').exists())
                self.assertEqual(debt.read_bytes(),original);self.assertEqual(lock.read_text(),execution+'\n')
                runner.reconcile_local_debt(execution)
            self.assertEqual(lock.read_bytes(),b'')

    def test_conflicting_or_malformed_marker_refuses_and_old_operation_cannot_clear_new_fence(self):
        for value in ('{bad',json.dumps({'schema':runner.LOCAL_RECEIPT_SCHEMA,'execution':'foreign'})):
            with self.subTest(value=value),tempfile.TemporaryDirectory() as home,mock.patch.dict(os.environ,{'HOME':home},clear=True):
                execution,root,folder,lock,debt=self.fixture(home);marker=folder/'cleanup-reconciled.json';marker.write_text(value);marker.chmod(0o600)
                with mock.patch.object(runner,'local_group_snapshot',return_value=[]):
                    with self.assertRaises((ValueError,runner.Refused)):runner.reconcile_local_debt(execution)
                self.assertTrue(debt.exists());self.assertEqual(marker.read_text(),value);self.assertEqual(lock.read_text(),execution+'\n')
                lock.write_text('c'*32+'\n')
                with self.assertRaises(runner.Refused):runner.reconcile_local_debt(execution)
                self.assertEqual(lock.read_text(),'c'*32+'\n')


if __name__=='__main__':unittest.main()
