"""Plan-only Bash fixtures: no selector, supervisor, payload or network execution."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(os.environ.get('SIRSI_FABRIC_FIXTURE_SCRIPT',
                             str(Path(__file__).with_name('fabric-run.sh'))))
PROXY_KEYS = ('ALL_PROXY', 'all_proxy', 'HTTP_PROXY', 'http_proxy', 'HTTPS_PROXY', 'https_proxy')


class TransportPlan(unittest.TestCase):
    def setUp(self):
        private = tempfile.TemporaryDirectory(prefix='fabric-run-plan-')
        self.addCleanup(private.cleanup)
        self.fixture_home = private.name

    def run_plan(self, *options, proxy=None, proxies=None, host='m1', payload=('/usr/bin/true',)):
        # No inherited proxy, Bash startup hook, credential or installed HOME.
        environment = {'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'LANG': 'C', 'LC_ALL': 'C',
                       'HOME': self.fixture_home, 'TMPDIR': self.fixture_home,
                       'TMP': self.fixture_home, 'TEMP': self.fixture_home,
                       'SIRSI_AGENT_ID': 'sirsi-software-admin',
                       'SIRSI_ROUTER_AGENT': 'sirsi-software-admin'}
        if proxy is not None:
            environment['ALL_PROXY'] = proxy
        if proxies is not None:
            environment.update(proxies)
        return subprocess.run(['/bin/bash', str(SCRIPT), '--plan', *options,
                               host, '--work', 'fixture-only', '--', *payload],
                              env=environment, text=True, capture_output=True,
                              timeout=5, check=False)

    def test_default_without_proxy_matches_explicit_pinned_direct(self):
        for host in ('m1', 'm5', 'studio'):
            for proxies in ({}, dict.fromkeys(PROXY_KEYS, '')):
                with self.subTest(host=host, empty_keys=bool(proxies)):
                    result = self.run_plan(host=host, proxies=proxies)
                    explicit = self.run_plan('--transport', 'direct', host=host, proxies=proxies)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(explicit.returncode, 0, explicit.stderr)
                    self.assertEqual(result.stdout, explicit.stdout)
                    self.assertEqual(result.stderr, explicit.stderr)
                    self.assertIn('PLAN_ONLY_NO_EXECUTION', result.stdout)

    def test_explicit_direct_uses_shared_selector_and_logical_identity(self):
        for host in ('m1', 'm5', 'studio'):
            with self.subTest(host=host):
                result = self.run_plan('--transport', 'direct', host=host)
                self.assertEqual(result.returncode, 0, result.stderr)
                for literal in ('sirsi-' + host, 'StrictHostKeyChecking=yes',
                                'BatchMode=yes', 'PasswordAuthentication=no',
                                'KbdInteractiveAuthentication=no',
                                'fabric-endpoints.py', 'ProxyJump=none',
                                'sirsi-maat-run', '--register-executor'):
                    self.assertIn(literal, result.stdout)
                self.assertNotIn('/usr/bin/nc', result.stdout)

    def test_direct_cannot_bypass_present_runtime_proxy(self):
        for key in PROXY_KEYS:
            with self.subTest(key=key):
                result = self.run_plan('--transport', 'direct',
                                       proxies={key: 'socks5://127.0.0.1:19001', 'NO_PROXY': '*'})
                self.assertEqual(result.returncode, 78)
                self.assertIn('must not be bypassed', result.stderr)
                self.assertEqual(result.stdout, '')

    def test_default_preserves_upper_lower_and_precedence_socks(self):
        cases = [{key: scheme + '127.0.0.1:19001'}
                 for key in ('ALL_PROXY', 'all_proxy') for scheme in ('socks5://', 'socks5h://')]
        cases.append({'ALL_PROXY': '', 'all_proxy': 'socks5://127.0.0.1:19001'})
        for proxies in cases:
            with self.subTest(keys=tuple(proxies)):
                result = self.run_plan(proxies=proxies)
                explicit = self.run_plan('--transport', 'socks', proxies=proxies)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(explicit.returncode, 0, explicit.stderr)
                self.assertEqual(result.stdout, explicit.stdout)
                self.assertEqual(result.stderr, explicit.stderr)
                self.assertIn('socks', result.stdout)
                self.assertNotIn('19001', result.stdout)

    def test_default_refuses_each_unsupported_proxy_without_direct_fallback(self):
        for key in PROXY_KEYS:
            with self.subTest(key=key):
                result = self.run_plan(proxies={key: 'unsupported-fixture-proxy', 'NO_PROXY': '*'})
                self.assertEqual(result.returncode, 78)
                self.assertIn('supported runtime SOCKS proxy required', result.stderr)
                self.assertEqual(result.stdout, '')

    def test_explicit_socks_still_refuses_absent_or_empty_proxy(self):
        for proxies in ({}, dict.fromkeys(PROXY_KEYS, '')):
            with self.subTest(empty_keys=bool(proxies)):
                result = self.run_plan('--transport', 'socks', proxies=proxies)
                self.assertEqual(result.returncode, 78)
                self.assertIn('supported runtime SOCKS proxy required', result.stderr)
                self.assertEqual(result.stdout, '')

    def test_proxy_is_preserved_by_selector_without_disclosing_proxy(self):
        result = self.run_plan(proxy='socks5h://127.0.0.1:19001', host='studio')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('sirsi-studio', result.stdout)
        self.assertIn('HostKeyAlias=sirsi-studio', result.stdout)
        self.assertIn('fabric-endpoints.py', result.stdout)
        self.assertIn('socks', result.stdout)
        self.assertNotIn('19001', result.stdout)  # inherited by helper, not a logged credential

    def test_m5_uses_enrolled_named_host_key_namespace(self):
        result = self.run_plan('--transport', 'direct', host='m5')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('sirsi-m5', result.stdout)
        self.assertIn('HostKeyAlias=sirsi-m5', result.stdout)
        self.assertIn('UserKnownHostsFile=', result.stdout)
        self.assertIn('.ssh/sirsi_known_hosts', result.stdout)
        self.assertIn('HostKeyAlgorithms=ssh-ed25519', result.stdout)
        self.assertNotIn('HostKeyAlias=100.92.193.26', result.stdout)

    def test_plan_has_no_selected_address_or_network_probe(self):
        result = self.run_plan('--transport', 'direct', host='studio')
        self.assertEqual(result.returncode, 0)
        self.assertIn('selection only inside admitted transport', result.stdout)
        for address in ('10.96.127.2', '100.87.95.88', '192.168.1.208'):
            self.assertNotIn(address, result.stdout)

    def test_nonloopback_proxy_fails_closed(self):
        self.assertEqual(self.run_plan(proxy='socks5://192.0.2.1:9000').returncode, 78)

    def test_closed_transport_and_host_enums(self):
        for options, host in ((('--transport', 'auto'), 'm1'),
                              (('--transport', 'direct'), 'unregistered')):
            with self.subTest(options=options, host=host):
                self.assertEqual(self.run_plan(*options, host=host).returncode, 64)

    def test_payload_is_literal_remote_argv_not_shell_program(self):
        value = "a space'; $(false) ; 'suffix"
        result = self.run_plan('--transport', 'direct', payload=('/usr/bin/printf', '%s', value))
        self.assertEqual(result.returncode, 0, result.stderr)
        remote = next(x for x in result.stdout.splitlines() if x.startswith('REMOTE_COMMAND='))
        self.assertIn("'a space'\\''; $(false) ; '\\''suffix'", remote)

    def test_ra_worker_cannot_override_attended_lead(self):
        # ADR-079: Ra's lane is its interactive session; the worker never answers beside it.
        registry = json.loads((SCRIPT.parents[2] / '.agents/idea-router/agents.json').read_text())
        ra = registry['agents']['ra']
        self.assertIs(ra.get('attended'), True)
        self.assertFalse(ra.get('worker_answers', False))
        self.assertEqual(ra['wake']['mechanism'], 'session-message')
        self.assertTrue(ra['consumer']['command'])

    def test_mercury_standby_cannot_override_attended_lead(self):
        registry = json.loads((SCRIPT.parents[2] / '.agents/idea-router/agents.json').read_text())
        mercury = registry['agents']['mercury']
        self.assertIs(mercury.get('attended'), True)
        self.assertFalse(mercury.get('worker_answers', False))
        self.assertEqual(mercury['consumer']['mode'], 'command')
        self.assertIn('claude', mercury['consumer']['command'])
        self.assertNotIn('--dangerously-skip-permissions', mercury['consumer']['command'])
        self.assertIn('{{agent}}', mercury['consumer']['prompt'])
        self.assertIn('{{thread}}', mercury['consumer']['prompt'])
        self.assertIn('{{router_root}}', mercury['consumer']['prompt'])

    def test_independent_civic_lane_uses_neutral_coordination_root(self):
        base = SCRIPT.parents[2]
        registry = json.loads((base / '.agents/idea-router/agents.json').read_text())
        lane = registry['agents']['claude-kfca']
        self.assertEqual(lane.get('repo'), '/Users/thekryptodragon/KingFarmCAB')
        self.assertEqual(lane['cwd'], '/Users/thekryptodragon/.sirsi/worktrees/kfca-router-coordination')
        self.assertNotEqual(lane['cwd'], lane['repo'])
        policy = (base / 'docs/router-service/worker-workspaces/kfca/CLAUDE.md').read_text()
        for phrase in ('ONLY `claude-kfca` items', 'Do not read `~/Development`',
                       'Do not change KingFarmCAB', 'No product tests',
                       'not override them'):
            self.assertIn(phrase, policy)


if __name__ == '__main__':
    unittest.main()
