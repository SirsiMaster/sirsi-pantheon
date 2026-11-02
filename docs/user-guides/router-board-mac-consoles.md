# Router board and Mac consoles

The **router board** is the hosted, read-only view of the work fabric: lanes, mailboxes,
threads and drill-down. Open it from any device after signing in with a sirsi.ai Google
account.

**Mac controls** (scan, doctor, guard, renice and the rest) never run in the cloud. Each
Mac serves its own console on the Sirsi tailnet, and the board's Host view links to every
admitted Mac. A Mac checks who you are before it answers:

- your device must be on the Sirsi tailnet, signed in with a **sirsi.ai** account;
- tagged machines and other domains are refused;
- a control must be sent from the console's own page, so another website can't press
  buttons through your browser.

## Turning a Mac's console on (host lane, once per Mac)

Recommended, on a private Unix socket:

```bash
sirsi dashboard --no-open --tailnet-socket ~/.sirsi/run/console.sock
tailscale serve --bg --https=443 unix:$HOME/.sirsi/run/console.sock
```

Or on a loopback port:

```bash
sirsi dashboard --no-open --tailnet-port 9120
tailscale serve --bg --https=443 http://127.0.0.1:9120
```

Point `tailscale serve` at the tailnet entrance, **never** at the keyboard port (9119), which
stays loopback-only and unchanged. The console refuses to start if the entrance can't be
bound, or if `--tailnet-port` is the same as the keyboard port or an alias port.

**What the entrance trusts.** Tailscale stamps the signed-in person on each request
(`Tailscale-User-Login`) and drops any copy a client sends. The entrance believes that
stamp, so anything that can connect to it directly can claim a sign-in:

- **Socket:** the socket is `0600` in a `0700` directory, so only you (and tailscaled) can
  connect. Tailscale's `serve --help` (1.104) lists `unix:` targets as supported on macOS.
  If your Tailscale build can't reach the socket, use the port.
- **Port:** any process on the same Mac can reach `127.0.0.1:9120`. Same-Mac processes are
  treated as inside the boundary, because they can already run the `sirsi` CLI as you.
  Nothing off the Mac can reach it, and requests from any non-loopback peer are refused.

Either way, a request with no identity, a blank identity or two identities is refused.

**Check it (host lane):**

```bash
scripts/router-service/verify-mac-console.sh entrance ~/.sirsi/run/console.sock
scripts/router-service/verify-mac-console.sh tailnet m1
```

Run the first on the Mac. It expects a 403 without an identity. Run the second from a
personal sirsi.ai device on the tailnet. It expects you to be admitted as yourself, and a
forged `Tailscale-User-Login` header to be ignored. From a tagged Mac, a 403 is the
correct answer: Tailscale sends no person for tagged machines.

## Which Macs appear

Every Mac in the machine registry (`.agents/idea-router/machines.json`) appears, at
`https://<name>.<tailnet>.ts.net`. Admitting a Mac is a merge to that file; the board shows
it on its next deploy. Two names that collide, or a name that is not a valid tailnet name,
stop the list rather than linking a button to the wrong Mac.

## If the board is down

The installed board app falls back to the first Mac console that passes a full health
check (HTTP 200, body `ok`, admitted for your sign-in). If none passes, it says so and
does not guess. The ledger itself lives in the router service; if that is down, every
console reports it.
