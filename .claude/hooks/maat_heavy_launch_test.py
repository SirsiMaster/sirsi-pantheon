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
        for command in ["~/.local/bin/sirsi-maat-run -- python3 probe.py"]:
            self.assertIsNotNone(g.denial({"tool_name":"Bash","tool_input":{"command":command,"run_in_background":True}}))
        self.assertIsNone(g.denial({"tool_name":"Bash","tool_input":{"command":"sirsi thread heartbeat --thread own","run_in_background":True}}))
    def test_source_mentions_are_data(self):
        for command in ["rg 'import mlx.core' source.py", "git show HEAD:mlx.py", "git diff -- mlx_lm.py", "cat python-model.py | head -20"]:
            self.assertIsNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
    def test_malformed_refuses(self):
        for payload in [[], {"tool_name":"Bash"}, {"tool_name":"Bash","tool_input":[]}]:
            self.assertIsNotNone(g.denial(payload))
    def test_registration_coordination_without_workload_spawn(self):
        for command in [g.CLI+" thread register --agent lane --anchor-pid 123", "sirsi thread heartbeat --thread own", "sirsi router send --to ra --instructions hello", "sirsi maat reserve m5 --work own --json"]:
            self.assertIsNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
        for command in ["sirsi router worker lane", "sirsi router wake-install lane", "/tmp/sirsi thread register --agent lane", "sirsi gemma serve", "sirsi maat heal"]:
            self.assertIsNotNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
    def test_literal_child_script_data_is_not_parent_shell_execution(self):
        for command in [g.FRONT+" --work proof -- /bin/sh -c 'printf %s \"$VALUE\"; true'", "rg '$VALUE' source.go"]:
            self.assertIsNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
        for command in [g.FRONT+' --work proof -- /bin/echo "$(python3 probe.py)"', g.FRONT+' --work proof -- /bin/echo `python3 probe.py`', g.FRONT+' --work proof -- /usr/bin/true\npython3 probe.py']:
            self.assertIsNotNone(g.denial({"tool_name":"Bash","tool_input":{"command":command}}))
    def test_lightweight_coordination_does_not_need_a_gpu_window(self):
        for command in ['cd /repo && git status', 'gh pr view 12 --json title', 'launchctl print gui/501/service', 'sysctl -n hw.memsize', 'memory_pressure -Q', 'codesign --verify /path/binary']:
            self.assertIsNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}))
        for command in ['gh workflow run build', 'launchctl kickstart gui/501/service', 'sysctl -w kern.limit=1', 'memory_pressure -l critical', 'codesign --verify --sign identity /path/binary']:
            self.assertIsNotNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}))
    def test_actual_registration_instruction_mask(self):
        for command in ['export SIRSI_AGENT_ID=own; sirsi thread register --agent own --anchor-pid 123', 'env SIRSI_AGENT_ID=own SIRSI_THREAD_ID=thr-own sirsi thread heartbeat --thread thr-own', '"$HOME/.local/bin/sirsi" thread register --agent own --anchor-pid 123', 'cd "$HOME/Development/Apollo" && sirsi thoth status', 'sirsi thread list --json | jq .', 'export SIRSI_THREAD_ID=$OWN_THREAD; sirsi router pull own']:
            self.assertIsNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}),command)
        for command in ['env PATH=/tmp sirsi thread register --agent own', 'export HOME=/tmp; sirsi thread register --agent own', 'env SIRSI_AGENT_ID=own python3 /tmp/probe.py', 'sirsi thread register --agent $(python3 /tmp/probe.py)']:
            self.assertIsNotNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}),command)
    def test_registration_can_capture_its_real_thread_id(self):
        for command in ['SIRSI_THREAD_ID=$(sirsi thread register --agent own --anchor-pid 123 --json | jq -r .thread_id); export SIRSI_THREAD_ID; sirsi thread heartbeat --thread "$SIRSI_THREAD_ID"', '"$HOME/.local/bin/sirsi" thread register --agent own --anchor-pid $(ps -o ppid= -p $$)']:
            self.assertIsNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}),command)
        for command in ['SIRSI_THREAD_ID=$(python3 probe.py); export SIRSI_THREAD_ID', '$(printf python3) probe.py', 'sirsi thread register --agent $(go run probe.go)', 'sirsi thread register --agent `python3 probe.py`']:
            self.assertIsNotNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}),command)
    def test_acknowledge_is_coordination_not_a_workload_exemption(self):
        for command in ['sirsi router acknowledge own-item --agent own', g.CLI+' router acknowledge own-item --agent own', 'env SIRSI_AGENT_ID=own SIRSI_THREAD_ID=thr-own sirsi router acknowledge own-item --agent own']:
            self.assertIsNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}),command)
        for command in ['sirsi router acknowledge own-item; python3 probe.py', 'sirsi router acknowledge $(python3 probe.py)', 'env PATH=/tmp sirsi router acknowledge own-item', '/tmp/sirsi router acknowledge own-item', 'sirsi router worker own', 'sirsi router token mint host']:
            self.assertIsNotNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}),command)
    def test_multiline_registration_mask_preserves_command_boundaries(self):
        command='export SIRSI_AGENT_ID=own\nSIRSI_THREAD_ID=$(sirsi thread register --agent own --anchor-pid $(ps -o ppid= -p $$) --json | jq -r .thread_id)\nexport SIRSI_THREAD_ID\nsirsi thread heartbeat --thread "$SIRSI_THREAD_ID"'
        self.assertIsNone(g.denial({'tool_name':'Bash','tool_input':{'command':command}}))
        self.assertIsNotNone(g.denial({'tool_name':'Bash','tool_input':{'command':command+'\npython3 probe.py'}}))
if __name__ == "__main__": unittest.main()
