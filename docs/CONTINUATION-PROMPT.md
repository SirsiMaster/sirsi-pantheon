# Continuation Prompt — INDEX (read this, then open YOUR thread's file)

> ⚠️ This file is a **router**, not a continuation. A single shared continuation collides:
> every thread overwrites it and a resumer can load the *wrong* one. Real continuations live
> per-thread in **`docs/continuations/<agent>-<workstream>-<date>-<session8>.md`**.
>
> **To resume:** find the row whose `agent` + `workstream` match who you are, open the file at
> the exact path, and follow it. Do NOT act on a row that isn't yours.

## Active continuations

| Agent | Workstream | Date | Exact path |
|-------|-----------|------|------------|
| `claude-pantheon` | releasable-single-install (native menubar + clean install/upgrade) | 2026-06-11 | `/Users/thekryptodragon/Development/sirsi-pantheon/docs/continuations/claude-pantheon-releasable-single-install-20260611-40374aa6.md` |

## Naming convention (so threads never collide)

```
docs/continuations/<agent>-<workstream-slug>-<YYYYMMDD>-<session8>.md
                    │         │                 │          └ first 8 of the session UUID (uniqueness)
                    │         │                 └ date the continuation was written
                    │         └ short workstream slug (what this thread is doing)
                    └ agent id (claude-pantheon, codex-pantheon, …)
```

When you write a new continuation: create the per-thread file above, then add/replace your row
in this table (one row per agent+workstream; keep the latest). The `<session8>` makes two same-day
runs of the same workstream distinct. Never write the full continuation into this index file.
