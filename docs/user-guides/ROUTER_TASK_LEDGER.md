# Router Task Ledger

Router lifecycle mutations use declared acting identity. `sirsi router close`
and `sirsi router respond` accept `--agent <registered-id>`; when omitted, the
CLI resolves the current session identity from `SIRSI_AGENT_ID`, the session
marker, or one unambiguous live thread. The acting identity must be the item's
addressed recipient. Ambiguous, undeclared, or wrong-recipient actors fail
before the item or response queue is mutated.

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
