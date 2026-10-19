"""No network/process-launch fixtures: --plan or pre-transport refusal only."""
import json
import os
from pathlib import Path
import subprocess
import unittest

SCRIPT = Path(os.environ.get('SIRSI_FABRIC_FIXTURE_SCRIPT',
                             str(Path(__file__).with_name('fabric-run.sh'))))


class TransportPlan(unittest.TestCase):
    def run_plan(self, *options, proxy=None, host='m1', payload=('/usr/bin/true',)):
        environment = dict(os.environ)
        for key in ('ALL_PROXY', 'all_proxy', 'HTTP_PROXY', 'http_proxy', 'HTTPS_PROXY', 'https_proxy'):
            environment.pop(key, None)
        environment['SIRSI_AGENT_ID'] = 'sirsi-software-admin'
        environment['SIRSI_ROUTER_AGENT'] = 'sirsi-software-admin'
        if proxy is not None:
            environment['ALL_PROXY'] = proxy
        return subprocess.run(['/bin/bash', str(SCRIPT), '--plan', *options,
                               host, '--work', 'fixture-only', '--', *payload],
                              env=environment, text=True, capture_output=True,
                              timeout=5, check=False)

    def test_default_still_refuses_absent_proxy(self):
        result = self.run_plan()
        self.assertEqual(result.returncode, 78)
        self.assertNotIn('PLAN_ONLY', result.stdout)

    def test_explicit_direct_fixed_tailnet_destinations(self):
        for host, address in (('m1', '100.88.242.95'),
                              ('m5', '100.92.193.26'),
                              ('studio', '100.87.95.88')):
            with self.subTest(host=host):
                result = self.run_plan('--transport', 'direct', host=host)
                self.assertEqual(result.returncode, 0, result.stderr)
                for literal in (address, 'StrictHostKeyChecking=yes',
                                'BatchMode=yes', 'PasswordAuthentication=no',
                                'KbdInteractiveAuthentication=no',
                                'ProxyCommand=none', 'ProxyJump=none',
                                'sirsi-maat-run', '--register-executor'):
                    self.assertIn(literal, result.stdout)
                self.assertNotIn('/usr/bin/nc', result.stdout)

    def test_direct_cannot_bypass_present_runtime_proxy(self):
        result = self.run_plan('--transport', 'direct', proxy='socks5://127.0.0.1:19001')
        self.assertEqual(result.returncode, 78)
        self.assertIn('must not be bypassed', result.stderr)
        self.assertEqual(result.stdout, '')

    def test_proxy_uses_loopback_and_same_stable_destinations(self):
        result = self.run_plan(proxy='socks5h://127.0.0.1:19001', host='studio')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('100.87.95.88', result.stdout)
        self.assertIn('10.96.127.2', result.stdout)  # retained pinned key alias
        self.assertIn('19001', result.stdout)

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

    def test_ra_existing_worker_has_answering_grant(self):
        registry = json.loads((SCRIPT.parents[2] / '.agents/idea-router/agents.json').read_text())
        ra = registry['agents']['ra']
        self.assertIs(ra.get('worker_answers'), True)
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


if __name__ == '__main__':
    unittest.main()
