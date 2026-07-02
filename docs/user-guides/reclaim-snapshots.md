# `sirsi reclaim-snapshots` — free disk from local Time Machine snapshots

**One-line:** thin the local APFS Time Machine snapshots quietly holding space on
your startup disk — the biggest painless win when you're low on space.

## What it does

macOS keeps *local* Time Machine snapshots on your startup volume even when your
real backups live elsewhere. They're useful, but they hold disk space you may
need. `reclaim-snapshots` counts them, shows how much disk is free now, and — on
confirm — thins them:

- Runs macOS's own tool: `tmutil thinlocalsnapshots /` at the most aggressive
  urgency (level 4).
- Removes **local** snapshots only. Your real Time Machine backups on an
  external or network drive are untouched.
- Reversible in the sense that macOS re-creates local snapshots automatically
  later.

The apply proves its result with disk-free **before and after** rather than
claiming a number.

## Usage

```sh
sirsi reclaim-snapshots            # preview: how many snapshots, disk free now (default)
sirsi reclaim-snapshots --confirm  # thin them (asks for your admin password once)
```

### Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--confirm` | `false` | Apply the reclaim. Without it, `reclaim-snapshots` only previews (Rule A1). |

## Example output

Preview (default):

```text
Local snapshots   6
Disk free now     18.4 GB

6 local Time Machine snapshot(s) on this disk. Thinning them frees space (your
real external/network backups are untouched). Runs `tmutil thinlocalsnapshots`
— asks for your admin password once.

  ── What's Next ──
  Reclaim snapshot space now   thins local APFS snapshots via the macOS admin prompt — external backups untouched
```

After `--confirm`:

```text
Disk free before      18.4 GB
Disk free after       31.9 GB
Snapshots remaining   1

Thinned local snapshots — reclaimed ~13.5 GB. Disk free: 18.4 GB → 31.9 GB.
Your external/network Time Machine backups are untouched.
```

When there's nothing to do:

```text
No local snapshots to reclaim — nothing to do.
```

## When to use

- Your startup disk is low on space and `sirsi diagnose` or Finder shows storage
  pressure.
- You have Time Machine enabled and want the fastest, safest space win before
  hunting through caches and build artifacts.

## Safety notes

- **Backups are never touched.** Only *local* snapshots on the startup volume are
  thinned. Your external/network Time Machine backups are safe.
- **macOS decides what to free.** Thinning delegates to `tmutil` — macOS frees
  what it can at the requested urgency; sirsi never deletes snapshot data itself.
- **Admin handled by macOS.** `--confirm` runs `tmutil` through macOS's own
  admin-authorization dialog; your password goes to the OS, never to sirsi
  (Rule A11).

## Related

- **`sirsi diagnose`** — the health diagnostic that flags disk pressure.
- **`sirsi clean`** — reclaim caches, logs, and build artifacts
  (`docs/user-guides/clean.md`).
