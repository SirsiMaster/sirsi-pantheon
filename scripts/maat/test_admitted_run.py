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

    def test_registered_foreign_thread_anchor_is_refused(self):
        native = runner.Native()
        native.call = lambda *args: [{'thread':dict(thread_id='thread', agent_id='lane', status='active',pid=2,start_time='fixture')}]
        with mock.patch.object(runner.subprocess, 'check_output', side_effect=['1', 'fixture']):
            with self.assertRaises(runner.Refused):
                native.registered('lane', 'thread')


if __name__ == '__main__':
    unittest.main()
