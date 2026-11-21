"""No network/process-launch fixtures: --plan or pre-transport refusal only."""
import json
import os
import shlex
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(os.environ.get('SIRSI_FABRIC_FIXTURE_SCRIPT',
                             str(Path(__file__).with_name('fabric-run.sh'))))


class TransportPlan(unittest.TestCase):
    def selected_transport(self, output):
        argv = shlex.split(output.splitlines()[-1])
        proxy_command = next(arg.split('=', 1)[1] for arg in argv if arg.startswith('ProxyCommand='))
        helper_argv = shlex.split(proxy_command)
        return helper_argv[helper_argv.index('--transport') + 1]

    def run_plan(self, *options, proxy=None, proxy_env=None, host='m1', payload=('/usr/bin/true',)):
        environment = dict(os.environ)
        for key in ('ALL_PROXY', 'all_proxy', 'HTTP_PROXY', 'http_proxy', 'HTTPS_PROXY', 'https_proxy'):
            environment.pop(key, None)
        environment['SIRSI_AGENT_ID'] = 'sirsi-software-admin'
        environment['SIRSI_ROUTER_AGENT'] = 'sirsi-software-admin'
        if proxy is not None:
            environment['ALL_PROXY'] = proxy
        if proxy_env:
            environment.update(proxy_env)
        return subprocess.run(['/bin/bash', str(SCRIPT), '--plan', *options,
                               host, '--work', 'fixture-only', '--', *payload],
                              env=environment, text=True, capture_output=True,
                              timeout=5, check=False)

    def test_default_uses_direct_when_no_proxy_exists(self):
        result = self.run_plan()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('PLAN_ONLY_NO_EXECUTION', result.stdout)
        self.assertEqual(self.selected_transport(result.stdout), 'direct')

    def test_auto_respects_valid_proxy_and_matching_aliases(self):
        url = 'socks5://127.0.0.1:19001'
        for env in ({'all_proxy': url}, {'ALL_PROXY': url, 'all_proxy': url},
                    {'ALL_PROXY': url, 'HTTPS_PROXY': url}):
            with self.subTest(env=env):
                result = self.run_plan('--transport', 'auto', proxy_env=env)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.selected_transport(result.stdout), 'socks')
                self.assertNotIn('19001', result.stdout)

    def test_ambiguous_or_unsupported_proxy_settings_fail_closed(self):
        for env in ({'HTTPS_PROXY': 'https://proxy.invalid:443'},
                    {'ALL_PROXY': 'socks5://127.0.0.1:19001', 'all_proxy': 'socks5://127.0.0.1:19002'},
                    {'ALL_PROXY': 'socks5://127.0.0.1:19001', 'HTTP_PROXY': 'http://proxy.invalid:80'},
                    {'ALL_PROXY': 'socks5://127.0.0.1:0'},
                    {'ALL_PROXY': 'socks5://127.0.0.1:65536'},
                    {'ALL_PROXY': 'socks5://127.0.0.1:999999999999999999999999'}):
            with self.subTest(env=env):
                result = self.run_plan(proxy_env=env)
                self.assertEqual(result.returncode, 78, result.stderr)
                self.assertNotIn('PLAN_ONLY', result.stdout)
                self.assertNotIn('19001', result.stderr)

    def test_explicit_socks_requires_proxy_and_explicit_direct_rejects_http(self):
        self.assertEqual(self.run_plan('--transport', 'socks').returncode, 78)
        result = self.run_plan('--transport', 'direct', proxy_env={'http_proxy': 'http://proxy.invalid:80'})
        self.assertEqual(result.returncode, 78)
        self.assertIn('must not be bypassed', result.stderr)

    def test_explicit_direct_uses_shared_selector_and_logical_identity(self):
        for host in ('m1', 'm5', 'studio'):
            with self.subTest(host=host):
                result = self.run_plan('--transport', 'direct', host=host)
                self.assertEqual(result.returncode, 0, result.stderr)
                for literal in ('sirsi-' + host, 'StrictHostKeyChecking=yes',
                                'BatchMode=yes', 'PasswordAuthentication=no',
                                'KbdInteractiveAuthentication=no',
                                'fabric-endpoints.py', 'ProxyJump=none',
                                'GlobalKnownHostsFile=/dev/null', 'KnownHostsCommand=none',
                                'VerifyHostKeyDNS=no', 'UpdateHostKeys=no',
                                'sirsi-maat-run', '--register-executor'):
                    self.assertIn(literal, result.stdout)
                self.assertNotIn('/usr/bin/nc', result.stdout)

    def test_direct_cannot_bypass_present_runtime_proxy(self):
        result = self.run_plan('--transport', 'direct', proxy='socks5://127.0.0.1:19001')
        self.assertEqual(result.returncode, 78)
        self.assertIn('must not be bypassed', result.stderr)
        self.assertEqual(result.stdout, '')

    def test_effective_ssh_config_restricts_host_key_sources(self):
        result = self.run_plan('--transport', 'direct')
        self.assertEqual(result.returncode, 0, result.stderr)
        argv = shlex.split(result.stdout.splitlines()[-1])
        ssh = argv.index('/usr/bin/ssh')
        with tempfile.TemporaryDirectory() as root:
            config = Path(root) / 'config'
            config.write_text('Host *\n  KnownHostsCommand /usr/bin/false\n  GlobalKnownHostsFile /tmp/foreign-known-hosts\n')
            effective = subprocess.run([argv[ssh], '-G', '-F', str(config), *argv[ssh+1:]], text=True, capture_output=True, timeout=5)
        self.assertEqual(effective.returncode, 0, effective.stderr)
        for item in ('stricthostkeychecking true', 'globalknownhostsfile /dev/null',
                     'verifyhostkeydns false', 'updatehostkeys false'):
            self.assertIn(item, effective.stdout.lower())
        self.assertFalse(any(line.startswith('knownhostscommand ') and line != 'knownhostscommand none'
                             for line in effective.stdout.lower().splitlines()))

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
        for options, host in ((('--transport', 'bogus'), 'm1'),
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
