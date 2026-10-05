# Authenticated mobile desktop recovery

`sirsi recovery serve` is Pantheon's narrow browser-to-RFB recovery bridge for
an existing, separately configured macOS Screen Sharing service. It is not a
VNC server, general TCP proxy, remote shell, permission-grant mechanism, or
Tailscale configuration tool.

## Boundaries

- The process only listens on a literal loopback address.
- An authenticated Tailscale Serve proxy is expected to be the sole upstream.
  The bridge accepts the `Tailscale-User-Login` identity header only from that
  loopback proxy and matches it against a local allowlist.
- Destinations are a closed JSON allowlist of literal RFC1918 or Tailscale IPs
  on port 5900. Browser requests cannot name a host, port, or TCP service.
- Each exact HTTPS origin receives a single, expiring, `Secure`, `HttpOnly`,
  host-only cookie. The browser never receives an RFB password or URL token.
- A WebSocket may connect only once for the admitted node. Disconnect or expiry
  removes the session. The bridge does not launch or execute desktop payloads.
- Screen Sharing, TCC, FileVault, SIP, reboot, and Tailscale changes remain
  deliberate, separately authorized host operations.

## Start an approved bridge

Create an operator-owned configuration file with no credentials:

```json
{
  "nodes": [
    {
      "id": "m1",
      "address": "100.88.242.95:5900",
      "origins": ["https://m5.example.ts.net"]
    }
  ],
  "allowed_tailnet_logins": ["owner@example.com"],
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

The session endpoint checks the browser `Origin`, mints the cookie, and sends
the client to the embedded noVNC page. Put a separately reviewed, authenticated
Tailscale Serve route in front of `127.0.0.1:9188`; do not publish port 5900
or this loopback listener directly.

## Operator resolution path

If the page cannot open a desktop, it reports the affected node and whether
the recovery session was denied, expired, disconnected, or the existing RFB
service was unreachable. It never asks a user to paste a VNC secret. The first
safe fixes are: confirm the operator's tailnet identity is allowlisted, confirm
the exact configured origin, then have the Mac owner inspect Screen Sharing
and authorization state locally. Pantheon does not bypass those controls.
