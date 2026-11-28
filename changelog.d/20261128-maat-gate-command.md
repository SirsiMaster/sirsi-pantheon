Restores the `sirsi maat gate` CLI and its repository source-pattern package on
current main (FinalWishes `.githooks/pre-push` calls it). The A–H checks are optional heuristics; ADR-076 remains
unallocated. The command has no hook-install mode, and JSON exposes skipped
checks separately from completeness. Pre-push fails closed on empty input,
incomplete checks, unreviewed secret-policy changes, or a new branch without an
explicit ancestor base. Gitleaks receives committed policy snapshots; its
heuristic worksheet JSON has no gate verdict fields. Explicit
`--lint-only --base <base> --head <head>` reports a distinct `lint-only-range`
scope and inspects only changed blobs at the selected head; missing endpoints
remain a refusal, never a working-tree fallback. Bare `--lint-only` now refuses
unless paired with `--all` or both range endpoints. Git gate subprocesses ignore
system/global configuration but continue honoring repository-local Git config.
An explicit `--base`/`--head` run now exits nonzero when any step was skipped,
as `--pre-push` already did; only `--lint-only` may pass incomplete. A bare
unranged `sirsi maat gate` is refused instead of exiting 0. The unused
hook-arming code is removed. The H pipe heuristic no longer flags a
display-only `tool --version | head` or a verifier word from a different command
on the same line.
