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
        for padding in ['', '\n', '\t ', '\n\n']:
            with self.subTest(padding=repr(padding)):
                self.assertIsNone(self.denied(padding + self.front + padding))

    def test_registration_and_source_reads_outer_whitespace(self):
        for command in [self.registration, 'git status --short', 'git log -1', 'cat README.md']:
            for padding in ['', '\n', '\t ', '\n\n']:
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

    def test_worker_bootstrap_diagnostic_names_actual_executor_not_controller(self):
        reason = self.denied('python3 workload.py')
        self.assertIn('--register-executor', reason)
        self.assertIn("job's actual local runtime", reason)
        self.assertIn('standalone canonical CLI', reason)
        self.assertNotIn('--thread <own-thread>', reason)

    def test_actual_compasspoint_envelope_still_refused_atomic_read_allowed(self):
        self.assertIsNotNone(self.denied('which sirsi 2>/dev/null; echo "---"; sirsi router pull claude-m5-compasspoint 2>&1 | head -100'))
        self.assertIsNone(self.denied(str(gate.CLI) + ' router pull claude-m5-compasspoint'))
        self.assertIsNone(self.denied(self.front))

    def test_compasspoint_prompts_scope_owner_task_dispatcher_and_compute_wrapper(self):
        import json
        root = Path(__file__).resolve().parents[2]
        registry = json.loads((root / '.agents/idea-router/agents.json').read_text())
        prompts = {
            agent: registry['agents'][agent]['consumer']['prompt']
            for agent in ['claude-m5-compasspoint', 'codex-compasspoint-contract']
        }
        coordination = (
            'Register the actual session and perform router/task coordination with '
            'standalone installed CLI calls outside the compute wrapper.'
        )
        source_read = (
            'Read Thoth/canon with standalone permitted source-read commands outside '
            'the compute wrapper.'
        )
        dispatcher = (
            "The dispatcher binding is {{thread}}; it identifies the wake controller, "
            "not this child's actual PID or a new runtime to adopt."
        )
        payload = (
            '~/.local/bin/sirsi-maat-run --agent {{agent}} --register-executor '
            '--work <exact-work> --timeout 300 -- <program> <args>'
        )
        for agent, prompt in prompts.items():
            with self.subTest(agent=agent):
                self.assertIn(dispatcher, prompt)
                self.assertIn(coordination, prompt)
                self.assertIn(source_read, prompt)
                self.assertIn('Never pass the dispatcher binding as --thread.', prompt)
                self.assertIn(payload, prompt)
                self.assertNotIn('your worker thread is {{thread}}', prompt)
                self.assertNotIn('--thread {{thread}}', prompt)
                self.assertIn('No account/security changes', prompt)
        codex_prompt = prompts['codex-compasspoint-contract']
        self.assertIn('Local payload work still uses ~/.local/bin/sirsi-maat-run.', codex_prompt)
        for native_fact in (
            '~/.local/bin/sirsi-fabric-run m1|m5|studio --work <own-work> '
            '--cpu-cores <needed> --timeout <seconds> -- <argv>',
            'only through the runtime-provided scoped SOCKS proxy',
            'Existing host pins and public-key authentication remain mandatory',
        ):
            self.assertIn(native_fact, codex_prompt)
        self.assertIn('already-authorized', prompts['claude-m5-compasspoint'])

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

    def test_unquoted_carriage_return_never_rewrites_command_identity(self):
        for command in ['\rcat README.md', 'cat\r README.md',
                        self.front + '\r\n', 'cat README.md\r\n',
                        '\rcat README.md 2>/dev/null']:
            with self.subTest(command=repr(command)):
                self.assertIsNotNone(self.denied(command))
        self.assertIsNone(self.denied("cat 'name\rwith-data'"))


if __name__ == '__main__':
    unittest.main()
