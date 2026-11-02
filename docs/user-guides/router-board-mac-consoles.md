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

```bash
sirsi dashboard --no-open --tailnet-port 9120
tailscale serve --bg --https=443 http://127.0.0.1:9120
```

Point `tailscale serve` at the tailnet port (9120), **never** at the keyboard port (9119).
The keyboard port stays loopback-only and unchanged.

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
