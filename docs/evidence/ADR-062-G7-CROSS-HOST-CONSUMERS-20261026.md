# ADR-062 G7: a codex lane and a headless Claude consumer, on different Macs, both close items (2026-10-26)

**Verdict: PASS.** Ra sent one item to each lane through the production router at 20:33:22–23Z. Each lane was
woken headlessly by its own worker loop; no human and no attended session was involved. Both closed their item
in the same minute.

| lane | surface | host | opened | acked | closed | result text (from the ledger) |
|---|---|---|---|---|---|---|
| `claude-io` | Claude, headless worker | M1 (`M1.local`) | 20:33:22Z | 20:34:25Z | **20:34:40Z** | `closed by claude-io on M1.local` |
| `codex-homebrew` | codex, headless worker | M5 (`Mac`) | 20:33:23Z | 20:34:34Z | **20:34:53Z** | `closed by codex-homebrew on Mac (worker thread thr-6888cab4e55f9b27; hostname observed with hostname)` |

Items: `20261026-203322-ra-claude-io-g7-probe-claude-io-m1-headless-ack-close-only` and
`20261026-203323-ra-codex-homebrew-g7-probe-codex-homebrew-m5-headless-ack-close-only`. Reread them with `sirsi router show <id>`.
At send time, the router classed both recipients as `WAKEABLE`: a worker loop with a working consumer.

## Why the hosts are swapped relative to the original wording

G7 was written as "a codex lane on the M1 and a Claude headless consumer on the M5". On 2026-10-26 the live
thread registry shows **no codex lane on the M1**: every live codex worker runs on the M5. So the run uses the
layout that exists, with the same property: one codex and one Claude headless consumer, on different Macs. Lanes
are picked by `answer_role=true`, the router's marker for a headless answering worker.

Three earlier probes were sent to the wrong kind of lane and withdrawn by Ra. They do not count toward the result:
- `codex-pantheon` is an attended session, not headless.
- `claude-pantheon`'s mailbox is aliased to `codex-pantheon`.
- `claude-home` on the M1 is `WATCH_ONLY` (a wake loop with no working consumer).

## Limits

- **Sample size:** one item per lane. G7 asks for proof that both close, not for throughput.
- **Layout:** if a codex lane returns to the M1, the original orientation can be rerun the same way: send an ack-only probe, then read `closed` from the ledger.
