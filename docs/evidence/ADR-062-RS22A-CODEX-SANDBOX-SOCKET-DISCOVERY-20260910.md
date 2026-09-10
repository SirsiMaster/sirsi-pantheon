# ADR-062 step 20a.1 — can a codex workspace-write sandbox reach a unix socket? (2026-09-10T03:16Z, M5)

Setup: a 20-line Python responder bound to `~/.sirsi/discovery.sock` (mode 0600, owned by the same uid as codex), answering any request with `socket-pong`. Prompt to codex, verbatim: *Run exactly this shell command and paste its raw stdout and stderr verbatim, nothing else: `curl -sS --unix-socket ~/.sirsi/discovery.sock http://x/ping; echo exit=$?`*

| Run | Command | Raw codex output |
|---|---|---|
| control | `curl --unix-socket …` outside any sandbox | `socket-pong` |
| A | `codex exec --sandbox workspace-write` (no network_access) | `curl: (7) Failed to connect to x port 80 after 0 ms: Couldn't connect to server` / `exit=7` |
| B | `codex exec --sandbox workspace-write -c sandbox_workspace_write.network_access=true` | `socket-pong` / `exit=0` |

**Result: NO.** The seatbelt profile behind `network_access=false` blocks unix-domain socket connects as well as TCP/DNS. A unix-socket relay would still require the network exception, so it delivers nothing the exception does not. **20a.1b fallback design (filesystem spool) governs the rest of step 20a**; the socket steps 20a.2–20a.4 are superseded as written in `docs/ROUTER_SERVICE_GOAL.md`. codex-cli 0.153.4, macOS 25.6.0, M5 host `Mac`.
