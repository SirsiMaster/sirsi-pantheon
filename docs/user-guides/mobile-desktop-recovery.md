# Authenticated mobile desktop recovery

`sirsi recovery serve` is Pantheon's narrow browser-to-RFB recovery bridge for
an existing, separately configured macOS Screen Sharing service. It is not a
VNC server, general TCP proxy, remote shell, permission-grant mechanism, or
Tailscale configuration tool.

## Boundaries

- The process only listens on a literal loopback address.
- Authenticated Tailscale Serve provides private encrypted transport, but is
  not the admission authority. The browser must also present a short-lived
  Ed25519-signed Pantheon recovery capability. The gateway holds public keys
  only and rejects a forged local `Tailscale-*` identity header.
- Destinations are a closed JSON allowlist of literal RFC1918 or Tailscale IPs
  on port 5900. Browser requests cannot name a host, port, or TCP service.
- Each exact HTTPS origin receives a single, expiring, `Secure`, `HttpOnly`,
  host-only cookie. The recovery capability is submitted in a POST body, never
  in a URL. A create-only local claim ledger retains only a hash-derived
  single-use record through signed expiry, so a bridge restart cannot reuse an
  admitted capability.
- A WebSocket may connect only once for the admitted node. Disconnect or expiry
  removes the session. The bridge does not launch or execute desktop payloads.
- Screen Sharing, TCC, FileVault, SIP, reboot, and Tailscale changes remain
  deliberate, separately authorized host operations.

## Start an approved bridge

Create an operator-owned configuration file with public keys only. The public
key below is illustrative; the corresponding private signing key remains in a
separately authenticated Pantheon authority and never belongs in this file.

```json
{
  "nodes": [
    {
      "id": "m1",
      "address": "100.88.242.95:5900",
      "origins": ["https://m5.example.ts.net"]
    }
  ],
  "admission_public_keys": {
    "pantheon-ra-operator-2026q4": "base64url-encoded-ed25519-public-key"
  },
  "admission_claim_dir": "/Users/operator/Library/Application Support/SirsiPantheon/recovery-claims",
  "session_ttl_seconds": 600
}
```

`admission_claim_dir` must already be an absolute directory owned by the user
running the bridge, with permissions 0700 (access for that user only). Startup
and each new admission check these permissions. A changed owner or mode denies
new admissions; the bridge never repairs permissions or deletes replay records.
The bridge opens it without following a symlink and only creates opaque,
create-once claim records. It never receives a signing key or an RFB password.

Run the bridge locally:

```sh
sirsi recovery serve --config recovery.json --listen 127.0.0.1:9188
```

The first browser endpoint is:

```
https://<your-Serve-name>/recovery/v1/nodes/m1/client
```

The first visit opens an operator entry page rather than returning a dead-end
401. The signed recovery capability is submitted through that form, checked
against the exact node and origin, then exchanged for the cookie and embedded
noVNC page. Put a separately reviewed, authenticated Tailscale Serve route in
front of `127.0.0.1:9188`; do not publish port 5900 or this loopback listener
directly.

## Operator resolution path

If the page cannot open a desktop, it reports the affected node and whether
the signed admission was denied, expired, disconnected, or the existing RFB
service was unreachable. The first safe fixes are: obtain a new signed recovery
admission, confirm the exact configured origin, then have the Mac owner inspect
Screen Sharing and authorization state locally. Pantheon does not bypass those
controls.

After transport admission, Apple's Screen Sharing service may require the
operator to enter an Apple Remote Desktop username and password in noVNC's
standard browser prompt. Pantheon does not broker, save, put in a URL, or log
those RFB credentials. They exist transiently in the browser/RFB handshake;
use a trusted device and decline browser password persistence for a recovery
session.

## Disconnect and hardware holds

Use noVNC's Disconnect action to close the desktop transport. An authorized
same-origin client may also POST `/recovery/v1/nodes/<node>/disconnect` using
its session cookie; the bridge cancels that session and clears the cookie.
Stopping the bridge cancels all sessions, including open WebSockets. Apple input
release and preservation of local control require live qualification.

Compute admission remains separate. This release-source slice does not transplant
the old SNE supervisor; resource HOLD does not remove desktop recovery access.

The bridge remains a development candidate pending ADR-075 security acceptance
and real M1 phone/M5 alternate-anchor qualification.

## Existing Universal Control peer reconnect

For an already authorized and discoverable Mac peer, open System Settings on
the controlling Mac, then Displays → Add → Link keyboard and mouse to, and
select the existing peer. Confirm the displays appear and verify actual mouse
and keyboard input on both Macs. Discovery or a “Connection Ready” log alone
does not prove input works.

This procedure restored M5-to-M1 control in the owner-reported 2026-10-05
incident. It does not diagnose the recurrent wireless/sync errors, establish a
permanent repair, or qualify phone recovery. If it recurs, preserve timestamped
discovery, connected-session and actual-input outcomes separately before
changing host settings.
