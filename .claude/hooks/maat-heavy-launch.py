#!/usr/bin/env python3
"""Conservative PreToolUse execution gate; the native runner owns admission."""
import json
import os
import re
import shlex
import sys

READS = {"rg", "grep", "cat", "head", "tail", "sed", "ls", "pwd", "stat", "find", "wc", "sha256sum", "shasum", "diff", "file", "uname", "ps", "pgrep", "id", "date", "cd", "lsof", "uptime", "whoami", "jq", "echo", "printf"}
GIT_READS = {"status", "diff", "show", "log", "rev-parse", "ls-files", "ls-tree", "cat-file", "describe", "add", "commit", "branch", "fetch", "push"}
FRONT = os.path.expanduser("~/.local/bin/sirsi-maat-run")
CLI = os.path.expanduser("~/.local/bin/sirsi")
REASON = ("Ma'at: execution requires the supervised foreground front door. "
          "Register your own actual runtime, then use ~/.local/bin/sirsi-maat-run "
          "--agent <own-id> --thread <own-thread> --work <exact-work> --timeout 300 -- <program> <args>. "
          "Do not change permissions or retry in another session. Registration, router coordination and source reads remain available.")
OPERATORS = {";", "&", "&&", "|", "||", "<", ">", ">>", "<<", "(", ")"}

def unsafe_expansion(command):
    single = double = escaped = False
    for char in command:
        if char == '\x00':
            return True
        if escaped:
            escaped = False
            continue
        if char == '\\' and not single:
            escaped = True
        elif char == "'" and not double:
            single = not single
        elif char == '"' and not single:
            double = not double
        elif not single and char in '$`':
            return True
        elif not single and not double and char in '\n\r':
            return True
    return False

def words(command):
    lexer = shlex.shlex(command, posix=True, punctuation_chars=";&|<>()")
    lexer.whitespace_split = True
    lexer.commenters = ""
    return list(lexer)

def read_command(argv):
    if not argv:
        return False
    if argv[0] == 'export':
        return bool(argv[1:]) and all(selector_assignment(x) or re.fullmatch(r'SIRSI_(AGENT_ID|THREAD_ID|ROUTER_AGENT)',x) for x in argv[1:])
    if argv[0] == 'env':
        argv = argv[1:]
    while argv and selector_assignment(argv[0]):
        argv = argv[1:]
    if not argv:
        return True
    executable = argv[0]
    name = os.path.basename(executable)
    if executable in (CLI, "~/.local/bin/sirsi", "$HOME/.local/bin/sirsi", "${HOME}/.local/bin/sirsi", "sirsi"):
        # Registration/coordination must be available before any expensive job
        # can acquire admission. Never permit a service/worker spawn here.
        family = argv[1:2]
        verb = argv[2:3]
        return (family == ["thread"] and verb in (["register"], ["heartbeat"], ["list"], ["attended"], ["close"])) or \
               (family == ["router"] and verb in (["send"], ["pull"], ["show"], ["respond"], ["status"], ["close"], ["ledger"], ["task"])) or \
               (family == ["maat"] and verb in (["reserve"], ["heartbeat"], ["release"], ["who-is-on"], ["status"], ["conflict-check"], ["should-defer"])) or \
               (family == ["thoth"] and verb in (["status"], ["sync"], ["compact"], ["init"], ["journal"])) or \
               argv[1:] in (["--version"], ["version"])
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
    if name == "gh":
        return len(argv) > 2 and argv[1] in {"pr", "issue", "run", "repo"} and argv[2] in {"view", "list", "diff", "checks"}
    if name == "launchctl":
        return argv[1:2] in (["print"], ["list"])
    if name == "sysctl":
        return not any(x == "-w" or '=' in x for x in argv[1:])
    if name == "memory_pressure":
        return argv[1:] == ["-Q"]
    if name == "codesign":
        return len(argv)>1 and any(x in {"-d", "-dv", "--display", "--verify"} for x in argv[1:]) and not any(x in {"-s", "--sign", "--force", "--remove-signature"} or x.startswith('--sign=') for x in argv[1:])
    if name == "go":
        return argv[1:] in (["version"], ["help"], ["env"])
    if name == "sirsi":
        return argv[1:] in (["maat", "status"], ["--version"], ["version"])
    return False

def selector_assignment(word):
    # Only own-session selectors, not PATH/HOME/PYTHONPATH/permission or router
    # destination overrides. These selectors never bypass ancestry checks.
    return re.fullmatch(r'SIRSI_(?:AGENT_ID|THREAD_ID|ROUTER_AGENT)=[A-Za-z0-9_./${}-]+', word) is not None

def denial(payload, depth=0):
    if depth > 8:
        return REASON
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
    normalized = normalize_control_substitutions(command, depth)
    if normalized is None:
        return REASON
    try:
        argv = words(normalized)
    except ValueError:
        return "Malformed shell command: " + REASON
    # Registered-session execution remains supervised. Only bounded control
    # plane/source operations get an exception, never arbitrary interpreters.
    front = argv and argv[0] in (FRONT, "~/.local/bin/sirsi-maat-run")
    if front:
        if tool.get('run_in_background') or payload.get('run_in_background'):
            return REASON
        return None if not unsafe_expansion(command) and not any(x in OPERATORS or (x and set(x) <= set(";&|<>()")) for x in argv[1:]) else REASON
    segment = []
    for word in argv + [";"]:
        if word in OPERATORS or (word and set(word) <= set(";&|<>()")):
            if word not in {";", "&&", "|"} or not read_command(segment):
                return REASON
            segment = []
        else:
            segment.append(word)
    return None

def normalize_control_substitutions(command, depth):
    # Observe only: never evaluate a shell expansion. Its nested commands must
    # themselves be control-plane/read operations, and the substituted result
    # remains data (not an executable) in the outer allowlisted command.
    result = []
    i = 0
    single = double = escaped = False
    while i < len(command):
        char = command[i]
        if escaped:
            result.append(char); escaped = False; i += 1; continue
        if char == chr(92) and not single:
            result.append(char); escaped = True; i += 1; continue
        if char == "'" and not double:
            single = not single
        elif char == '"' and not single:
            double = not double
        elif not single and char == '`':
            return None
        elif not single and command[i:i+2] == '$(':
            j = i+2
            count = 1
            inner_single = inner_double = inner_escape = False
            while j < len(command) and count:
                c = command[j]
                if inner_escape:
                    inner_escape = False
                elif c == chr(92) and not inner_single:
                    inner_escape = True
                elif c == "'" and not inner_double:
                    inner_single = not inner_single
                elif c == '"' and not inner_single:
                    inner_double = not inner_double
                elif not inner_single and not inner_double:
                    if c == '(':
                        count += 1
                    elif c == ')':
                        count -= 1
                j += 1
            if count:
                return None
            body = command[i+2:j-1]
            if denial({'tool_name':'Bash','tool_input':{'command':body}}, depth+1) is not None:
                return None
            result.append('registered_value')
            i = j
            continue
        result.append(char)
        i += 1
    return ''.join(result)

def command_substitution(command):
    single = escaped = False
    double = False
    for i, char in enumerate(command):
        if escaped:
            escaped = False
        elif char == "\\\\" and not single:
            escaped = True
        elif char == "'" and not double:
            single = not single
        elif char == '"' and not single:
            double = not double
        elif not single and (char == "`" or (char == "$" and command[i+1:i+2] == "(")):
            return True
    return False

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
