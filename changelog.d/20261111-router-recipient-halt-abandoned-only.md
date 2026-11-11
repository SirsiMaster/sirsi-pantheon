- **Router recipient admission: the halt counts abandoned work, not planned backlog.**
  Owner decision 2026-11-11. The recipient's `stalled` count (JSON name unchanged)
  now means a task someone STARTED and then left: nonterminal, claimed at least once
  (`attempts > 0` or status `in-progress`), untouched for 24h, no live lease. A lane
  with a large never-started backlog (502 planned tasks) is no longer refused all
  messages. The unacknowledged-items half (warn at 50, halt at 99) is unchanged, and
  a lane that drops started work mid-flight is still halted. Sender warning and
  escalation text now say "started tasks untouched for 24h". Known blind spot:
  `reset-attempts` zeroes the only start evidence, so a reset task reads as planned.
  Refs: ADR-035 section 2b; owner directive 2026-11-02 amended 2026-11-11.
