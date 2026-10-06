# Router user guide

The router is a durable work queue that lets agents (Claude, Codex, or your own) hand work to each other and
wake when work arrives. It runs in two shapes: a **local ledger** on one machine, and a **shared service** that
many machines use at once.

## 1. Try it on one machine (no service)

With `SIRSI_ROUTER_URL` unset, every `sirsi router` command uses a SQLite ledger at `~/.sirsi/router.db`.

```bash
sirsi router send --from alice --to bob --title "review the schema" --instructions @note.md
sirsi router pull bob                     # list bob's open items
sirsi router acknowledge <id>             # says "received"; never closes the item
sirsi router close <id> --result @result.md
sirsi router ledger bob                   # open work, ages, dependencies
```

Bodies passed with `--instructions` and `--result` should always be `@file`. Inline text is evaluated by the shell.

Agents (`alice`, `bob`) must be declared in the agent registry (`agents.json`) before they can send or receive.

## 2. Wake an agent when work arrives

```bash
sirsi router wake-install bob             # installs a launchd job that waits for bob's inbox
sirsi router ping bob                     # can bob actually work right now?
```

`ping` reports one verdict per lane: LIVE, WAKEABLE, HELD, AUTH_REQUIRED, WATCH_ONLY, UNSTAFFED or UNREACHABLE.
The wake loop costs almost nothing while idle. It starts a headless session only when a gate allows it
(quarantine, idle CPU, back-off, an hourly ceiling, attended hold and a per-host consumer cap).

A lane that reads WATCH_ONLY has a loop but no usable consumer, usually a bad working directory or command in
its registry entry.

## 3. Share one ledger across machines

1. On the service host, run the service (`sirsi router serve`, see the runbook).
2. An administrator mints a token for each host (`sirsi router token mint <host>`).
3. On each machine, run the spool relay so lanes never hold the token: set `SIRSI_ROUTER_URL=https://<service>` and
   `SIRSI_ROUTER_TOKEN=<token>` for `sirsi router relay serve`, and set `SIRSI_ROUTER_URL=spool://<spool dir>` for lanes.
4. Register each session: `sirsi thread register --agent <id> --surface <claude|codex|...>`.

Unsetting `SIRSI_ROUTER_URL` on a node returns it to its local ledger. Items it wrote only to the service are not copied back.

## 4. What the commands do not do

- `acknowledge` never closes an item. `close` needs a result, and the owner's own items close only with `dismiss`.
- `quarantine` stops dispatchers from starting anything. It does not stop sessions already running.
- The router carries work items. It does not carry code, secrets or Stack Lab records; those go through git.
