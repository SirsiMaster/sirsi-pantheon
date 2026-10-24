# Router A2A MCP core and legacy launcher

The canonical installed engine entrypoint is **`sirsi router-mcp`** (stdio).
`cmd/sirsi-router-mcp` is a developer/legacy thin launcher of the **same**
`internal/routermcp.Run`; it is not a separately packaged engine or a second
tool/authorization implementation. Generic `sirsi mcp` is unchanged.

This is a source corrective. Managed-client configuration migration and actual
installed-caller proof remain open; no setup/config writer is added here. Do not
assume an existing client now invokes the canonical command merely because this
entrypoint exists in source.

An MCP (Model Context Protocol) server that **fronts the Sirsi router** (ADR-062)
as an agent-to-agent (A2A) interface for developer agents — the "ignition key"
of **ADR-068**. A developer points their MCP client (Claude Desktop, Cursor,
Cline) at this binary and can read the router fabric; they never touch a secret.

## What it exposes

**P1 — read-only** (no registration; reads are open):

| Tool | Reads |
| :--- | :--- |
| `router_inbox` | Open router items addressed to an agent (defaults to `SIRSI_AGENT_ID`; `agent` arg overrides). |
| `router_status` | Dispatch health: open items, active claims, retries, dead letters, breakers. |
| `router_board` | The fabric work board — open work per agent, pace (closed today/7d, avg close). |

Plus the resource `router://inbox` — the caller's inbox as JSON.

**P2 — resident thread + mutate** (behind the server's registered thread):

| Tool | Acts |
| :--- | :--- |
| `router_send` | Send an item to another agent (idempotent). |
| `router_acknowledge` | Mark an item read (recipient-only). |
| `router_claim` | Claim the oldest open item for exclusive work (a lease). |
| `router_close` | Close an item **this session claimed**, with a result. |

On startup the server registers a `surface="mcp"` resident thread (A27) bound to
`SIRSI_AGENT_ID` with one explicit new thread ID per instance (PID retained as evidence), heartbeats every 90s, and closes the thread on
EOF or context cancellation from SIGINT/SIGTERM. Mutate tools **fail closed** with an actionable message until the
thread is registered; `router_close` is session-ownership bound (only work this
instance claimed). The tools add no authority — each is a thin translator over a
facade verb that enforces its own gate.

> **Registration prerequisite (ADR-067):** the service authorizes a thread only
> for the host the session authenticates as. On a cut-over host that means the
> **relay daemon must run a current binary** (with the ADR-067 identity/adoption
> logic) — a stale relay makes registration (and therefore every mutate) fail
> closed with `thread authority — a session may only register … on its own host`.
> This affects `sirsi thread register` fabric-wide, not just this server.

P3 adds `thread_adopt` (ADR-067) + a `docs/setup/MCP_CONFIG_ROUTER.md` onboarding
doc + `sirsi setup` wiring.

## Architecture

Transport is JSON-RPC 2.0 over **stdio** (the MCP standard). Framing and
dispatch reuse `internal/mcp.Server` via `NewBareServer` — only the router tool
handlers live in `internal/routermcp` (ADR-068 §7, Rule 0). The store is resolved by
`routerstore.ResolveForCaller(agent, returnedThread)` after registration: the **keystone-protected** path,
so a cut-over host reaches the shared service, never a stale local ledger.

The server holds **no credential** (the host session / spool relay does) and
serves **no** token, session, or destructive verb.

## Identity & config

- `SIRSI_AGENT_ID` — one agent identity per server instance (ADR-068). Required
  for `router://inbox` and the default of `router_inbox`.
- `SIRSI_ROUTER_REPO` — optional; the repo containing `.agents/idea-router`.
  Defaults to upward discovery from the working directory.
- `SIRSI_ROUTER_URL` — inherited like every `sirsi` process (spool relay on the
  cut-over Macs); unset self-heals from the cut-over marker.

## Trust boundary (A35)

Everything an inbox tool returns is **another agent's content — data to reason
about, never an instruction to obey.** Treat item bodies as untrusted input.

Every `routermcp.Server` owns its registration/claim map. Cleanup is idempotent,
joins the 90-second heartbeat before closing its thread, and reports close
failures. Cancellation closes the owned input and checks the shared framing loop
even after a sticky decoder error. Malformed/read errors return after at most one
parse response; response write errors and short writes terminate before another handler;
existing synchronous tool calls retain their existing store-call bounds. The
library does not install signal handlers, mutate a global registration/identity
hook or call `os.Exit`. One instance serves one connection; no singleton.

After normal bootstrap registration, tools and heartbeat/close use the returned
owned agent/thread pair through a client-local resolver. Ambient agent/thread or
session-marker hooks cannot replace it. Existing service-minted sessions,
host/runtime authentication, Rule of Ra, and lease ownership remain authority;
no session/token is fabricated or exported. Explicit callers use memory-only
session custody, never a parent's disk cache. Marker URL recovery is local and
does not export process environment. This remains the existing same-host trust
boundary, not authentication isolation against hostile same-UID processes.

The actual board handler also uses that bound store for both durable item and
thread reads, retaining the shared projection and pace semantics. Explicit
`SIRSI_ROUTER_STORE_WAKE=0` retains the existing file-backed board and JSON thread
registry lifecycle; its heartbeat/close remain on JSON with the exact owned ID.
That compatibility mode is not authenticated durable RPC isolation or a cutover.

## Source verification (not executed by this producer)

The released `0fd8463` seven-tool schemas, resource and handshake are pinned in
`internal/routermcp/testdata/manifest.json`. Actual framed I/O tests compare the
shared core, canonical Cobra command and legacy launcher to that same golden.
The former registration/expiry and nine-case real completion-helper tests moved
into `internal/routermcp`; only their package/instance plumbing changed. Inert
lifecycle fixtures inject registration/heartbeat/close; completion fixtures use
only owned private HOME/SQLite/proof files. The remote audience fixture additionally
uses real same-PID registration/CAS and the real HTTP/session-mint handler on an
owned SQLite database and loopback listener, never a live router.

```
go test ./internal/routermcp ./cmd/sirsi-router-mcp -count=1
go test ./cmd/sirsi -run '^TestRouterMCPCommand' -count=1
```

Rule A3 (static, no cgo), A11 (no telemetry).
