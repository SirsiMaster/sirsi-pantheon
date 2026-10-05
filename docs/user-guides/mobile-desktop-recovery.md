# Authenticated mobile desktop recovery

`sirsi recovery serve` is Pantheon's narrow browser-to-RFB recovery bridge for
an existing, separately configured macOS Screen Sharing service. It is not a
VNC server, general TCP proxy, remote shell, permission-grant mechanism, or
Tailscale configuration tool.

## Boundaries

- The process only listens on a literal loopback address.
- An authenticated Tailscale Serve proxy is expected to be the sole upstream.
  The bridge requires an allowlisted `Tailscale-User-Login` header AND an
  independent recovery operator password verified against a local bcrypt hash.
  A loopback caller forging the header alone cannot obtain admission.
- Destinations are a closed JSON allowlist of literal RFC1918 or Tailscale IPs
  on port 5900. Browser requests cannot name a host, port, or TCP service.
- Each exact HTTPS origin receives a single, expiring, `Secure`, `HttpOnly`,
  host-only cookie. The browser receives no URL token. Recovery and Mac Screen Sharing credentials
  are entered transiently in the browser over HTTPS; the bridge does not log or
  persist them. noVNC handles Mac credentials in browser memory for AppleARD.
- A WebSocket may connect only once for the admitted node. Disconnect or expiry
  removes the session. The bridge does not launch or execute desktop payloads.
- Screen Sharing, TCC, FileVault, SIP, reboot, and Tailscale changes remain
  deliberate, separately authorized host operations.

## Start an approved bridge

Create an operator-owned configuration file containing only allowlists and password verifiers (never plaintext passwords).
Protect the file from other users; use a unique recovery password, not your Mac password:

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
  "operator_password_hashes": {"owner@example.com": "<bcrypt hash, cost 10 through 14>"},
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

A fresh browser sees a recovery operator sign-in page. It submits a same-origin
POST to the session endpoint with independent operator authentication, receives
the secure cookie, then reloads into noVNC. After expiry or disconnect, reopen
the client endpoint to authenticate again. Mac Screen Sharing may separately
request a username and password in noVNC. Neither credential is stored by
Pantheon; browser memory and the trusted TLS terminator remain inside the
credential trust boundary. The session endpoint checks the exact browser
`Origin` before minting the cookie. Put a separately reviewed, authenticated
Tailscale Serve route in front of `127.0.0.1:9188`; do not publish port 5900
or this loopback listener directly.

## Operator resolution path

If the page cannot open a desktop, it reports the affected node and whether
the recovery session was denied, expired, disconnected, or the existing RFB
service was unreachable. It never asks a user to paste a VNC secret. The first
safe fixes are: confirm the operator's tailnet identity is allowlisted, confirm
the exact configured origin, then have the Mac owner inspect Screen Sharing
and authorization state locally. Pantheon does not bypass those controls.
