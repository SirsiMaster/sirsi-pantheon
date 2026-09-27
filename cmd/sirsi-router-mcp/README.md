# sirsi-router-mcp

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
`SIRSI_AGENT_ID` + its PID, heartbeats every 90s, and closes the thread on
SIGINT/SIGTERM. Mutate tools **fail closed** with an actionable message until the
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
handlers live here (ADR-068 §7, Rule 0). The store is resolved by
`routerstore.Resolve()` inside `dispatch.Open`: the **keystone-protected** path,
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

## Build

```
go build ./cmd/sirsi-router-mcp/
```

Rule A3 (static, no cgo), A11 (no telemetry).
