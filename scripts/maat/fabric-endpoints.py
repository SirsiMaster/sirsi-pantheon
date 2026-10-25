#!/usr/bin/env python3
"""One native endpoint policy for admitted fabric SSH and rendered SSH profiles.

Selection checks TCP availability only. SSH authenticates exactly once afterwards;
host-key/authentication rejection never causes a retry of the command elsewhere.
"""
import argparse
import ipaddress
import json
import os
from types import SimpleNamespace
import re
import selectors
import time
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


class MetadataCleanupError(Exception):
    """Owned metadata child termination/reaping is uncertain: no next route."""
    pass


# These facts select locality only. The existing native supervisor remains the
# sole scoped registration/admission authority; no enrollment/credential is read.
_UUID = re.compile(r'[0-9A-Fa-f]{8}(?:-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}')
_HEADER = re.compile(r'([A-Za-z][A-Za-z0-9_.-]{0,31}): flags=([0-9A-Fa-f]{1,8})<([A-Z0-9_,]*)> mtu ([0-9]{1,6})')
_INET = re.compile(r'inet ([0-9.]+)(?: --> ([0-9.]+))? netmask (0x[0-9a-fA-F]{1,8}|[0-9.]+)(?: broadcast ([0-9.]+))?')
_FLAGS = frozenset(('UP','BROADCAST','DEBUG','LOOPBACK','POINTOPOINT','NOTRAILERS','RUNNING','NOARP','PROMISC','ALLMULTI','OACTIVE','SIMPLEX','LINK0','LINK1','LINK2','MULTICAST','SMART'))


def metadata_output(argv, limit):
    """Bounded fixed native metadata child; never DNS, a socket or a shell."""
    process = None
    try:
        process = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                   stderr=subprocess.DEVNULL, env={'PATH':'/usr/bin:/bin:/usr/sbin:/sbin','LC_ALL':'C','LANG':'C'})
        deadline = time.monotonic() + 3
        data = bytearray()
        with selectors.DefaultSelector() as ready:
            ready.register(process.stdout, selectors.EVENT_READ)
            while ready.get_map():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    return None
                for key, _ in ready.select(remaining):
                    block = os.read(key.fileobj.fileno(), 8192)
                    if not block:
                        ready.unregister(key.fileobj)
                    else:
                        if len(data) + len(block) > limit:
                            return None
                        data.extend(block)
        remaining = deadline - time.monotonic()
        if remaining <= 0 or process.wait(timeout=remaining) != 0:
            return None
        return data.decode('ascii')
    except (OSError, UnicodeError, subprocess.TimeoutExpired):
        return None
    finally:
        if process is not None:
            if process.poll() is None:
                try:
                    process.kill()
                except ProcessLookupError:
                    pass
            try:
                process.wait(timeout=1)
            except subprocess.TimeoutExpired as exc:
                raise MetadataCleanupError('native metadata child cleanup unconfirmed') from exc
            finally:
                if process.stdout is not None:
                    process.stdout.close()


def parse_interfaces(text):
    """Closed interface/IPv4 proof grammar. Other indented properties carry
    no address evidence; malformed IPv4, flags or duplicate rows refuse locality.
    """
    if (not isinstance(text, str) or len(text) > 262144 or len(text.splitlines()) > 4096
            or any(ord(c) > 126 or (ord(c) < 32 and c not in '\n\t') for c in text)):
        raise PolicyError('native interface inventory unavailable')
    interfaces, addresses = set(), {}
    current = None
    for line in text.splitlines():
        if not line.strip():
            continue
        if len(line) > 2048:
            raise PolicyError('native interface inventory unavailable')
        if not line[0].isspace():
            match = _HEADER.fullmatch(line)
            if not match or match[1] in interfaces:
                raise PolicyError('native interface inventory unavailable')
            name, bits = match[1], int(match[2],16)
            labels = match[3].split(',') if match[3] else []
            if (len(labels) != len(set(labels)) or not set(labels) <= _FLAGS
                    or bool(bits & 1) != ('UP' in labels) or bool(bits & 8) != ('LOOPBACK' in labels)):
                raise PolicyError('native interface inventory unavailable')
            interfaces.add(name)
            current = (name, bool(bits & 1), bool(bits & 8) or name.startswith('lo'))
            continue
        if current is None:
            raise PolicyError('native interface inventory unavailable')
        detail = line.strip()
        if _HEADER.fullmatch(detail):
            raise PolicyError('native interface inventory unavailable')
        if detail.split()[0] != 'inet':
            # Never search arbitrary property text, IPv6, routes or names for an
            # IPv4 match. A misspelled IPv4 field is not silently accepted.
            if detail.startswith('inet') and not detail.startswith('inet6 '):
                raise PolicyError('native interface inventory unavailable')
            continue
        match = _INET.fullmatch(detail)
        if not match:
            raise PolicyError('native interface inventory unavailable')
        try:
            address = str(ipaddress.IPv4Address(match[1]))
            for value in (match[2], match[4]):
                if value is not None:
                    ipaddress.IPv4Address(value)
            mask = int(match[3],16) if match[3].startswith('0x') else int(ipaddress.IPv4Address(match[3]))
            inverted = (~mask) & 0xffffffff
            if inverted & (inverted + 1) or address in addresses:
                raise ValueError('duplicate or malformed address')
        except ValueError as exc:
            raise PolicyError('native interface inventory unavailable') from exc
        addresses[address] = current
    if not interfaces:
        raise PolicyError('native interface inventory unavailable')
    return addresses


def parse_native_uuid(text):
    if not isinstance(text, str) or len(text) > 65536:
        return None
    values = re.findall(r'"IOPlatformUUID"\s*=\s*"([^"\r\n]*)"', text)
    if len(values) != 1 or not _UUID.fullmatch(values[0]):
        return None
    return values[0].upper()


def locality(target, interfaces, native_uuid, uid, euid, account):
    """Pure metadata classification. No ambient identity is authorization."""
    endpoint = PEERS[target]['tail']
    owner = interfaces.get(endpoint) if isinstance(interfaces, dict) else None
    if owner is None:
        return 'ssh', 'endpoint-not-local', None
    if (not isinstance(owner, tuple) or len(owner) != 3
            or not isinstance(owner[0], str) or not re.fullmatch(r'[A-Za-z][A-Za-z0-9_.-]{0,31}', owner[0])
            or type(owner[1]) is not bool or type(owner[2]) is not bool):
        return 'ssh', 'interface-inventory-unavailable', None
    if not owner[1] or owner[2]:
        return 'ssh', 'endpoint-not-active-nonloopback', None
    if not isinstance(native_uuid, str) or not _UUID.fullmatch(native_uuid):
        return 'ssh', 'native-uuid-unavailable', None
    if (type(uid) is not int or type(euid) is not int or uid != euid
            or account is None or account.pw_uid != uid or account.pw_name != PEERS[target]['user']):
        return 'ssh', 'account-not-local-owner', None
    return 'local', 'kernel-endpoint-and-own-uid', owner[0]


_ACCOUNT_QUERY = ('import json,pwd,sys; row=pwd.getpwnam(sys.argv[1]); '
                  'print(json.dumps({"name":row.pw_name,"uid":row.pw_uid}))')


def parse_account(text, expected_name):
    """Only the configured native account's closed name/UID result is accepted."""
    def unique_pairs(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError('duplicate account field')
            result[key] = value
        return result
    if not isinstance(text, str) or len(text) > 1024:
        return None
    try:
        row = json.loads(text, object_pairs_hook=unique_pairs)
    except (ValueError, TypeError):
        return None
    if (not isinstance(row, dict) or set(row) != {'name','uid'}
            or row['name'] != expected_name or type(row['uid']) is not int
            or not 0 <= row['uid'] <= 0xffffffff):
        return None
    return SimpleNamespace(pw_name=row['name'], pw_uid=row['uid'])


def native_account(name):
    # Exact configured name as argv data; isolated stdlib child shares the same
    # three-second/output/kill/wait boundary as other native metadata. No lookup
    # runs synchronously in the selector process; errors remain safe unknown.
    return parse_account(metadata_output(['/usr/bin/python3','-I','-c',_ACCOUNT_QUERY,name],1024),name)


def dispatch(target):
    # No caller address, UUID, hostname, output pathname or grant parameter.
    uid, euid = os.getuid(), os.geteuid()
    native_uuid = parse_native_uuid(metadata_output(['/usr/sbin/ioreg','-rd1','-c','IOPlatformExpertDevice'],65536))
    try:
        interfaces = parse_interfaces(metadata_output(['/sbin/ifconfig','-a'],262144))
        account = native_account(PEERS[target]['user'])
        decision, reason, interface = locality(target, interfaces, native_uuid, uid, euid, account)
    except PolicyError:
        decision, reason, interface = 'ssh', 'interface-inventory-unavailable', None
    # Bounded closed diagnostic, captured by normal logs. This is neither an
    # authorization receipt nor a new store/signature. Parent PID is the shell
    # launcher, which exec replaces with the normal supervisor on the local path.
    print(json.dumps({'schema':'fabric-locality-observation/v1','decision':decision,
                      'reason':reason,'target':target,'endpoint':PEERS[target]['tail'],
                      'native_machine_id':native_uuid,'interface':interface,'uid':uid,'euid':euid,
                      'selector_pid':os.getpid(),'exec_parent_pid':os.getppid(),
                      'authority':'existing-native-executor-only'},sort_keys=True),file=sys.stderr)
    return 0 if decision == 'local' else 3


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
    args = ["/usr/bin/nc", "-G", "2" if probe else "8"]
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
        args.extend(["/usr/bin/nc", "-G", "2", "-z", "-w", "2", address, "22"])
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
    local = commands.add_parser("dispatch")
    local.add_argument("target", choices=tuple(PEERS))
    render = commands.add_parser("ssh-config")
    render.add_argument("--helper", default=str(Path(__file__).resolve()))
    render.add_argument("--python", default="/usr/bin/python3")
    args = parser.parse_args(argv)
    try:
        if args.command == "ssh-config":
            print(render_config(args.helper, args.python), end="")
            return 0  # rendering never probes or opens a connection
        if args.command == "dispatch":
            return dispatch(args.target)
        proxy = runtime_proxy(args.transport, os.environ)
        kind, address, via = select_endpoint(origin_host(), args.target, proxy)
        print("fabric transport: " + args.target + " via " + kind + " " + address +
              (" through Studio " + via if via else ""), file=sys.stderr)
        # One raw tunnel, one SSH handshake, one remote command. No auth retries.
        tunnel = relay_ssh(via, proxy, address) if via else nc_args(address, proxy)
        os.execv(tunnel[0], tunnel)
    except MetadataCleanupError:
        print("REFUSED: native metadata child cleanup unconfirmed", file=sys.stderr)
        return 78
    except PolicyError as exc:
        print("REFUSED: " + str(exc), file=sys.stderr)
        return 78
    except OSError:
        print("REFUSED: native transport executable unavailable", file=sys.stderr)
        return 78


if __name__ == "__main__":
    sys.exit(main())
