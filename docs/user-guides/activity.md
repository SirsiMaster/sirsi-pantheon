# `sirsi activity` — the trust ledger

**One-line:** every destructive operation sirsi has performed — clean and purge —
recorded and read back, newest first, with the size it affected.

## What it does

`activity` is the provenance ledger. Every destructive operation sirsi runs
(`clean`, `purge`) is appended to `~/Library/Logs/sirsi/operations.log` with its
timestamp, action, target path, and bytes affected. This command reads that log
back, newest first — so you can always answer "what did sirsi actually touch, and
when?"

## Usage

```sh
sirsi activity              # last 20 operations (default)
sirsi activity --limit 5    # fewer (or more) entries
sirsi activity --limit 0    # all entries
sirsi activity --json       # structured output for tools and dashboards
```

### Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--limit` | `20` | Maximum entries to show. `0` means all. |

## Example output

```text
𓁢 Activity — Operations Ledger
  TIME                  ACTION   TARGET                                    SIZE
  2026-07-01T22:04:00   purge    …/001/node_modules                        100 B
  2026-06-29T19:54:21   clean    …/refs/heads/fix/flaky-router-ack…        —

  5 operations (newest first) · ledger: ~/Library/Logs/sirsi/operations.log
```

### JSON shape

```json
{
  "command": "sirsi activity",
  "log_path": "/Users/you/Library/Logs/sirsi/operations.log",
  "count": 2,
  "entries": [
    {
      "time": "2026-07-01T22:04:00",
      "action": "purge",
      "target": "/…/001/node_modules",
      "bytes": 100,
      "source": "oplog"
    }
  ]
}
```

`entries` are newest-first; `count` is how many were returned; `log_path` is the
ledger location on this machine.

## When to use

- To audit what sirsi has removed — the recoverable proof behind every `clean`
  and `purge`.
- Before recovering an item from Trash: find the exact path and timestamp here.
- In a dashboard or compliance script (`--json`) that needs a record of
  destructive operations.

## Safety notes

- **Read-only.** `activity` only reads the ledger; it changes nothing.
- **Local-only.** The operations log lives on your machine and is never uploaded
  (Rule A11).
- **Opt out entirely.** Set `SIRSI_NO_OPLOG=1` to disable ledger writing; with it
  set, destructive operations are not recorded and `activity` will show nothing
  new.

## Related

- **`sirsi clean`** — the operation whose actions this ledger records
  (`docs/user-guides/clean.md`).
- **`sirsi purge`** — remove stale build artifacts (also recorded here).
