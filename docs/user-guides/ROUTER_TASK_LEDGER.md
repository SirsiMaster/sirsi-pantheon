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

## Finish a task: receipts

A task becomes `done` only on a receipt the router checks itself. Free text
("fixed it", a path, a URL) is refused.

```bash
sirsi router task complete <agent> <task-id> --lease <token> --receipt pr:SirsiMaster/sirsi-pantheon#1337
```

| Receipt | What the router checks |
|---|---|
| `pr:owner/repo#N` | GitHub says the PR is merged and its merge commit is on the default branch |
| `ci:owner/repo#RUN@SHA` | the Actions run finished `success`, on exactly that 40-character commit |
| `tag:owner/repo@TAG` | the tag exists; every check run on its commit (all pages) finished, none failed, and at least one succeeded — skipped/neutral alone is not enough |
| `maat:RESERVATION-ID` | the router's own Ma'at ledger shows that a host-token holder released, with exit 0, a reservation held under the task agent's name. **Not proof the run happened**: the exit code is what the releaser reported, and any host token can release in an agent's name (scoped cloud tokens cannot reserve or release). `maat:<run-dir>` naming a local `~/.sirsi/evidence/maat-runs/<run-dir>` is translated to its reservation id for convenience; the file is never the proof |

- **Bound.** Repos must belong to an allowed GitHub owner (`SirsiMaster` by
  default, `routerstore.ReceiptRepoOwners`); empty, `.` and `..` path segments are
  refused; and one receipt proves one task — a receipt already recorded for
  another task is refused.
- **Ma'at exit outcome.** A release only carries an exit status when it is made
  with `sirsi maat release <id> --exit-code N`. Until the launcher
  (`sirsi-maat-run`) passes it, `maat:` receipts are refused as unverifiable.
- **Fails closed.** If the router cannot reach GitHub or has no token, the task
  stays not-done and the error says the receipt is unverifiable. The service
  reads a read-only GitHub token from `SIRSI_ROUTER_GITHUB_TOKEN`; the owner
  provisions it into Secret Manager. Agents never create tokens.
- **Re-checked.** `sirsi router task recheck-receipts [--older-than 24h]` (and the
  supervisor's `receipt-recheck` duty, every 6 hours) re-verifies PR, CI and tag
  receipts. Only a receipt positively proven false reopens its task to `pending`
  with a timeline note `receipt no longer valid: …`: a PR no longer merged or
  whose merge commit left the default branch (history rewrite), a tag deleted
  from a repo the token can still read, a run whose conclusion changed. A network
  or token failure, or a bare 404 (expired Actions runs, token scope), never
  reopens anything: unknown is not invalid. A revert *commit* leaves the original
  merge reachable, so it is not detected. Ma'at receipts are ledger facts and are
  not re-checked. Reopen and refresh only apply to the exact receipt that was read.
- **Owner sign-off and cancel.** Work with nothing to build (a decision, a read)
  is closed by the owner with a required reason, recorded as tier `owner`. A task
  that will not be done is `canceled` with a reason; canceled never counts as done.
  Both are owner-only and are not served to lanes over the router.
- `--result-ref` is a deprecated alias of `--receipt`; it accepts only a valid receipt.
- No other path reaches `done`: `task add` cannot create a done or canceled task,
  and `task update` cannot set either status.
