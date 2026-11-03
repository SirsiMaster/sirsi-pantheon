"""Inert hook inputs only; supplied commands are never executed."""
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


chain = load('hook_chain', 'job-stop-hook-chain.py')
heavy = load('heavy_launch_limit', 'heavy-launch.py')
runner = load('hook_runner', 'admitted-run.py')


class HookEnvelopeLimits(unittest.TestCase):
    def run_heavy(self, raw):
        stdin = io.TextIOWrapper(io.BytesIO(raw), encoding='utf-8')
        with mock.patch.object(heavy.sys, 'stdin', stdin), mock.patch.object(heavy.sys, 'stderr', io.StringIO()):
            return heavy.main()

    def test_large_noncommand_reaches_all_stages_unchanged(self):
        raw = json.dumps({'tool_name': 'mcp__github__create_blob',
                          'tool_input': {'content': 'x' * 100000}}).encode()
        self.assertGreater(len(raw), 65536)
        runner.check_stop_hook_bytes(raw)
        self.assertEqual(self.run_heavy(raw), 0)
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            for name in ('guard-pretool.original-20261016', 'maat-heavy-launch.py', 'maat-job-stop-policy.py'):
                (folder / name).touch()
            result = mock.Mock(returncode=0, stdout=b'', stderr=b'')
            with mock.patch.object(chain.subprocess, 'run', return_value=result) as invoke:
                self.assertEqual(chain.check(raw, folder), 0)
            self.assertEqual(invoke.call_count, 3)
            self.assertTrue(all(call.kwargs['input'] == raw for call in invoke.call_args_list))

    def test_over_limit_and_malformed_refuse(self):
        with self.assertRaises(runner.Refused):
            runner.check_stop_hook_bytes(b' ' * (runner.MAX_HOOK_BYTES + 1))
        with self.assertRaises(ValueError):
            runner.check_stop_hook_bytes(b'{')
        self.assertEqual(self.run_heavy(b' ' * (heavy.MAX_HOOK_BYTES + 1)), 2)
        self.assertEqual(self.run_heavy(b'{'), 2)
        with tempfile.TemporaryDirectory() as directory, mock.patch.object(chain.subprocess, 'run') as invoke:
            with mock.patch.object(chain.sys, 'stderr', io.StringIO()):
                self.assertEqual(chain.check(b' ' * (chain.MAX_HOOK_BYTES + 1), Path(directory)), 2)
            invoke.assert_not_called()

    def test_exact_byte_limit_including_multibyte_text(self):
        prefix = b'{"tool_name":"mcp__github__create_blob","tool_input":{"content":"'
        suffix = b'"}}'
        remaining = runner.MAX_HOOK_BYTES - len(prefix) - len(suffix)
        raw = prefix + ('é' * (remaining // 2)).encode() + (b'x' if remaining % 2 else b'') + suffix
        self.assertEqual(len(raw), runner.MAX_HOOK_BYTES)
        runner.check_stop_hook_bytes(raw)
        self.assertEqual(self.run_heavy(raw), 0)
        with self.assertRaises(runner.Refused):
            runner.check_stop_hook_bytes(raw + b' ')
        self.assertEqual(self.run_heavy(raw + b' '), 2)

    def test_invalid_encoding_and_json_never_reach_legacy(self):
        text = '{"tool_name":"Bash","tool_input":{"command":"pkill -f train"}}'
        bad_inputs = (text.encode('utf-16'), text.encode('utf-32'),
                      b'\xef\xbb\xbf' + text.encode(), b'\xff', b'{',
                      b'{"tool_name":"Bash","tool_input":{"command":"pkill -f train","x":NaN}}',
                      b'{"x":Infinity}', b'{"x":-Infinity}')
        with tempfile.TemporaryDirectory() as directory, mock.patch.object(chain.subprocess, 'run') as invoke:
            folder = Path(directory)
            for name in ('guard-pretool.original-20261016', 'maat-heavy-launch.py', 'maat-job-stop-policy.py'):
                (folder / name).touch()
            invoke.return_value = mock.Mock(returncode=0, stdout=b'', stderr=b'')
            self.assertEqual(chain.check(b'{"tool_name":"Read","tool_input":{}}', folder), 0)
            self.assertEqual(invoke.call_count, 3)
            invoke.reset_mock()
            for raw in bad_inputs:
                with self.subTest(raw=raw[:8]), mock.patch.object(chain.sys, 'stderr', io.StringIO()):
                    self.assertEqual(chain.check(raw, folder), 2)
                    with self.assertRaises(ValueError):
                        runner.check_stop_hook_bytes(raw)
                    self.assertEqual(self.run_heavy(raw), 2)
            invoke.assert_not_called()

    def test_large_envelope_does_not_launder_stop_command(self):
        raw = json.dumps({'tool_name': 'Bash', 'tool_input': {
            'command': 'pkill -f train', 'padding': 'x' * 100000}}).encode()
        with self.assertRaises(runner.Refused):
            runner.check_stop_hook_bytes(raw)
        self.assertEqual(self.run_heavy(raw), 2)


if __name__ == '__main__':
    unittest.main()
