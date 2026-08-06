# Permanent execution

Pantheon decides whether an agent lane has work from three separate durable
sources: open router messages, actionable ledger tasks, and unmet canonical
requirements. A lane may park only when all three are empty.

Canonical requirements record their source document and anchor, assigned lane,
linked ledger task, lifecycle status, and completion evidence. `verified` is a
strong terminal state: it requires implementation, test, deployment, and
production evidence. `waived` requires an explicit waiver reference. This keeps
a passing test or merged pull request from being mistaken for whole-product
completion.

The first runtime slice stores this data in the local router SQLite database and
computes one `Runnable` predicate. Later supervisor slices consume that predicate
for event-driven wake, lease recovery, and Horus lane classification. The current
15-minute Codex heartbeat remains a compatibility bridge until autonomous wake
and acknowledgment evidence are deployed.
