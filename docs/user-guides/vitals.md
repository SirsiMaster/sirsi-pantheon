# `sirsi vitals` — the fast read on memory right now

**One-line:** total/used/free RAM, live pressure level, swap in use, and the
processes holding the most memory — sampled once, returned immediately.

## What it does

`vitals` is the quick memory snapshot for *this moment*: how much RAM is in use,
the current pressure level (normal / warn / critical), how much swap is in play,
and the top memory-consuming processes. It samples once and returns immediately —
built to power a ~2-second dashboard tick without the cost of a full diagnostic.

For the full 12-check system health diagnostic, run `sirsi diagnose` instead.

## Usage

```sh
sirsi vitals              # snapshot with the top memory users (default: top 5)
sirsi vitals --top 10     # show more memory users
sirsi vitals --json       # structured output for tools and dashboards
```

### Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--top` | `5` | Number of top memory users to include in the snapshot. |

## Example output

```text
𓁢 Memory Vitals
  Pressure   Swap      Used                 Free
  normal    12.5 GB   20.3 GB of 48.0 GB   27.7 GB

  TOP MEMORY USERS                  PID     MEMORY
  mediaanalysisd                    29361   1.5 GB
  codex                             74439   1.1 GB
  claude                            23966   745.4 MB
  Claude Helper (Renderer)          22727   689.5 MB
  Google Chrome Helper (Renderer)   3102    655.9 MB

  Sampled in 32ms · pressure source: bootstrap-snapshot

  ── What's Next ──
  sirsi relieve --memory   Flush inactive caches if memory is tight
  sirsi diagnose           Full 12-check health diagnostic
```

### JSON shape

```json
{
  "command": "sirsi vitals",
  "total_bytes": 51539607552,
  "used_bytes": 21826420736,
  "free_bytes": 29713186816,
  "swap_used_bytes": 13474526658,
  "pressure": "normal",
  "pressure_source": "bootstrap-snapshot",
  "top": [
    { "name": "mediaanalysisd", "pid": 29361, "rss_bytes": 1601175552 }
  ]
}
```

`pressure` is one of `normal` / `warn` / `critical`; `pressure_source` records
where the reading came from. `top` is ordered by resident memory (`rss_bytes`),
highest first, and honors `--top`.

## When to use

- You want a one-glance answer to "is memory tight right now, and what's eating
  it?" without waiting on a full diagnostic.
- Powering a live dashboard or menubar tick that refreshes every couple of
  seconds (`--json`).
- To decide whether `sirsi relieve --memory` is worth running.

## Safety notes

- **Read-only.** `vitals` only samples and reports — it never touches a process
  or frees memory. Act on it with `sirsi relieve --memory`.
- **On-device.** The snapshot stays on the machine; nothing is transmitted
  (Rule A11).

## Related

- **`sirsi relieve --memory`** — the non-destructive memory-pressure lever
  (`docs/user-guides/relieve.md`).
- **`sirsi diagnose`** — the full 12-check health diagnostic when you need depth
  over speed.
