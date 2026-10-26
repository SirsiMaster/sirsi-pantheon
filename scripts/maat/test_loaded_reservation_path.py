"""An explicit loaded request must ask the scheduler, not borrow a foreign row."""
import unittest
from unittest import mock

import test_admitted_run as support

runner = support.runner


class LoadedBackend(support.Backend):
    cpu_cores = None
    requested_regime = 'loaded'

    def __init__(self, existing, reply):
        super().__init__(existing)
        self.reply = reply

    def reserve(self, resource, agent, work):
        self.reserve_calls += 1
        if self.reply.get('granted') is True:
            self.row = self.reply['reservation']
        return self.reply


class LoadedReservationPathTests(unittest.TestCase):
    def run_request(self, backend):
        clock = support.Clock()
        record = {}
        with mock.patch.object(runner.time, 'monotonic', clock.monotonic), \
                mock.patch.object(runner.subprocess, 'Popen', return_value=support.FakeChild(clock, 0)) as spawn, \
                mock.patch.object(runner, 'stop'):
            code = runner.supervise(['fixture'], 'm5', 'lane', 'thread', 'work', 30, backend, record)
        return code, record, spawn

    def test_foreign_cpu_row_does_not_replace_an_atomic_loaded_request(self):
        foreign = support.admission(id='foreign-cpu', holder='other', work='relay',
                                    regime='build', cores=1)
        own = support.admission(id='own-loaded', regime='loaded')
        backend = LoadedBackend(foreign, {'granted': True, 'reservation': own})
        code, record, spawn = self.run_request(backend)
        self.assertEqual(backend.reserve_calls, 1)
        self.assertEqual(code, 0)
        self.assertEqual(record['admission'], 'own-loaded')
        self.assertFalse(record['borrowed'])
        self.assertEqual(backend.released, ['own-loaded'])
        self.assertEqual(foreign['holder'], 'other')
        self.assertEqual(foreign['status'], 'active')
        spawn.assert_called_once()

    def test_scheduler_denial_is_not_bypassed_or_released(self):
        foreign = support.admission(id='foreign-model', holder='other', regime='loaded')
        backend = LoadedBackend(foreign, {'granted': False, 'conflict': foreign})
        code, record, spawn = self.run_request(backend)
        self.assertEqual(backend.reserve_calls, 1)
        self.assertEqual(code, 78)
        self.assertEqual(record['admission_decision']['conflict']['id'], 'foreign-model')
        self.assertEqual(backend.released, [])
        spawn.assert_not_called()

    def test_insufficient_or_foreign_reply_never_spawns(self):
        for updates, released in [({'share': 'floor', 'cores': 4}, ['own-loaded']),
                                  ({'holder': 'other'}, [])]:
            with self.subTest(updates=updates):
                reply = support.admission(id='own-loaded', regime='loaded', **updates)
                backend = LoadedBackend(support.admission(holder='other', regime='build'),
                                        {'granted': True, 'reservation': reply})
                code, _, spawn = self.run_request(backend)
                self.assertEqual(backend.reserve_calls, 1)
                self.assertEqual(code, 78)
                self.assertEqual(backend.released, released)
                spawn.assert_not_called()

    def test_valid_own_loaded_row_is_not_replaced(self):
        own = support.admission(id='existing-own', regime='loaded')
        backend = LoadedBackend(own, {'granted': False})
        code, record, spawn = self.run_request(backend)
        self.assertEqual(code, 0)
        self.assertEqual(backend.reserve_calls, 0)
        self.assertTrue(record['borrowed'])
        self.assertEqual(backend.released, [])
        spawn.assert_called_once()
