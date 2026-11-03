#!/usr/bin/env python3
"""Refuse any declared entitlements in a previously verified guard executable.

This does not verify the code signature; callers must do that separately.
"""
import plistlib
import subprocess
import sys

if len(sys.argv) != 2:
    raise SystemExit("usage: verify-pretoolguard-entitlements.py <signed-file>")
result = subprocess.run(
    ["/usr/bin/codesign", "-d", "--entitlements", ":-", sys.argv[1]],
    stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30, check=False,
)
if result.returncode or len(result.stdout) > 65536 or len(result.stderr) > 65536:
    raise SystemExit("cannot verify entitlement policy")
if result.stdout:
    try:
        entitlements = plistlib.loads(result.stdout)
    except (ValueError, plistlib.InvalidFileException) as error:
        raise SystemExit("malformed entitlements") from error
    if type(entitlements) is not dict or entitlements:
        raise SystemExit("unexpected entitlements")
