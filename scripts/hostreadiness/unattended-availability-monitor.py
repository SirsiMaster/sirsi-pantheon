#!/usr/bin/env python3
"""Bounded, non-mutating unattended host availability monitor.

Positive TCP/Tailscale evidence wins over stale client metadata.  Lock,
display-sleep, and clamshell state are independent GUI/session signals.
"""
import argparse
import datetime as dt
import json
import os
import socket
import subprocess
import tempfile
import time

SCHEMA = "sirsi.hardware.unattended-availability.v1"
POSITIVE = "reachable"
TIMEOUT = "timeout"
REFUSED = "connection-refused"
UNKNOWN = "unknown"


def tcp_probe(host, port, timeout):
    try:
        with socket.create_connection((host, port), timeout=timeout):
            return {"state": POSITIVE}
    except socket.timeout as exc:
        return {"state": TIMEOUT, "reason": str(exc) or "connect timeout"}
    except ConnectionRefusedError as exc:
        return {"state": REFUSED, "reason": str(exc)}
    except OSError as exc:
        text = str(exc).lower()
        if "timed out" in text or "operation timed out" in text:
            state = TIMEOUT
        elif "refused" in text:
            state = REFUSED
        else:
            state = UNKNOWN
        return {"state": state, "reason": str(exc)}


def classify(attempts):
    """Classify only after every bounded attempt has produced evidence."""
    probes = {name: [row[name] for row in attempts] for name in ("ssh", "screen-sharing")}
    positive = any(row["state"] == POSITIVE for rows in probes.values() for row in rows)
    if positive:
        return "available", "positive transport evidence"
    if not attempts:
        return "observer-uncertain", "no probe attempts executed"
    negative_states = {TIMEOUT, REFUSED}
    if all(item["state"] in negative_states for rows in probes.values() for item in rows):
        return "transport-unavailable", "all bounded SSH/5900 attempts were negative transport evidence"
    return "observer-uncertain", "one or more probes were unknown or observer-limited"


def remote_session(host, user, timeout):
    if not user:
        return {"status": "deferred", "reason": "no SSH user supplied"}
    command = (
        "locked=$(/usr/sbin/ioreg -l -w 0 | /usr/bin/grep -m1 'IOConsoleLocked' || true); "
        "printf '%s\\n' \"$locked\""
    )
    try:
        result = subprocess.run(
            ["ssh", "-o", "BatchMode=yes", "-o", f"ConnectTimeout={max(1, int(timeout))}", f"{user}@{host}", command],
            text=True, capture_output=True, timeout=timeout + 1,
        )
        if result.returncode != 0:
            return {"status": "deferred", "reason": result.stderr.strip() or "SSH observation failed"}
        text = result.stdout.strip()
        return {"status": "observed", "value": text, "gui_state": "locked" if '"IOConsoleLocked" = Yes' in text else "unlocked" if '"IOConsoleLocked" = No' in text else "unknown"}
    except PermissionError as exc:
        return {"status": "observer-denied", "reason": str(exc)}
    except (OSError, subprocess.TimeoutExpired) as exc:
        return {"status": "deferred", "reason": str(exc)}


def observed_state(receipt):
    """What a history row records a change of: admission state and GUI lock state."""
    return (receipt.get("admission", {}).get("state"), receipt.get("gui_session", {}).get("gui_state"))


def changed_since_current(receipt, current):
    """True when there is no readable latest projection or the observed state differs from it."""
    try:
        with open(current, encoding="utf-8") as handle:
            return observed_state(json.load(handle)) != observed_state(receipt)
    except (OSError, ValueError, AttributeError):
        return True


def publish_receipt(receipt, output, current=None, history="all"):
    """Publish the atomic latest projection, plus an immutable run receipt.

    history="changes" writes the run receipt only when the observed state changed
    since the latest projection. A 60s observer otherwise writes ~1,440 identical
    directories a day (96,740 on the M1 by 2026-11-10, SHA 085045/093848).
    """
    if history == "changes" and current and not changed_since_current(receipt, current):
        output = None
    if output:
        publish_run(receipt, output)
    if current:
        publish_current(receipt, current)
    return output


def publish_run(receipt, output):
    os.makedirs(output, mode=0o700, exist_ok=False)
    path = os.path.join(output, "unattended-availability.json")
    payload = json.dumps(receipt, indent=2) + "\n"
    with open(path, "x", encoding="utf-8") as handle:
        handle.write(payload)


def publish_current(receipt, current):
    payload = json.dumps(receipt, indent=2) + "\n"
    current_dir = os.path.dirname(current)
    os.makedirs(current_dir, mode=0o700, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=".current.", suffix=".json", dir=current_dir)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(payload)
        os.chmod(temporary, 0o600)
        os.replace(temporary, current)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", required=True)
    parser.add_argument("--ssh-user")
    parser.add_argument("--attempts", type=int, default=3)
    parser.add_argument("--interval", type=float, default=1.0)
    parser.add_argument("--timeout", type=float, default=3.0)
    parser.add_argument("--output", required=True)
    parser.add_argument("--current")
    parser.add_argument("--history", choices=("all", "changes"), default="all",
                        help="all: a run receipt every time; changes: only when the observed state changed (needs --current)")
    args = parser.parse_args()
    if not 1 <= args.attempts <= 10 or not 0 < args.interval <= 30 or not 0 < args.timeout <= 10:
        parser.error("attempts 1..10, interval 0..30s, timeout 0..10s")

    attempts = []
    for index in range(args.attempts):
        attempts.append({"attempt": index + 1, "captured_at": dt.datetime.now(dt.timezone.utc).isoformat(), "ssh": tcp_probe(args.host, 22, args.timeout), "screen-sharing": tcp_probe(args.host, 5900, args.timeout)})
        if index + 1 < args.attempts:
            time.sleep(args.interval)
    state, reason = classify(attempts)
    receipt = {
        "schema": SCHEMA,
        "captured_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "host": args.host,
        "admission": {"state": state, "available": state == "available", "reason": reason, "attempts": args.attempts},
        "transport": {"attempts": attempts, "positive_evidence_wins": True, "unavailable_requires_all_bounded_attempts_negative": True},
        "gui_session": remote_session(args.host, args.ssh_user, args.timeout),
        "containment": {"mutating": False, "services_restarted": False, "security_changed": False, "tailscale_plane_changed": False, "unlock_or_authentication_attempted": False, "remote_control_preserved": True},
    }
    publish_receipt(receipt, args.output, args.current, args.history)
    print(json.dumps(receipt, indent=2))
    return 0 if state == "available" else 67


if __name__ == "__main__":
    raise SystemExit(main())
