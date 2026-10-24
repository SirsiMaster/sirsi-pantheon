"""Classify inert command text only; never spawn the commands under test."""
import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location('heavy_launch', Path(__file__).with_name('heavy-launch.py'))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class ShellWhitespaceTests(unittest.TestCase):
    front = '~/.local/bin/sirsi-maat-run --agent claude-m5-compasspoint --register-executor --work proof --cpu-cores 1 --timeout 90 -- /usr/bin/true'
    registration = '~/.local/bin/sirsi thread register --agent claude-m5-compasspoint --surface claude --repo /Users/thekryptodragon/Development/compasspoint --json'

    def denied(self, command, **fields):
        return gate.denial({'tool_name': 'Bash', 'tool_input': {'command': command, **fields}})

    def test_foreground_frontdoor_outer_whitespace(self):
        for padding in ['', '\n', '\r\n', '\t ', '\n\n']:
            with self.subTest(padding=repr(padding)):
                self.assertIsNone(self.denied(padding + self.front + padding))

    def test_registration_and_source_reads_outer_whitespace(self):
        for command in [self.registration, 'git status --short', 'git log -1', 'cat README.md']:
            for padding in ['', '\n', '\r\n', '\t ', '\n\n']:
                with self.subTest(command=command, padding=repr(padding)):
                    self.assertIsNone(self.denied(padding + command + padding))

    def test_background_frontdoor_stays_refused(self):
        self.assertIsNotNone(self.denied(self.front + '\n', run_in_background=True))

    def test_frontdoor_followed_by_command_stays_refused(self):
        for suffix in ['\npython3 workload.py', '; python3 workload.py', ' && python3 workload.py', ' &', ' | cat', ';']:
            with self.subTest(suffix=suffix):
                self.assertIsNotNone(self.denied(self.front + suffix + '\n'))

    def test_direct_workload_stays_refused(self):
        for command in ['python3 workload.py\n', 'go test ./...\n', self.registration + '\npython3 workload.py']:
            with self.subTest(command=command):
                self.assertIsNotNone(self.denied(command))

    def test_substitution_and_expansion_stay_refused(self):
        for value in ['$(python3 workload.py)', '`python3 workload.py`', '$UNTRUSTED']:
            with self.subTest(value=value):
                self.assertIsNotNone(self.denied(self.front + ' ' + value + '\n'))

    def test_multiline_coordination_remains_classified(self):
        self.assertIsNone(self.denied(self.registration + '\ngit status --short'))

    def test_empty_input_refused(self):
        self.assertIsNotNone(self.denied(' \n\t'))

    def test_exact_compasspoint_null_stderr_source_read(self):
        command = 'cat /Users/thekryptodragon/Development/compasspoint/.thoth/memory.yaml 2>/dev/null | head -150'
        self.assertIsNone(self.denied(command))
        self.assertIsNone(self.denied(command + '\n'))

    def test_null_stderr_does_not_admit_workloads_or_extra_commands(self):
        for command in ['python3 workload.py 2>/dev/null',
                        'cat README.md 2>/dev/null | python3 workload.py',
                        'cat README.md 2>/dev/null; python3 workload.py',
                        self.front + ' 2>/dev/null']:
            with self.subTest(command=command):
                self.assertIsNotNone(self.denied(command))

    def test_other_redirections_remain_refused(self):
        for suffix in ['2>/tmp/output', '>/dev/null', '2>>/dev/null', '2>&1', '2>/dev/null > /tmp/output']:
            with self.subTest(suffix=suffix):
                self.assertIsNotNone(self.denied('cat README.md ' + suffix))

    def test_io_number_adjacency_does_not_change_executable(self):
        for command in ['2 > /dev/null cat README.md',
                        "'2' > /dev/null cat README.md",
                        '"2">/dev/null cat README.md',
                        '""2>/dev/null cat README.md',
                        r'\2>/dev/null cat README.md',
                        'cat README.md 2 > /dev/null',
                        'cat README.md 2>/dev/null\r',
                        'cat README.md 2>/dev/null\r cat README.md']:
            with self.subTest(command=command):
                self.assertIsNotNone(self.denied(command))

    def test_literal_unquoted_null_stderr_preserves_read_argv(self):
        for command in ['cat README.md 2>/dev/null',
                        'cat README.md 2> /dev/null',
                        '2>/dev/null cat README.md']:
            with self.subTest(command=command):
                self.assertIsNone(self.denied(command))
                self.assertEqual(gate.words(gate.read_null_stderr(command)), ['cat', 'README.md'])


if __name__ == '__main__':
    unittest.main()
