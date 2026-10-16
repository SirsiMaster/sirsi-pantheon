import importlib.util
from pathlib import Path
import unittest
import subprocess
import json
import tempfile
s = importlib.util.spec_from_file_location("guard", Path(__file__).with_name("maat-heavy-launch.py"))
g = importlib.util.module_from_spec(s)
s.loader.exec_module(g)
class GuardTest(unittest.TestCase):
    def test_detached_python_and_mlx(self):
        for command in ["nohup python3 /tmp/probe.py >log 2>&1 &", "setsid /venv/bin/python3.12 probe.py", "env X=1 sh -c 'python3 probe.py &'", "python3 -c 'import mlx.core'", "python3 -m mlx_lm.generate", "python3 probe.py &"]:
            with self.subTest(command=command):
                self.assertIsNotNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
    def test_permission_held_cross_session_has_no_exemption(self):
        for session in ["owner", "subagent-ad389", "fallback-session"]:
            self.assertIsNotNone(g.denial({"tool_name":"Bash","session_id":session,"permission_mode":"bypassPermissions","admitted":True,"tool_input":{"command":"nohup python3 /tmp/probe.py &"}}))
    def test_harmless_commands(self):
        for command in ["git status --porcelain", "git status && git diff", "sirsi maat status"]:
            self.assertIsNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
    def test_hook_exit_blocks_without_executing_payload(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "must-not-exist"
            payload = {"tool_name":"Bash", "tool_input":{"command":"nohup python3 -c 'import mlx.core; open(\"" + str(marker) + "\", \"w\")' &"}}
            result = subprocess.run(["python3", str(Path(__file__).with_name("maat-heavy-launch.py"))], input=json.dumps(payload), text=True, capture_output=True, timeout=3)
            self.assertEqual(result.returncode, 2)
            self.assertFalse(marker.exists())
    def test_settings_overlay_does_not_change_permissions(self):
        settings = json.loads((Path(__file__).resolve().parents[2] / "hooks" / "maat-launch.settings.json").read_text())
        self.assertNotIn("permissions", settings)
        self.assertEqual(settings["hooks"]["PreToolUse"][0]["matcher"], "Bash")
    def test_foreground_execution_requires_frontdoor(self):
        for command in ["python3 probe.py", "go run probe.go", "go test ./...", "./apollo-worker --model x", "/tmp/rg --model x", "env X=1 python3 probe.py", "sh -c 'python3 probe.py'", "bash -c 'rg mlx file'", "sed 'e python3 probe.py' file", "find . -exec python3 probe.py ;", "rg --pre python3 mlx file", "git diff --ext-diff"]:
            with self.subTest(command=command):
                self.assertIsNotNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
    def test_exact_foreground_frontdoor(self):
        for command in ["~/.local/bin/sirsi-maat-run --work proof -- python3 probe.py", g.FRONT + " --work proof -- ./model"]:
            self.assertIsNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
        for command in ["sirsi-maat-run -- python3 probe.py", "/tmp/sirsi-maat-run -- python3 probe.py", "env X=1 ~/.local/bin/sirsi-maat-run -- python3 probe.py", "sh -c '~/.local/bin/sirsi-maat-run -- python3 probe.py'", "~/.local/bin/sirsi-maat-run -- python3 probe.py &", "~/.local/bin/sirsi-maat-run -- python3 probe.py; echo done", "~/.local/bin/sirsi-maat-run -- python3 probe.py >log", "~/.local/bin/sirsi-maat-run $(python3 probe.py)"]:
            with self.subTest(command=command):
                self.assertIsNotNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
    def test_background_flag_cannot_use_frontdoor(self):
        for command in ["rg mlx source.go", "~/.local/bin/sirsi-maat-run -- python3 probe.py"]:
            self.assertIsNotNone(g.denial({"tool_name":"Bash","tool_input":{"command":command,"run_in_background":True}}))
    def test_source_mentions_are_data(self):
        for command in ["rg 'import mlx.core' source.py", "git show HEAD:mlx.py", "git diff -- mlx_lm.py", "cat python-model.py | head -20"]:
            self.assertIsNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
    def test_malformed_refuses(self):
        for payload in [[], {"tool_name":"Bash"}, {"tool_name":"Bash","tool_input":[]}]:
            self.assertIsNotNone(g.denial(payload))
if __name__ == "__main__": unittest.main()
