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
        for command in ["git status --porcelain", "python3 -c 'print(1)'", "git status && git diff", "sirsi maat status"]:
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
    def test_malformed_refuses(self):
        for payload in [[], {"tool_name":"Bash"}, {"tool_name":"Bash","tool_input":[]}]:
            self.assertIsNotNone(g.denial(payload))
if __name__ == "__main__": unittest.main()
