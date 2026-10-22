#!/usr/bin/env python3
"""One native endpoint policy for admitted fabric SSH and rendered SSH profiles.

Selection checks TCP availability only. SSH authenticates exactly once afterwards;
host-key/authentication rejection never causes a retry of the command elsewhere.
"""
import argparse
import ipaddress
import os
from pathlib import Path
import shlex
import socket
import subprocess
import sys


PEERS = {
    "m5": {"user": "thekryptodragon", "local": "M5.local", "tail": "100.92.193.26"},
    "m1": {"user": "sirsimasterdev", "local": "M1.local", "tail": "100.88.242.95"},
    "studio": {"user": "sirsimasterdev", "local": "M5-Studio.local", "tail": "100.87.95.88"},
}
# Surveyed directed links, not a fabricated direct M1--M5 Thunderbolt link.
THUNDERBOLT = {
    ("m5", "studio"): ("10.96.127.2", "10.96.195.2", "10.96.154.2"),
    ("studio", "m5"): ("10.96.127.1", "10.96.195.1", "10.96.154.1"),
    ("m1", "studio"): ("10.96.178.2",),
    ("studio", "m1"): ("10.96.178.1",),
}
PRIVATE_LAN = tuple(ipaddress.ip_network(n) for n in
                    ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"))
TB_RANGE = ipaddress.ip_network("10.96.0.0/16")


class PolicyError(Exception):
    pass


def origin_host():
    """Actual native host, never an origin PID or a caller-provided lane name."""
    try:
        result = subprocess.run(["/usr/sbin/scutil", "--get", "LocalHostName"],
                                capture_output=True, text=True, timeout=3, check=False)
    except (OSError, subprocess.TimeoutExpired):
        return None
    return {"m5": "m5", "m1": "m1", "m5-studio": "studio"}.get(result.stdout.strip().lower()) if result.returncode == 0 else None


def runtime_proxy(mode, env):
    proxy = env.get("ALL_PROXY") or env.get("all_proxy") or ""
    any_proxy = proxy or any(env.get(k) for k in ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"))
    if mode == "direct":
        if any_proxy:
            raise PolicyError("configured runtime proxy must not be bypassed")
        return None
    if not proxy:
        if mode == "socks" or any_proxy:
            raise PolicyError("supported runtime SOCKS proxy required")
        return None
    if not proxy.startswith(("socks5://", "socks5h://")):
        raise PolicyError("supported runtime SOCKS proxy required")
    address = proxy.split("://", 1)[1]
    parts = address.split(":")
    if len(parts) != 2 or parts[0] != "127.0.0.1" or not parts[1].isascii() or not parts[1].isdigit() or not 1 <= int(parts[1]) <= 65535:
        raise PolicyError("proxy must be loopback with a valid port")
    return address


def resolve_lan(name):
    try:
        result = subprocess.run(["/usr/bin/dscacheutil", "-q", "host", "-a", "name", name],
                                capture_output=True, text=True, timeout=3, check=False)
        return [line.split(":", 1)[1].strip() for line in result.stdout.splitlines()
                if line.startswith("ip_address:")] if result.returncode == 0 else []
    except (OSError, subprocess.TimeoutExpired):
        return []


def is_lan(address):
    try:
        value = ipaddress.ip_address(address)
    except ValueError:
        return False
    return value.version == 4 and value not in TB_RANGE and any(value in net for net in PRIVATE_LAN)


def candidates(origin, target, resolve=resolve_lan):
    """Classify mDNS answers: .local may resolve to TB or link-local, not LAN."""
    rows = [("thunderbolt", a, None) for a in THUNDERBOLT.get((origin, target), ())]
    if origin in ("m1", "m5") and target in ("m1", "m5") and origin != target:
        # Existing authenticated SSH forwarding, not a new network bridge.
        rows.extend(("thunderbolt-relay", far, via)
                    for via in THUNDERBOLT[(origin, "studio")]
                    for far in THUNDERBOLT[("studio", target)])
    yield from rows
    lan = sorted({a for a in resolve(PEERS[target]["local"]) if is_lan(a)},
                 key=lambda a: int(ipaddress.ip_address(a)))
    yield from (("lan", a, None) for a in lan)
    yield "tailscale", PEERS[target]["tail"], None


def nc_args(address, proxy=None, probe=False):
    args = ["/usr/bin/nc"]
    if probe:
        args.extend(["-z", "-w", "2"])
    if proxy:
        args.extend(["-x", proxy, "-X", "5"])
    return args + [address, "22"]


def reachable(address, proxy):
    # Never directly probe around a runtime SOCKS proxy.
    try:
        if proxy:
            return subprocess.run(nc_args(address, proxy, probe=True), stdin=subprocess.DEVNULL,
                                  stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                  timeout=4, check=False).returncode == 0
        with socket.create_connection((address, 22), timeout=2):
            return True
    except (OSError, subprocess.TimeoutExpired):
        return False


def relay_ssh(via, proxy, address, probe=False):
    args = ["/usr/bin/ssh", "-T", "-oBatchMode=yes", "-oStrictHostKeyChecking=yes",
            "-oPasswordAuthentication=no", "-oKbdInteractiveAuthentication=no",
            "-oPreferredAuthentications=publickey", "-oHostKeyAlias=sirsi-studio",
            "-oUserKnownHostsFile=" + str(Path.home() / ".ssh/sirsi_known_hosts"),
            "-oHostKeyAlgorithms=ssh-ed25519", "-oConnectTimeout=8",
            "-oControlMaster=no", "-oControlPath=none", "-oProxyJump=none",
            "-oProxyCommand=" + shlex.join(nc_args(via, proxy))]
    if not probe:
        args.extend(["-oExitOnForwardFailure=yes", "-W", address + ":22"])
    args.append(PEERS["studio"]["user"] + "@" + via)
    if probe:
        args.extend(["/usr/bin/nc", "-z", "-w", "2", address, "22"])
    return args


def relay_reachable(via, address, proxy):
    try:
        result = subprocess.run(relay_ssh(via, proxy, address, probe=True),
                                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, timeout=12, check=False)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise PolicyError("Studio relay authentication/probe unavailable; no fallback") from exc
    if result.returncode not in (0, 1):
        raise PolicyError("Studio relay identity/authentication/probe rejected; no fallback")
    return result.returncode == 0


def select_endpoint(origin, target, proxy, resolve=resolve_lan, probe=reachable, relay_probe=relay_reachable):
    seen = {}
    for kind, address, via in candidates(origin, target, resolve):
        first = via or address
        if first not in seen:
            seen[first] = probe(first, proxy)
        if seen[first] and (not via or relay_probe(via, address, proxy)):
            return kind, address, via
    raise PolicyError("no reachable enrolled endpoint for " + target)


def render_config(helper, python):
    for value in (helper, python):
        if not Path(value).is_absolute() or any(c in value for c in "\r\n%"):
            raise PolicyError("SSH helper/interpreter paths must be absolute without control/expansion characters")
    lines = ["# Generated from the ONE fabric endpoint policy; no copied endpoint table.",
             "# Fresh connections select TB, classified LAN, then Tailscale; keys remain logical."]
    aliases = {"m5": "sirsi-m5 m5 m5-dev", "m1": "sirsi-m1 m1 m1-backup", "studio": "sirsi-studio m5-studio studio"}
    for name, peer in PEERS.items():
        connect = shlex.join([python, helper, "connect", name, "--transport", "auto"])
        lines.extend(["Host " + aliases[name], "  HostName sirsi-" + name,
                      "  User " + peer["user"], "  HostKeyAlias sirsi-" + name,
                      "  UserKnownHostsFile ~/.ssh/sirsi_known_hosts",
                      "  HostKeyAlgorithms ssh-ed25519", "  BatchMode yes",
                      "  StrictHostKeyChecking yes", "  UpdateHostKeys no", "  CheckHostIP no",
                      "  PasswordAuthentication no", "  KbdInteractiveAuthentication no",
                      "  ConnectTimeout 45", "  ProxyCommand " + connect, ""])
    lines.append("Host *")
    return "\n".join(lines) + "\n"


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    connect = commands.add_parser("connect")
    connect.add_argument("target", choices=tuple(PEERS))
    connect.add_argument("--transport", choices=("auto", "socks", "direct"), default="auto")
    render = commands.add_parser("ssh-config")
    render.add_argument("--helper", default=str(Path(__file__).resolve()))
    render.add_argument("--python", default="/usr/bin/python3")
    args = parser.parse_args(argv)
    try:
        if args.command == "ssh-config":
            print(render_config(args.helper, args.python), end="")
            return 0  # rendering never probes or opens a connection
        proxy = runtime_proxy(args.transport, os.environ)
        kind, address, via = select_endpoint(origin_host(), args.target, proxy)
        print("fabric transport: " + args.target + " via " + kind + " " + address +
              (" through Studio " + via if via else ""), file=sys.stderr)
        # One raw tunnel, one SSH handshake, one remote command. No auth retries.
        tunnel = relay_ssh(via, proxy, address) if via else nc_args(address, proxy)
        os.execv(tunnel[0], tunnel)
    except PolicyError as exc:
        print("REFUSED: " + str(exc), file=sys.stderr)
        return 78
    except OSError:
        print("REFUSED: native transport executable unavailable", file=sys.stderr)
        return 78


if __name__ == "__main__":
    sys.exit(main())
