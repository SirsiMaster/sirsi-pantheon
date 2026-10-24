"""Test-only transport instrumentation: record, then forward unchanged.

All validation semantics and exit status come from the exact retained portfolio
helper. No policy functions or helper inputs are mocked or replaced here.
"""
import json
import os
from pathlib import Path
import subprocess
import sys

mode = sys.argv[1]
raw = sys.stdin.buffer.read()
Path(os.environ["SIRSI_COMPLETION_ENTRY_RECEIPT"]).write_text(
    json.dumps({"Mode": mode, "Captured": raw.decode("utf-8")})
)
# Replace the pathname after capture. Completion must use the stdin object,
# never a second read of this now-contradictory pathname.
proof = Path(sys.argv[sys.argv.index("--proof") + 1])
proof.write_text('{"status":"draft","blockers":["pathname replaced"]}')
helper = Path(__file__).with_name("agent_completion_gate.py")
completed = subprocess.run([sys.executable, str(helper), *sys.argv[1:]], input=raw)
raise SystemExit(completed.returncode)
