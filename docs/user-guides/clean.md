# `sirsi clean` — reclaim storage, safely

**One-line:** move the waste your last scan found to the Trash — dry-run by
default, so the amount you preview is exactly the amount you reclaim.

`clean` is the apply half of Anubis. `sirsi scan` finds the waste; `sirsi clean`
removes it. It never scans on its own and never deletes without a preview first.

## What it does

Reads the findings from your most recent `sirsi scan`, filters them to the items
that are safe to remove (caches, logs, temp files, stale build artifacts), shows
you exactly what would go, and — only when you confirm — moves those items to the
**Trash** (recoverable), not a permanent delete.

Two safety properties are enforced in code, not by convention:

- **Preview matches apply (Rule A1).** The set of items shown in the dry-run is
  the *identical* set that `--confirm` moves to Trash. `--confirm` cannot widen
  the scope — the selector that picks targets has no "confirm" input at all, so
  apply structurally cannot touch anything the preview didn't list.
- **Protected paths are hardcoded.** 25 protected paths (system roots, keychains,
  `.ssh`/`.gnupg`/`.git`, your `~/Desktop`, `~/Documents`, `~/Downloads`, etc.)
  are compiled in and **cannot be overridden** by any flag, config, or input.
  Every deletion passes this check — including the symlink-resolved real target,
  so an innocent-looking link can't smuggle a delete into a protected location.

## Usage

```sh
sirsi clean                     # preview safe items — changes nothing (default)
sirsi clean --confirm           # apply: move safe items to Trash (asks first)
sirsi clean --include-caution   # also target caution-tier items (preview + apply)
sirsi clean all                 # same as --include-caution
```

Run `sirsi scan` first — with no scan on record, `clean` stops and tells you to
scan.

### Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--dry-run` | `true` | Preview only. This is the default; `clean` is a no-op preview until you pass `--confirm`. |
| `--confirm` | `false` | Apply the cleanup — move items to Trash. Asks `[y/N]` first. Does **not** change *what* is cleaned, only whether the (already-final) set is applied. |
| `--yes` | `false` | Skip the interactive `[y/N]` prompt (use with `--confirm`). For dispatchers (TUI, menubar, dashboard) that already showed their own confirmation. `--yes` without `--confirm` is still a dry-run. |
| `--include-caution` | `false` | Also target caution-tier items. Applies to **both** the preview and the apply, so preview still matches apply. |

The positional `all` (`sirsi clean all`) is a back-compat alias for
`--include-caution`.

> **Caution tier vs. safe tier.** By default `clean` targets only *safe* findings
> (caches, logs, temp files). *Caution*-tier items — things like app preferences
> and ghost-app application-support data — are excluded unless you opt in with
> `--include-caution`. *Warning*-tier findings are never auto-cleaned by `clean`
> under any flag.

## Example output

A default preview (dry-run):

```text
𓃣 Cleanup
  Loaded 46 findings from scan at 22:03:11 (1.6 GB waste)
  Cleaning 28 findings (1.4 GB):
    System & App Caches — ~/Library/Caches/... (211.0 KB)
    System & App Logs — ~/Library/Logs/zoom.us (1.8 MB)
    ... and 18 more

  Dry run: 28 items (1.4 GB) would be cleaned
  Completed in 1ms

  Cleanable items     28
  Reclaimable space   1.4 GB

  ── What's Next ──
  sirsi clean --confirm   Apply cleanup (items moved to Trash)
  sirsi scan              Run a fresh scan to update findings
```

The `1.4 GB` shown is exactly what `--confirm` will move to Trash — no surprise
widening.

## When to use

- After `sirsi scan` reports reclaimable waste and you want it gone.
- When your startup disk is filling up with build and cache clutter.
- As the routine follow-up to `sirsi insight` when it flags Anubis with
  reclaimable space.

## Safety notes

- **Trash-first, always.** On macOS every removed item goes to the Trash and is
  recoverable. Nothing is permanently deleted by `clean`.
- **Every move is logged.** Each action is recorded to the operations ledger; see
  `sirsi activity` for the audit trail.
- **Scan-derived only.** `clean` can only act on paths a scan already surfaced —
  it does not walk the filesystem itself and cannot be pointed at arbitrary paths.
- **`--only` (advanced).** The `sirsi anubis clean` subcommand adds a repeatable
  `--only <path>` flag that restricts cleanup to specific paths a UI has curated.
  It is a pure intersection — it can only *narrow* the target set, never widen it
  — which is how the menubar and TUI let you clean a hand-picked subset. It is not
  exposed on the top-level `sirsi clean`.

## Related

- **`sirsi scan`** — find the waste `clean` acts on (`docs/user-guides/anubis.md`).
- **`sirsi activity`** — the ledger of every clean/purge `clean` performed
  (`docs/user-guides/activity.md`).
- **Rule A1** — Safety First (PANTHEON_RULES.md §2.1). Protected paths live in
  `internal/cleaner/safety.go`.
