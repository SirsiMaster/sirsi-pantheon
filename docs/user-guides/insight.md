# `sirsi insight` — cross-deity state of the union

**One-line:** one deterministic, platform-wide read across every Pantheon deity,
with the concrete next actions ranked by urgency — works with zero AI.

## What it does

`insight` aggregates a single picture of your workstation across the deities —
Anubis (hygiene), Horus (ops/health), Thoth (memory), the router (collaboration),
and Ma'at/Ra — and prioritizes what to do next.

The recommendation is **100% rule-based and never requires a model.** If the
optional local Gemma backend happens to be installed, `insight` adds a
plain-language narration on top; its absence changes nothing about the signals or
the ranking. This is Pantheon's "operate without AI, include it if present"
contract.

## Usage

```sh
sirsi insight            # deterministic signals + next actions (narrated if Gemma is present)
sirsi insight --no-ai    # force the deterministic-only view (skip any Gemma narration)
sirsi insight --json     # structured output for tools and dashboards
```

### Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--no-ai` | `false` | Deterministic only — skip the optional Gemma narration even if it's installed. The signals and ranking are identical with or without it. |

## Example output

```text
𓁢 Pantheon — State of the Union
  𓁟 🟢 Horus — Ops — all healthy
  𓁟 🟢 Anubis — Hygiene — 1.4 GB reclaimable
  𓁟 🟢 Thoth — Memory — memory current
  𓁟 🟢 Router — Collaboration — 27 open router item(s)
  𓆄 Everything healthy — nothing to do right now. (source: rules)
```

The `(source: rules)` tag confirms the result was deterministic. When Gemma
narrated it, the source reads `rules+gemma` and the signals are unchanged.

### JSON shape

```json
{
  "signals": [
    { "deity": "Anubis — Hygiene", "glyph": "🐺", "status": "1.4 GB reclaimable", "severity": 0 }
  ],
  "actions": null,
  "worst": 0,
  "source": "rules"
}
```

Each signal carries a `severity` (0 = healthy); `worst` is the highest severity
across all signals, and `source` is `rules` or `rules+gemma`. `actions` lists the
ranked next actions when there is something to do.

## When to use

- As your first command of a session — one read tells you whether anything needs
  attention across the whole platform.
- In scripts or a dashboard tick (`--json`) where you want a stable, model-free
  health-and-next-action feed.
- When you want a recommendation you can trust to be reproducible: `--no-ai`
  guarantees the deterministic path.

## Safety notes

- **Read-only.** `insight` inspects state and recommends; it never cleans, kills,
  or modifies anything. The next actions are suggestions you run yourself.
- **AI is strictly optional and local.** Any narration comes from the on-device
  Gemma backend, gated by an availability check; a failure there is non-fatal and
  the deterministic result still stands. Nothing is sent off the machine
  (Rule A11).

## Related

- **`sirsi diagnose`** — the deep 12-check health diagnostic behind Horus's
  signal.
- **`sirsi clean`** — act on Anubis's "reclaimable" signal
  (`docs/user-guides/clean.md`).
- **`sirsi router node-status`** — the operator view behind the router signal
  (`docs/user-guides/router-node-status.md`).
