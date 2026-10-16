#!/usr/bin/env python3
"""Conservative PreToolUse execution gate; the native runner owns admission."""
import json
import os
import shlex
import sys

READS = {"rg", "grep", "cat", "head", "tail", "sed", "ls", "pwd", "stat", "find", "wc", "sha256sum", "shasum", "diff", "file", "uname"}
GIT_READS = {"status", "diff", "show", "log", "rev-parse", "ls-files", "ls-tree", "cat-file", "describe"}
FRONT = os.path.expanduser("~/.local/bin/sirsi-maat-run")
REASON = ("Ma'at: execution requires the supervised foreground front door. "
          "Register your own actual runtime, then use ~/.local/bin/sirsi-maat-run "
          "with your own host/agent/work admission. Do not change permissions or retry in another session.")
OPERATORS = {";", "&", "&&", "|", "||", "<", ">", ">>", "<<", "(", ")"}

def words(command):
    lexer = shlex.shlex(command, posix=True, punctuation_chars=";&|<>()")
    lexer.whitespace_split = True
    lexer.commenters = ""
    return list(lexer)

def read_command(argv):
    if not argv:
        return False
    executable = argv[0]
    name = os.path.basename(executable)
    # An arbitrary /tmp/rg is not a trusted read command.
    if "/" in executable and executable not in {"/usr/bin/" + name, "/bin/" + name}:
        return False
    if name == "rg" and any(x == "--pre" or x.startswith("--pre=") or x == "--hostname-bin" or x.startswith("--hostname-bin=") for x in argv[1:]):
        return False
    if name == "git" and any(x in {"--ext-diff", "--textconv"} for x in argv[1:]):
        return False
    if name in READS:
        # sed's e command/find -exec can execute arbitrary code: don't parse a
        # second programming language as a harmless source read.
        return name not in {"sed", "find"}
    if name == "git":
        return len(argv) > 1 and argv[1] in GIT_READS
    if name == "go":
        return argv[1:] in (["version"], ["help"], ["env"])
    if name == "sirsi":
        return argv[1:] in (["maat", "status"], ["--version"], ["version"])
    return False

def denial(payload):
    if not isinstance(payload, dict):
        return "Malformed hook input"
    if payload.get("tool_name") not in ("Bash", "bash", "exec_command"):
        return None
    tool = payload.get("tool_input", {})
    if not isinstance(tool, dict):
        return "Malformed command input"
    command = tool.get("command", tool.get("cmd"))
    if not isinstance(command, str) or not command.strip():
        return "Missing command input"
    if tool.get("run_in_background") or payload.get("run_in_background"):
        return REASON
    # Shell substitution/line folding can hide execution before argv exists.
    if any(x in command for x in ("$", "`", "\n", "\r", "\\\n")):
        return REASON
    try:
        argv = words(command)
    except ValueError:
        return "Malformed shell command: " + REASON
    front = argv and argv[0] in (FRONT, "~/.local/bin/sirsi-maat-run")
    if front:
        return None if not any(x in OPERATORS or (x and set(x) <= set(";&|<>()")) for x in argv[1:]) else REASON
    # Source-read arguments mentioning MLX/Python are data, not executable names.
    # Permit only read-only simple commands/pipelines; never shell/env wrappers.
    segment = []
    for word in argv + [";"]:
        if word in OPERATORS or (word and set(word) <= set(";&|<>()")):
            if word not in {";", "&&", "|"} or not read_command(segment):
                return REASON
            segment = []
        else:
            segment.append(word)
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
