# `sirsi relieve` — hand the CPU (or memory) back

**One-line:** find the process saturating your CPU right now and lower its
priority so the app you're actually using regains responsiveness — reversible,
nothing killed.

## What it does

When your Mac beachballs or drops frames, `relieve` finds the process currently
hogging the CPU — the *live* cause, this moment — and lowers its scheduler
priority (renice +10 + background QoS) so your foreground app gets the cycles
back. It is gentle and reversible:

- Nothing is killed. The process keeps running; it just yields CPU.
- The priority change resets automatically when the process exits.
- Critical system processes (WindowServer, the kernel, audio, the session UI,
  and `sirsi` itself) are never touched — the renice routes through an
  A1-protected path that refuses them.

With `--memory`, it switches to relieving *memory* pressure instead: it names the
top memory user, then flushes macOS's inactive/cached pages with `purge` (the only
safe, non-destructive memory lever — it returns cached pages to the free pool
without touching any app).

## Usage

```sh
sirsi relieve                 # preview the top live CPU hog — no change (default)
sirsi relieve Chrome          # target a specific process by name
sirsi relieve --confirm       # actually lower its priority
sirsi relieve --memory        # preview a memory-pressure flush instead
sirsi relieve --memory --confirm   # flush inactive caches (asks for admin once)
```

### Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--confirm` | `false` | Apply the change. Without it, `relieve` only previews (Rule A1). |
| `--memory` | `false` | Relieve **memory** pressure by flushing inactive caches (via `purge`, needs admin) instead of the CPU renice. |
| `--min-cpu` | `15` | Only relieve a process above this %CPU. Below the floor, `relieve` reports there's nothing live to relieve. |

An optional positional argument (`sirsi relieve Chrome`) targets a process by
name instead of picking the top hog automatically.

## Example output

Preview (default):

```text
Process   Google Chrome Helper (GPU) (pid 3140)
CPU       82%

Top live offender: Google Chrome Helper (GPU) (pid 3140) at 82% CPU. Lowering
its priority hands the CPU back to your foreground app — reversible, nothing killed.

  ── What's Next ──
  Relieve it now   renice +10 + background QoS — reversible, resets when the process exits
```

When nothing is actually saturating the CPU:

```text
Nothing to relieve — no process is above 15% CPU right now (or only protected
system processes are busy). The hang isn't live this moment.
```

The `--memory` apply proves its result with before/after free memory and pressure
rather than claiming it:

```text
Free before   4.2 GB
Free after    9.8 GB
Pressure      warn → normal
```

## When to use

- Your Mac is beachballing or an app feels sluggish and you want to know *what's*
  causing it and hand the CPU back — without force-quitting anything.
- Memory pressure is high (`sirsi vitals` shows `warn`/`critical`) and you want a
  non-destructive way to ease it: `sirsi relieve --memory`.

## Safety notes

- **Reversible by design.** The CPU renice resets when the process exits;
  `--memory` only flushes OS caches (macOS re-caches as it needs to). Nothing is
  ever killed.
- **Protected processes refused.** Critical system processes are hardcoded off-
  limits. Processes you don't own need elevated privileges and are refused rather
  than force-touched.
- **Admin handled by macOS.** `--memory --confirm` runs `purge` through macOS's
  own admin-authorization dialog — your password goes to the OS, never to sirsi
  (Rule A11).

## Related

- **`sirsi vitals`** — the memory snapshot that tells you when `--memory` is
  worth running (`docs/user-guides/vitals.md`).
- **`sirsi diagnose`** — the full 12-check health diagnostic.
