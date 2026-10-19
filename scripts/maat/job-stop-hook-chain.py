#!/usr/bin/python3
"""Preserve the existing guard, then apply Ma'at at its already-loaded path."""
from pathlib import Path
import subprocess
import sys


def check(payload, folder):
    if len(payload) > 65536:
        print('Ma\'at: hook payload exceeds bound', file=sys.stderr)
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
    sys.exit(check(sys.stdin.buffer.read(65537), Path(__file__).resolve().parent))
