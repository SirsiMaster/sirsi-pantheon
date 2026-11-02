# Ma'at Casebook

Ma'at Casebook is Pantheon's local System One view for operational reasoning.
It turns the node's recorded Ma'at decisions into searchable, classified cases
with explicit actors, resources, and evidence links. It is a native Pantheon
feature; it does not depend on JEV or another external service.

The source of authority remains the append-only Ma'at decision journal. The
casebook is read-only: it never grants a reservation, changes a determination,
or starts work. That boundary lets operators inspect why work is queued,
refused, released, or still open without creating a parallel policy engine.

## Source lifecycle evidence

The Casebook includes an informational lifecycle control for an existing
structured canonical receipt. Preview with an exact receipt ID, repository,
and independently selected full head before recording an observation:

```sh
sirsi maat lifecycle --receipt-id ID --expect-repo REPO --expect-head FULLSHA --preview --json
```

Only an explicit confirmation with the previewed receipt digest retains the
observation. Owner adoption is a separate preview/confirm flow for an exact
review and originating evaluation. Router receipt custody and `From` are not
authenticated reviewer identity. Lifecycle observations do not qualify a
model or enable a gate; absent installed pins remain unqualified and the
deterministic host checks remain independent.

## Use it

```sh
sirsi maat casebook
sirsi maat casebook m5 --status open
sirsi maat casebook --kind allocation --json
```

The Horus dashboard's **Ma'at** view shows the same projection at
`/api/maat/casebook`. Cases include an evidence graph that links each recorded
decision to its requester, affected agent, resource, and evidence reference.

## Review a managed service recovery

For a System One `launchd Disabled Override` case, choose **Preview exact
managed targets** before opening confirmation. The preview lists each eligible
managed label, its LaunchAgent plist digest, and whether the operation would
enable it only or enable and bootstrap it. Review those exact labels in the
confirmation sheet. Ma'at refuses to act if the preview binding, plist, or
eligible target set changed; the action does not accept caller-supplied labels
or launchctl commands. It rechecks the same diagnostic and records the outcome.
The same preview and confirmation flow is available from both native System
One finding entry paths.

The read-only CLI preview is also available directly:

```sh
sirsi maat repair launchd-disabled --json
```

Apply requires the `binding` from that preview:

```sh
sirsi maat repair launchd-disabled --preview-binding <binding> --confirm --json
```

The repair remains limited to eligible managed LaunchAgents and preserves the
existing quarantine rules. A loaded target is enabled without bootstrap; an
unloaded target is enabled and bootstrapped. A refusal requires a fresh preview
and review.

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
