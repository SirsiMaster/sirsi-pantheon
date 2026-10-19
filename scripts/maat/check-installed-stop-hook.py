"""Inert installed-hook controls: supplied command strings are never executed."""
import json
from pathlib import Path
import subprocess

hook = Path.home() / '.claude/hooks/guard-pretool'
cli = str(Path.home() / '.local/bin/sirsi')
cases = [
    ('~/.local/bin/sirsi-maat-run --work stop-own-training -- pkill -f train', 2),
    (cli + ' router task list sirsi-software-admin --json', 0),
    (cli + ' thread register --agent sirsi-software-admin --surface codex --json', 0),
]
for command, expected in cases:
    output = subprocess.run([str(hook)], input=json.dumps({
        'tool_name': 'Bash', 'tool_input': {'command': command}}),
        text=True, capture_output=True, timeout=15)
    print(json.dumps(dict(command=command, expected=expected,
                          actual=output.returncode, stderr=output.stderr)), flush=True)
    if output.returncode != expected:
        raise SystemExit('Installed hook did not preserve the required decision')
