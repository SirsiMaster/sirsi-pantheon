# Router Task Ledger

Router lifecycle mutations use declared acting identity. `sirsi router close`
and `sirsi router respond` accept `--agent <registered-id>`; when omitted, the
CLI resolves the current session identity from `SIRSI_AGENT_ID`, the session
marker, or one unambiguous live thread. The acting identity must be the item's
addressed recipient unless its registry entry carries the explicit `close:any`
supervisory capability. Delegated closes record both actor and recipient in the
durable Result. Ambiguous, undeclared, or unauthorized actors fail before the
item or response queue is mutated.

For Codex, `CODEX_THREAD_ID` identifies the current execution conversation and
is the preferred session-marker key. `CODEX_SESSION_ID` identifies the shared
root session and is a legacy fallback only when no thread ID is available. This
choice selects which existing marker to read; it does not register a thread,
grant authority, or persist parent/child lineage. Existing registered-agent,
thread, host, PID and compare-and-swap checks remain in force.

The task ledger answers two different questions together: “what messages are
open?” and “what work has each agent committed to?” It also shows age,
heartbeat staleness, dependency chains, and whether a live thread explicitly
picked an item through `current_item`.

## Inspect the ledger

```sh
sirsi router ledger
sirsi router ledger claude-nexus
sirsi router ledger claude-nexus --json
sirsi router ledger --stale-after 8h
```

The text view lists every open item with title, age, sender, and type. Per-agent
headers show the oldest age, stale state, blocked count, and
unblocked/unpicked count. `ctr` carries the same per-agent summary; use the
ledger verb for full detail.

## Register tasks

```sh
sirsi router task add claude-nexus sne-01 \
  --subject "Land the SNE backend" \
  --status in-progress \
  --responsible-party self

sirsi router task update claude-nexus sne-01 --status done
sirsi router task list claude-nexus
sirsi router task list --json
```

Statuses are `pending`, `in-progress`, `blocked`, and `done`.
`responsible-party` accepts `self`, `codex`, `owner`, or an agent id. Use
`--blocked-by <task-id>` when a task has a prerequisite; pass an empty
`--blocked-by` on update to clear it.

## Turn a router item into ledger work

```sh
sirsi router convert <item-id>                      # task on your ledger, id = item id
sirsi router convert <item-id> --agent claude-nexus --task sne-02
```

One transaction: the item closes with the result `converted to task <agent>/<task-id> by
<you>` and a `pending` task appears, with the item title as its subject and its sender and
instructions in the outline. If anything fails, neither happens. Running it again on the same
item returns the same task. An item already closed some other way is refused. Converting an
item addressed to another lane needs `close:any`, the same as `router close`. Over the router
service, a lane-scoped token may convert only items addressed to itself, onto its own ledger,
and the close result names the authenticated session.

## Declare item dependencies

```sh
sirsi router depend <item-id> <blocking-item-id>
sirsi router depend <item-id> -
```

The dash clears the dependency. Missing and cyclic dependencies show as
blocked. When the blocking item becomes terminal, the dependent item becomes
unblocked automatically.

## Staleness and pickup

The default stale window is four hours. An open item is stale when its assignee
has no usable heartbeat or its newest heartbeat is older than that window.
“Picked” is stricter: a non-terminal thread must name the exact item as its
`current_item`. A fresh session that has not selected the item remains
unpicked.
