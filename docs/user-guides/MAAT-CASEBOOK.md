# Ma'at Casebook

Ma'at Casebook is Pantheon's local System One view for operational reasoning.
It turns the node's recorded Ma'at decisions into searchable, classified cases
with explicit actors, resources, and evidence links. It is a native Pantheon
feature; it does not depend on JEV or another external service.

The source of authority remains the append-only Ma'at decision journal. The
casebook is read-only: it never grants a reservation, changes a determination,
or starts work. That boundary lets operators inspect why work is queued,
refused, released, or still open without creating a parallel policy engine.

## Use it

```sh
sirsi maat casebook
sirsi maat casebook m5 --status open
sirsi maat casebook --kind allocation --json
```

The Horus dashboard's **Ma'at** view shows the same projection at
`/api/maat/casebook`. Cases include an evidence graph that links each recorded
decision to its requester, affected agent, resource, and evidence reference.

## Classification

The casebook derives a descriptive category from the recorded `kind` field:

- reservation and cede records become `allocation`;
- conflict records become `contention`;
- guard, window, and CI records become `governance`;
- all remaining records become `assessment`.

Failed, blocked, refused, and declined determinations are marked `urgent`.
Pending, queued, and counter determinations are marked `high`. The projection
does not alter the original determination; it makes its operational consequence
visible for a human or a local Pantheon client.
