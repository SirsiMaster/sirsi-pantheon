#!/usr/bin/env python3
"""PreToolUse guard. No shell/model execution and no caller-asserted exemption."""
import json
import re
import sys

PYTHON = re.compile(r"(?:^|[\s/;|(&])python(?:3(?:\.\d+)?)?(?=[\s;|&]|$)")
MLX = re.compile(r"\b(?:mlx|mlx_lm|torch|transformers)(?:\b|[._])", re.I)
DETACH = re.compile(r"\b(?:nohup|setsid|disown)\b|(?<!&)&(?!&)")

def denial(payload):
    if not isinstance(payload, dict):
        return "Malformed hook input"
    if payload.get("tool_name") not in ("Bash", "bash", "exec_command"):
        return None
    tool = payload.get("tool_input", {})
    if not isinstance(tool, dict):
        return "Malformed command input"
    command = tool.get("command", tool.get("cmd"))
    if not isinstance(command, str):
        return "Missing command input"
    # Inspect wrappers/nested shell text too. A task lease, environment marker,
    # subagent identity, or another session's permission is never an exemption.
    if MLX.search(command) or (PYTHON.search(command.replace(chr(39), " ").replace(chr(34), " ")) and DETACH.search(command)):
        return ("Ma'at: raw MLX/model or detached Python launch refused. "
                "Use the existing foreground admission supervisor with a fresh "
                "matching host/holder/work admission; do not retry in another session.")
    return None

def main():
    try:
        raw = sys.stdin.read(65537)
        reason = "Hook input exceeds 64 KiB" if len(raw) > 65536 else denial(json.loads(raw))
    except (ValueError, TypeError):
        reason = "Malformed hook input"
    if reason:
        print(reason, file=sys.stderr)
        return 2
    return 0

if __name__ == "__main__":
    sys.exit(main())
