Restores the `sirsi maat gate` CLI and its repository source-pattern package as
an uncommitted candidate. The A–H checks are optional heuristics; ADR-076 remains
unallocated. The command has no hook-install mode, and JSON exposes skipped
checks separately from completeness. Pre-push range selection follows the
remote branch destination (including tag-sourced branch refspecs), and hook
arming refuses a missing, non-executable, or non-regular hook. Enforcement
ranges fail closed on skipped or incomplete checks; a no-range survey remains
explicitly informational. The legacy Ma'at guide no longer claims automatic
hook execution or `MAAT_DEPTH` tiers.
Explicit ranges cannot be combined with the informational `--lint-only`
compatibility mode; both the CLI and package API refuse that pairing.
The review worksheet now prints a valid no-range `--all --lint-only` example
and clearly labels it informational, not a full enforcement pass. Focused
lint-engine tests now call the internal range-lint implementation directly
rather than selecting the rejected public range-lint-only mode; the symlink
hook fixture creates its parent directory. Report wording uses lowercase
"informational" to match the regression assertion.
