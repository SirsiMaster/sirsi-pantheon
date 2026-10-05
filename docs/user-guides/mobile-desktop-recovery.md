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
  in a URL, and is not retained after admission.
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
  "session_ttl_seconds": 600
}
```

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
