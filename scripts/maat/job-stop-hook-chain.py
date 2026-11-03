#!/usr/bin/python3
"""Preserve the existing guard, then apply Ma'at at its already-loaded path."""
from pathlib import Path
import json
import subprocess
import sys

MAX_HOOK_BYTES = 8 * 1024 * 1024


def reject_nonfinite(value):
    raise ValueError('non-finite JSON constant: ' + value)


def check(payload, folder):
    if len(payload) > MAX_HOOK_BYTES:
        print('Ma\'at: hook payload exceeds bound', file=sys.stderr)
        return 2
    try:
        json.loads(payload.decode('utf-8'), parse_constant=reject_nonfinite)
    except (UnicodeDecodeError, ValueError, RecursionError):
        print('Ma\'at: malformed UTF-8 hook input', file=sys.stderr)
        return 2
    legacy = folder/'guard-pretool.original-20261016'
    gate = folder/'maat-heavy-launch.py'
    stop_guard = folder/'maat-job-stop-policy.py'
    if not legacy.is_file() or legacy.is_symlink() or not gate.is_file() or gate.is_symlink():
        print('Ma\'at: complete original+admission guard installation required', file=sys.stderr)
        return 2
    try:
        if not stop_guard.is_file() or stop_guard.is_symlink():
            print('Ma\'at: job stop ownership policy unavailable', file=sys.stderr)
            return 2
        ownership = subprocess.run(['/usr/bin/python3', '-B', str(stop_guard),
                                    '--check-stop-command'], input=payload,
                                   capture_output=True, timeout=5)
        if ownership.returncode:
            sys.stderr.buffer.write(ownership.stderr)
            return 2
        first = subprocess.run([str(legacy)], input=payload, capture_output=True, timeout=5)
        if first.returncode:
            sys.stderr.buffer.write(first.stderr)
            return 2
        second = subprocess.run(['/usr/bin/python3', '-B', str(gate)], input=payload,
                                capture_output=True, timeout=5)
        if second.returncode:
            sys.stderr.buffer.write(second.stderr)
            return 2
        sys.stdout.buffer.write(first.stdout)
        sys.stdout.buffer.write(second.stdout)
        return 0
    except (OSError, subprocess.TimeoutExpired) as error:
        print('Ma\'at: hook chain unavailable: '+str(error), file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(check(sys.stdin.buffer.read(MAX_HOOK_BYTES + 1), Path(__file__).resolve().parent))
