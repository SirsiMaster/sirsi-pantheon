# Router snapshot contract (`/api/router` and `sirsi router snapshot`)

One producer (`collectDashboardRouter`), two transports: the web dashboard reads `GET /api/router`; the native menubar runs `sirsi router snapshot` (always JSON). They cannot disagree. A consumer renders these fields and never recomputes a verdict, an attention item or a next step. Field names are pinned by `TestSnapshotJSONContractFieldNames`.

| Field | Meaning | Used by |
|---|---|---|
| `generated_at`, `version`, `built_ms`, `timings_ms{stage}` | When the snapshot was built, by which binary, and how long each stage took | freshness line, load/stale design |
| `lanes.counts{VERDICT: n}` | All seven verdicts (LIVE, WAKEABLE, HELD, AUTH_REQUIRED, WATCH_ONLY, UNSTAFFED, UNREACHABLE) always present, zero included | overview verdict bar |
| `lanes.list[]` `agent, verdict, detail, open` | One row per lane: the same verdict and reason as `sirsi router ping` | lanes, fleet |
| `lanes.list[]` `observed_at, worker_thread_id, last_report_at, last_report_summary` | Evidence. Projected from the worker's OWN published lane state. Omitted means unknown, never healthy; never inferred from a heartbeat | lane inspector |
| `queue[]` `agent, open` | Open items per recipient, most first | queue, top recipients |
| `consumers` `running, max` | Headless sessions running against the host cap | overview |
| `registry` `pinned, source, commit, fetched_at` | Whether the agent registry is pinned to origin/main | host, attention |
| `known_failures[]` `id, title, status, fixed_in, guard` | The known-failure catalog | failures |
| `swap` (optional) `verdict, used_mib, total_mib, free_pct, delta_pages, ...` | Last swap-hygiene receipt; absent means no receipt | host, overview |
| `releases[]` `version, date, items[]` | Changelog sections, newest first. `[Unreleased]` is the first entry; `items` is `[]` (never null) when empty | releases |
| `attention[]` `id, severity, agent, title, detail, action, next` | What needs a person or a fix now, most severe first, ids stable across refreshes. Empty means nothing does | overview, badges |
| `attention[].next` `kind, label, command` | A typed next step. `kind` is always `command-copy`: show it and copy it, never run it | attention, lane inspector |

## Allowlisted next-step commands (the only ones the producer emits)
`sirsi router ping <lane>`, `sirsi router wake-install <lane>`, `sirsi router registry sync --install`, `sirsi maat known-failures list`, `sirsi swap-hygiene --status`.

## Deliberately NOT in the snapshot
Item-level data (item ids, subjects, ages, acknowledgements, lease owners, dependencies). No item projection exists and aggregate counts cannot establish those facts. Native views that need them use the existing per-item router verbs (`sirsi router pull`, `show`, the owner-action methods). A snapshot-level item projection needs its own specified contract first.

## Failure behaviour
`sirsi router snapshot` exits non-zero with a message on stderr when the router cannot be read; it never prints a partial snapshot. The HTTP endpoint answers 502 with `{"error": ...}`. Consumers keep their last good snapshot, show its `generated_at`, and say it is retained.

## Fixtures
Live captures and a synthetic "sick" snapshot covering every severity and verdict are kept with the dashboard redesign handoff; regenerate live ones with `sirsi router snapshot > fixture.json`.
