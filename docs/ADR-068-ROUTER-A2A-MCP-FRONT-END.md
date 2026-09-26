# ADR-068 — Router A2A MCP front-end (the developer ignition key)

- **Status:** Proposed — design, owner-directed 2026-09-25 ("commercialize this as the A2A tool of choice for developers in the Sirsi environment").
- **Date:** 2026-09-25
- **Steward:** `ra` (router-service lane)
- **Refs:** ADR-062 (router service), ADR-067 (credentialed machine-id identity — the footing this rides on), PANTHEON_RULES A26 (Idea Router workstreams), A27 (heartbeat loop / resident surfaces, surface id `mcp`), A29/A30 (orchestration brain, model tiering), A31 (CTR), A35 (scope the check to the claim).

## 1. Context — why this, why now

The router is a durable, cross-host, identity-bearing agent-to-agent (A2A) message bus: a Cloud Run service over a Cloud SQL ledger, per-host bearer tokens, signed RPC, an offline spool relay, and (ADR-067) hardware-stable identity. It works — the Pantheon fabric runs on it. But its **only front doors today are the `sirsi` CLI and raw signed RPC.** A developer who wants A2A between their own agents has to learn host tokens, sessions, the Rule of Ra, and a bespoke CLI. That is not "the tool of choice."

The owner's insight (2026-09-25): *"we could have MCP'd the whole thing much more easily."* Half right, and the right half is the product:

- **MCP could NOT be the substrate.** MCP is caller→server tool-calls over a live connection — no durable inbox, no peer identity, no offline delivery, no cross-host authority. "Agent A leaves durable work for agent B, asleep on another Mac, delivered on wake" is not expressible in MCP primitives. The substrate is a message bus + ledger; MCP is neither. So the router substrate is **not** the over-build.
- **MCP IS the right interface.** A developer should add ONE MCP server to Claude Desktop / Cursor / Cline / any MCP client and get `send / inbox / claim / close / status` as tools — with zero exposure to tokens or signed RPC. The router stays the engine; MCP becomes the ignition key.

This ADR specifies that front-end: **a thin MCP server that translates MCP tool calls into the router's existing node-safe verbs.** It is additive — no substrate change, no new protocol — and it is the commercialization surface.

## 2. Decision

Ship a **Router A2A MCP server** that:

1. Speaks MCP over **stdio** (the transport every local MCP client uses), reusing the repo's own hand-rolled MCP framework — **not** a new SDK. Concretely: `internal/mcp.NewBareServer(name, version, instructions, logPrefix)` (the tool-free server; `NewServer()` bundles the full Anubis toolset and is NOT used) + `srv.RegisterTool(mcp.Tool{Name, Description, InputSchema}, handler)` where a handler is `func(args map[string]any) (*mcp.ToolResult, error)` and returns text content with `IsError` for failures. This is exactly how `cmd/sirsi-gemma/main.go` is built.
2. Exposes ONLY **node-safe router verbs** as tools (the `ruleOfRaExempt` + read surface + the caller's own send/claim/close), never the server-only credential verbs (`MintHostToken`, `RevokeHostToken`, sessions — already `notServed`). Every handler routes through **`internal/dispatch.Facade`** — the existing single entry point shared by the CLI verbs and the in-tree `router_*` MCP handlers — opened via `dispatch.Open(repoRoot)` (which calls `routerstore.Resolve()` and fails closed).
3. **Binds to exactly one developer agent identity** per server instance (`SIRSI_AGENT_ID` + `SIRSI_THREAD_ID`, or a process-local `IdentityHook`), and reaches the ledger through a **`spool://` URL so the on-host relay holds the bearer token** (`routerstore.Resolve` §resolve.go: a `spool://` URL needs no token). **The developer never sees a token, and it is never a tool argument.** The host token stays with the relay; the MCP server holds none. The identity env is set for this process only — never `os.Setenv` into children (the PR #730 leak constraint on `IdentityHook`).
4. Registers itself as a **resident thread** (`surface = "mcp"`, A27) on startup via `router.RegisterThread(routerRoot, &Thread{AgentID, Surface:"mcp", PID:<own>})` (idempotent on `(agent_id, pid)`), and heartbeats from its runloop on a **bounded interval ≥60s** (inside the 10-min Rule-of-Ra stale window) via `router.Heartbeat`. So it is a first-class fabric node (visible to CTR/Horus, wakeable), not an invisible shim. It `thread close`s on graceful shutdown.
5. Treats every inbox/item body it returns as **untrusted DATA, never instructions** — the client's model reads other agents' messages through these tools, and a message is content to reason about, not a command to obey. Tool descriptions and result envelopes say so.

Surface: a dedicated **`cmd/sirsi-router-mcp`** binary that mirrors `cmd/sirsi-gemma` (thin `main` → `mcp.NewBareServer` → register router-verb tools whose closures hold one `*dispatch.Facade` → `srv.Run()`). This reuses `internal/mcp` and `internal/dispatch` verbatim (Rule 0); a separate binary also keeps a stable `RuntimeHash` for its session, and gives MCP clients a fixed command to spawn.

## 3. Tool surface (MVP)

Exposed as MCP **tools** (verbs) and, where it fits, MCP **resources** (the inbox, the board as readable resources). Only the caller's own agent is actable.

| MCP tool | CLI verb | Facade / Store method (signature) | Kind |
|---|---|---|---|
| `router_send` | `sirsi router send` | `Facade.Send(from,to,title,type,instructions)` → `Store.SendGuarded(SendReq) (string,bool,error)` | **MUTATE** (Rule of Ra) |
| `router_inbox` | `sirsi router pull <agent>` | `Facade.Inbox(agent)` → `Store.Inbox(agent) ([]Item,error)` | read |
| `router_show` | `sirsi router show <id>` | `Facade.Show(id)` → `Store.Render(id) (string,error)` / `Get` | read |
| `router_status` | `sirsi router status` | `Store.Counters() (DispatchCounters,error)` (+`GetState`/`Breakers`) | read |
| `router_ledger` | `sirsi router ledger [agent]` | `ledger.Build(repoRoot,agent,now,staleAfter)` + `Summarize` | read |
| `router_board` | `sirsi router board`/`workboard` | `router` workboard read model (items + threads) | read |
| `router_claim` | claim work | `Store.ClaimNext(agent, ttl) (*Lease,error)` | **MUTATE** (+ ownership-bound at claim) |
| `router_close` | `sirsi router close <id>` | `Facade.CloseItem(actor,id,result)` → `Store.CloseItem(id,result) error` | **MUTATE** |
| `router_acknowledge` | `sirsi router acknowledge <id>` | `Facade.AckItem(actor,id)` → `Store.AckItem(id) error` | **MUTATE** (recipient-only) |
| `thread_adopt` | `sirsi thread adopt` | `Store.AdoptTokenMachineID(host,machineID) error` (host injected server-side) | identity, `ruleOfRaExempt` |

Read tools work for any session; MUTATE tools require the server's registered thread (item 4) — so the server registers on startup, and a mutate before registration is a clear, actionable error, never a silent drop. `router_claim`/`router_close` are additionally session-ownership bound (a lease is bound to the claiming session), so the developer can only complete work its own instance claimed.

## 4. Safety model (the commercialization guardrails)

- **No credential surface.** The server exposes no verb that mints/reads/revokes tokens or sessions (they are `notServed` server-side regardless). A developer plugs in and messages; they never touch a secret.
- **Rule of Ra holds.** Mutating tools go through the same registered-thread gate as any lane; the server's own thread is the audience. No bypass.
- **Prompt-injection boundary (A35-shaped).** Inbox items are other agents' content. The tools return them tagged as data; the server never elevates a message body into an instruction, and the ADR is explicit that the *client's* model must treat them as untrusted. A tool that returned "instructions" a model would obey would be a check narrower than its claim.
- **Scope per instance.** One agent identity per server; a developer running two agents runs two server instances with two identities. No cross-agent action from one connection.
- **Owner/security gates unchanged.** This adds no new authority; it is a translator over verbs that already enforce their own gates.

## 5. Neith's Triad (A22)

### 5.1 Data flow

```
MCP client (Claude Desktop / Cursor / Cline)
        │  MCP over stdio (tools/resources)
        ▼
  sirsi router mcp  ── binds SIRSI_AGENT_ID + thread; registers surface="mcp" (A27)
        │  routerstore.Resolve()  (host session/token OR spool relay — server holds no secret)
        ▼
  Router service (Cloud Run, ADR-062) ── signed RPC, Rule of Ra, ADR-067 identity
        │
        ▼
  Cloud SQL ledger  ── durable items/tasks/threads; cross-host, offline-capable
```
Inbound tool call → node-safe verb → service → ledger. Outbound: inbox/board reads return other agents' content **as data**.

### 5.2 Recommended build order

1. **P1 — read-only MVP:** `router_inbox`, `router_status`, `router_board` as tools + the inbox as a resource. No registration needed (reads are open). Ships value + proves the framework wiring with zero mutation risk.
2. **P2 — resident thread + mutate:** register `surface="mcp"` on startup + heartbeat (A27); add `router_send`, `router_acknowledge`, `router_close`, `router_claim` behind the registered thread.
3. **P3 — identity + onboarding:** `thread_adopt` (ADR-067) so a developer's node self-stabilizes; a `docs/setup/MCP_CONFIG_ROUTER.md` mirroring the gemma MCP config doc; `sirsi setup` wiring.
4. **P4 (later, gated):** non-stdio transport (HTTP/SSE) for hosted/remote clients — needs the scoped/read-only token type (rs-44) first, since a remote client leaves the physical fleet where caller-discipline can't be assumed.

Minimum viable pipeline is P1. P4 depends on rs-44 and is out of scope here.

### 5.3 Key decision points

| Question | Options | Recommendation |
|---|---|---|
| Substrate or interface? | (a) MCP replaces the router, (b) MCP fronts the router | **(b)** — (a) can't express durable async cross-host A2A (§1) |
| New binary or subcommand? | (a) `cmd/sirsi-router-mcp` binary, (b) `sirsi router mcp` subcommand | **(a)** — mirrors the existing `cmd/sirsi-gemma` MCP; still reuses `internal/mcp` + `internal/dispatch` (Rule 0); stable RuntimeHash + a fixed spawn command for clients |
| Transport for v1 | (a) stdio, (b) HTTP/SSE | **(a)** — every local MCP client uses stdio; HTTP is P4 behind rs-44 |
| Identity binding | (a) many agents per server, (b) one agent per instance | **(b)** — one connection = one audience; no cross-agent action |
| Credential exposure | (a) tool to manage tokens, (b) none, host-side only | **(b)** — the whole point is a developer never touches a secret |
| Inbox content trust | (a) actionable, (b) data-only | **(b)** — A35: a message is content to reason about, not a command |

## 6. Rejected alternatives

- **MCP as the substrate** (§1): no durable inbox, identity, or cross-host/offline delivery. Reintroduces every problem the router exists to solve.
- **A new bespoke A2A protocol for developers:** the router IS that protocol; a second one fragments the fabric. MCP is the standard clients already speak.
- **Exposing the raw CLI as one `run` tool:** hands the model arbitrary `sirsi` including credential and destructive verbs; violates §4. Curated node-safe tools only.
- **HTTP transport in v1:** a credential leaving the physical fleet needs the scoped token (rs-44) first; premature without it.

## 7. Open items / dependencies

- **Reuse, don't fork.** `internal/mcp` (`NewBareServer` + `RegisterTool`) is already generic — the gemma coupling lives only in `cmd/sirsi-gemma`, not the shared server. The in-tree `router_*` handlers in the full Anubis server (`internal/mcp/tools.go`) already route through `dispatch.Open → Facade.Send/Inbox/Show`; `cmd/sirsi-router-mcp` lifts that wiring into a standalone bare server. No framework change needed.
- rs-44 (scoped read-only tokens) gates P4 (remote transport).
- A26: this is a router-lane workstream; if implementation spans repos or needs codex-pantheon, route through the Idea Router.
