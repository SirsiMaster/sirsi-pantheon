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
