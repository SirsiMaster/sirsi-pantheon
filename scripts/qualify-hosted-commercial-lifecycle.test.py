"""Inert whole-driver controls. No real Brew, launchctl, installer or caller."""
import importlib.util
import pathlib
import subprocess
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('lifecycle', pathlib.Path(__file__).with_name('qualify-hosted-commercial-lifecycle.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class FakeOps:
    def __init__(self, fault=None):
        self.fault = fault
        self.calls = []
        self.mutations = []
        self.unjoined = []
        self.current = b'original canonical cask'
        self.saved = None
        self.tap_present = fault not in ('tap-fail', 'tap-partial', 'untap-fail', 'introduced-tap',
                                        'cleanup-probe-unjoined', 'cleanup-uninstall-unjoined')
        self.partial_tap = False
        self.registered = False
        self.paths = set()
        self.restores = 0
        self.joined = True
        self.receipt = None
        self.inventories = 0

    def provenance(self):
        return {'fixture': True}

    def account(self):
        a = {'uid': 501, 'euid': 501, 'name': 'runner', 'home': '/Users/runner',
             'env_home': '/Users/runner', 'platform': 'Darwin arm64', 'version': '15.7',
             'hosted': 'github-hosted', 'actions': 'true', 'runner_os': 'macOS', 'arch': 'ARM64'}
        if self.fault == 'wrong-account':
            a['name'] = 'thekryptodragon'
        if self.fault == 'wrong-euid':
            a['euid'] = 0
        if self.fault == 'wrong-home':
            a['home'] = '/Users/owner'
        if self.fault == 'wrong-os':
            a['version'] = '26.1'
        return a

    def caller_path(self):
        return '/elsewhere/sirsi' if self.fault == 'path-cli' else None

    def exists(self, path):
        if path == m.TAP:
            return self.tap_present or self.partial_tap
        return path in self.paths or (self.fault == 'dangling-cli' and path == m.CLI)

    def inventory(self):
        self.inventories += 1
        if self.fault == 'cleanup-probe-unjoined' and self.inventories == 2:
            self.unjoined.append(45678)
            raise m.Refused('cleanup probe left unknown owned group')
        if self.fault == 'inventory-error':
            raise m.Refused('inventory transport/probe failed')
        return ['sirsi-pantheon'] if self.registered or self.fault == 'existing-cask' else []

    def namespace(self, uid, allow_owned=False):
        if self.fault in ('existing-service', 'existing-system-service', 'existing-plist', 'running-app', 'probe-error'):
            raise m.Refused(self.fault)

    def taps(self):
        return ['sirsimaster/tools'] if self.tap_present else []

    def brew(self, *args):
        if args == ('--prefix',):
            return '/opt/homebrew\n'
        self.mutations.append(args)
        verb = args[0]
        if verb == 'tap':
            if self.fault in ('tap-fail', 'tap-partial'):
                self.partial_tap = self.fault == 'tap-partial'
                raise m.Refused('tap creation failed')
            self.tap_present = True
        elif verb == 'untap':
            if self.fault == 'untap-fail':
                raise m.Refused('untap failed')
            self.tap_present = False
        elif verb in ('install', 'upgrade', 'reinstall'):
            self.registered = True
            self.paths.update([m.APP, m.CLI])
            if self.fault == verb + '-fail':
                raise m.Refused(verb + ' failed')
            if self.fault in ('interrupted-joined', 'interrupted-unjoined') and verb == 'install':
                self.joined = self.fault == 'interrupted-joined'
                if not self.joined:
                    self.unjoined.append(12345)
                raise m.Interrupted('fixture interrupted child')
        elif verb == 'uninstall':
            if not self.joined:
                raise AssertionError('cleanup raced a running operation')
            if self.fault == 'uninstall-fail':
                raise m.Refused('uninstall failed')
            if self.fault == 'cleanup-uninstall-unjoined':
                self.unjoined.append(45678)
                raise m.Refused('uninstall group unknown')
            self.registered = False
            self.paths.clear()
        return ''

    def tap_state(self):
        return {'head': 'retained original HEAD', 'status': '' if self.current == b'original canonical cask' else ' M Casks/sirsi-pantheon.rb'}

    def read_cask(self):
        return self.current

    def backup(self, data):
        if self.fault == 'partial-backup':
            self.saved = data[:3]
            raise m.Refused('partial backup write')
        self.saved = data

    def restore(self, expected):
        self.restores += 1
        if self.fault == 'restore-fail':
            raise m.Refused('restore write failed')
        if m.digest(self.saved) != expected:
            raise m.Refused('invalid backup must not be copied')
        self.current = self.saved

    def fetch_casks(self):
        if self.fault == 'wrong-cask-hash':
            raise m.Refused('immutable cask hash mismatch')
        return {'140': b'exact140', '141': b'exact141'}

    def write_cask(self, data):
        self.current = data

    def verify(self, version, stage):
        if self.fault == 'stale-caller' and stage == 'upgrade141':
            raise m.Refused('installed caller bytes stale; not executed')
        if self.fault == 'rollback-digest' and stage == 'rollback140':
            return 'different rollback digest'
        return m.PINS[version]['cli']

    def recovery(self):
        pass

    def emit(self, _name, receipt):
        self.receipt = receipt


class WholePathTests(unittest.TestCase):
    def test_normal_transitions_original_caller_and_cleanup(self):
        o = FakeOps()
        r = m.Lifecycle(o).execute()
        self.assertEqual(r['status'], 'passed')
        self.assertEqual([x['stage'] for x in r['transitions']], ['install140', 'upgrade141', 'rollback140'])
        self.assertEqual([x[0] for x in o.mutations], ['install', 'upgrade', 'reinstall', 'uninstall'])
        self.assertFalse(o.inventory())
        self.assertEqual(o.current, b'original canonical cask')

    def test_new_tap_is_removed_only_when_introduced_by_this_run(self):
        o = FakeOps('introduced-tap')
        r = m.Lifecycle(o).execute()
        self.assertEqual(r['status'], 'passed')
        self.assertEqual(o.mutations[0][0], 'tap')
        self.assertEqual(o.mutations[-1][0], 'untap')
        self.assertFalse(o.tap_present)

    def test_rejected_identity_state_and_unknown_probes_never_mutate_brew(self):
        for fault in ['wrong-account', 'wrong-euid', 'wrong-home', 'wrong-os', 'path-cli',
                      'dangling-cli', 'existing-cask', 'existing-service', 'existing-system-service',
                      'existing-plist', 'running-app', 'probe-error', 'inventory-error']:
            with self.subTest(fault=fault):
                o = FakeOps(fault)
                r = m.Lifecycle(o).execute()
                self.assertNotEqual(r['status'], 'passed')
                self.assertEqual(o.mutations, [])

    def test_partial_backup_never_restores_truncated_bytes(self):
        o = FakeOps('partial-backup')
        r = m.Lifecycle(o).execute()
        self.assertIsNotNone(r['original_failure'])
        self.assertEqual(o.current, b'original canonical cask')
        self.assertEqual(o.restores, 0)
        self.assertEqual(o.mutations, [])

    def test_tap_failure_and_partial_tap_report_real_debt(self):
        for fault in ['tap-fail', 'tap-partial']:
            o = FakeOps(fault)
            r = m.Lifecycle(o).execute()
            self.assertIsNotNone(r['original_failure'])
            self.assertEqual([x[0] for x in o.mutations], ['tap'])
            self.assertEqual(bool(r['cleanup_debt']), fault == 'tap-partial')

    def test_each_transition_failure_preserves_original_and_cleans_owned_state(self):
        for verb in ['install', 'upgrade', 'reinstall']:
            o = FakeOps(verb + '-fail')
            r = m.Lifecycle(o).execute()
            self.assertIsNotNone(r['original_failure'])
            self.assertEqual(r['cleanup_debt'], [])
            self.assertEqual(o.mutations[-1][0], 'uninstall')
            self.assertEqual(o.current, b'original canonical cask')

    def test_stale_caller_and_rollback_mismatch_are_not_successes(self):
        for fault in ['stale-caller', 'rollback-digest', 'wrong-cask-hash']:
            o = FakeOps(fault)
            r = m.Lifecycle(o).execute()
            self.assertNotEqual(r['status'], 'passed')
            self.assertIsNotNone(r['original_failure'])
            if fault == 'wrong-cask-hash':
                self.assertEqual(o.mutations, [])

    def test_interruption_joins_before_recovery_or_retains_debt_without_racing(self):
        for fault in ['interrupted-joined', 'interrupted-unjoined']:
            o = FakeOps(fault)
            r = m.Lifecycle(o).execute()
            self.assertIsNotNone(r['original_failure'])
            if o.joined:
                self.assertEqual(o.mutations[-1][0], 'uninstall')
                self.assertEqual(r['cleanup_debt'], [])
            else:
                self.assertEqual([x[0] for x in o.mutations], ['install'])
                self.assertTrue(r['cleanup_debt'])

    def test_failed_uninstall_or_restore_is_not_clean(self):
        for fault in ['uninstall-fail', 'restore-fail', 'untap-fail']:
            o = FakeOps(fault)
            r = m.Lifecycle(o).execute()
            self.assertIsNone(r['original_failure'])
            self.assertTrue(r['cleanup_debt'])
            self.assertNotEqual(r['status'], 'passed')

    def test_new_unknown_group_during_cleanup_stops_all_following_recovery(self):
        for fault in ['cleanup-probe-unjoined', 'cleanup-uninstall-unjoined']:
            o = FakeOps(fault)
            r = m.Lifecycle(o).execute()
            self.assertIsNone(r['original_failure'])
            self.assertTrue(r['cleanup_debt'])
            self.assertEqual(o.restores, 0)
            self.assertNotIn('untap', [x[0] for x in o.mutations])
            if fault == 'cleanup-probe-unjoined':
                self.assertNotIn('uninstall', [x[0] for x in o.mutations])
            self.assertNotEqual(r['status'], 'passed')


class RealBoundaryMethodsTests(unittest.TestCase):
    def test_user_launchdaemon_path_refuses_before_any_probe(self):
        o = object.__new__(m.Ops)
        exact = '/Users/runner/Library/LaunchDaemons/ai.sirsi.pantheon.plist'
        o.exists = lambda p: p == exact
        o.run = mock.Mock()
        with mock.patch.object(m.os.path, 'expanduser', side_effect=lambda p: p.replace('~', '/Users/runner', 1)):
            with self.assertRaises(m.Refused):
                o.namespace(501)
        o.run.assert_not_called()

    def test_launch_probe_error_is_never_not_found(self):
        o = object.__new__(m.Ops)
        o.exists = lambda _p: False
        o.run = mock.Mock(return_value=(113, b'', b'permission denied'))
        with self.assertRaises(m.Refused):
            o.namespace(501)

    def test_exact_missing_services_and_missing_gui_domain_are_accepted(self):
        o = object.__new__(m.Ops)
        o.exists = lambda _p: False
        missing = (113, b'', b'Could not find service "ai.sirsi.pantheon" in domain')
        o.run = mock.Mock(side_effect=[missing, missing,
                           (125, b'', b'Could not find domain for gui/501'), (0, b'', b'')])
        o.namespace(501)

    def test_stale_bytes_rejected_before_trust_or_actual_caller_execution(self):
        o = object.__new__(m.Ops)
        o.env = {'PATH': '/opt/homebrew/bin'}
        o.run = mock.Mock()
        with mock.patch.object(m.pathlib.Path, 'is_symlink', return_value=True), \
             mock.patch.object(m.os.path, 'realpath', return_value=m.APP + '/Contents/MacOS/sirsi'), \
             mock.patch.object(m.shutil, 'which', return_value=m.CLI), \
             mock.patch.object(m.pathlib.Path, 'read_bytes', return_value=b'old installed caller'):
            with self.assertRaises(m.Refused):
                o.verify('141', 'stale-caller')
        o.run.assert_not_called()

    def test_stale_commit_after_actual_entrypoint_is_not_accepted(self):
        o = object.__new__(m.Ops)
        o.env = {'PATH': '/opt/homebrew/bin'}
        o.run = mock.Mock(side_effect=[(0, b'', b''), (0, b'', b''),
            (0, b'0.24.141\n', b''),
            (0, b'{"version":"v0.24.141","dirty":false,"commit":"old"}', b'')])
        with mock.patch.object(m.pathlib.Path, 'is_symlink', return_value=True), \
             mock.patch.object(m.os.path, 'realpath', return_value=m.APP + '/Contents/MacOS/sirsi'), \
             mock.patch.object(m.shutil, 'which', return_value=m.CLI), \
             mock.patch.object(m.pathlib.Path, 'read_bytes', return_value=b'fixture'), \
             mock.patch.object(m, 'digest', return_value=m.PINS['141']['cli']):
            with self.assertRaises(m.Refused):
                o.verify('141', 'stale-source')
        self.assertEqual(o.run.call_args_list[-1].args[0], [m.CLI, 'version', '--json'])

    def test_owned_timeout_joins_before_returning_to_recovery(self):
        with tempfile.TemporaryDirectory(prefix='lifecycle-inert-child-') as directory:
            o = object.__new__(m.Ops)
            o.root = pathlib.Path(directory)
            o.deadline = 20
            o.serial, o.env, o.calls, o.unjoined = 0, {}, [], []
            o.cancel_requested, o.recovering, o.spawn_pending, o.active_child = False, False, False, None
            clock = [0]
            child = mock.Mock(pid=98765)
            child.poll.return_value = None
            def wait(**_kwargs):
                if clock[0] == 0:
                    clock[0] = 21
                    raise subprocess.TimeoutExpired('inert child', 1)
                return 0
            child.wait.side_effect = wait
            with mock.patch.object(m.subprocess, 'Popen', return_value=child), \
                 mock.patch.object(m.time, 'monotonic', side_effect=lambda: clock[0]), \
                 mock.patch.object(m.os, 'getpgid', return_value=98765), \
                 mock.patch.object(m.os, 'killpg', side_effect=[None, ProcessLookupError()]) as kill:
                with self.assertRaises(subprocess.TimeoutExpired):
                    o.run(['/fixture/not-an-executable'])
            self.assertEqual(kill.call_args_list[0].args, (98765, m.signal.SIGTERM))
            self.assertEqual(child.wait.call_count, 2)
            self.assertEqual(o.unjoined, [])
            self.assertIsNone(o.active_child)

    def test_cancel_during_mocked_spawn_is_flag_only_then_owned_join(self):
        with tempfile.TemporaryDirectory(prefix='lifecycle-inert-spawn-') as directory:
            o = object.__new__(m.Ops)
            o.root = pathlib.Path(directory)
            o.deadline = m.time.monotonic() + 20
            o.serial, o.env, o.calls, o.unjoined = 0, {}, [], []
            o.cancel_requested, o.recovering, o.spawn_pending, o.active_child = False, False, False, None
            child = mock.Mock(pid=98765)
            child.poll.return_value = None
            child.wait.return_value = 0
            def spawned(*_args, **_kwargs):
                # Emulates the flag-only signal arriving during Popen return.
                o.cancel_requested = True
                return child
            with mock.patch.object(m.subprocess, 'Popen', side_effect=spawned), \
                 mock.patch.object(m.os, 'getpgid', return_value=98765), \
                 mock.patch.object(m.os, 'killpg', side_effect=[None, ProcessLookupError()]) as kill:
                with self.assertRaises(m.Interrupted):
                    o.run(['/fixture/not-an-executable'])
            self.assertEqual(kill.call_args_list[0].args, (98765, m.signal.SIGTERM))
            self.assertEqual(child.wait.call_count, 1)  # join, not a normal wait
            self.assertEqual(o.unjoined, [])
            self.assertIsNone(o.active_child)
            self.assertFalse(o.spawn_pending)


if __name__ == '__main__':
    unittest.main()
