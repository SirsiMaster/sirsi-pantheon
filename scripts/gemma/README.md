# scripts/gemma — the local-Gemma router worker

`sirsi-gemma-worker.sh` is the daemon behind the `gemma` router agent: it pulls
open `to: gemma` items through the router facade (`sirsi router pull gemma` —
never a file glob, so it survives the StoreWake cutover), completes them with
the on-device model via the `sirsi gemma` broker (ADR-031 RAM-gated, warm-server
first), routes the deliverable BACK to the requester as a fresh inbound item
(A26 relay), and closes the original.

This directory is the CANONICAL source (spec-fix Gap 2 — the worker previously
lived only at `~/.local/bin`, unversioned and outside CI). Deploy with:

    make install-gemma-worker

which copies it to `~/.local/bin/sirsi-gemma-worker.sh` and kickstarts the
`ai.sirsi.gemma-worker` LaunchAgent so the running daemon picks it up.

## Task modes

First line of the item body: `TASK: classify|summarize|draft|analyze|plan|extract|build`.

**`TASK: build` contract (canon):** gemma is text-in/text-out and resolves no
file paths. A build task MUST embed every source file it edits via
`--instructions @file` (or inline). The deliverable is complete file content
(`=== FILE: <path> ===` delimiters for multiple files); a referenced-but-not-
embedded file yields a trailing `MISSING-SOURCE: <path>` marker so the router
can re-scope. Gemma output is a DRAFT (A30 Tier 0) — a real agent reviews and
binds; gemma never merges, signs off, or runs tools.

## Model policy

Default model comes from `~/.sirsi/gemma-model.conf` (resolver-written);
`MODEL: max` in a body requests the 31B and is RAM-gated (falls back with a
note rather than Jetsam-killing a sibling). Owner law: the user-facing name is
"Ask Sirsi" — model identity stays internal.
