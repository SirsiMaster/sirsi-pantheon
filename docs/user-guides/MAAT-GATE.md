# `sirsi maat gate`

The Ma'at repository gate runs repository traceability, exemption-growth,
secrets, and optional A–H source-pattern checks. The A–H checks are historical
heuristics, not an adopted canon or a proof of security. Current Pantheon
`origin/main` leaves ADR-076 unallocated; this command does not restore that
proposal or change any repository's hook configuration. It does not install or
arm a pre-push hook; hook integration must be configured and reviewed separately.

## Commands

```sh
sirsi maat gate --pre-push < /dev/stdin
sirsi maat gate --pre-push --new-branch-base <trusted-base-commit> < /dev/stdin
sirsi maat gate --base <base-commit> --head <head-commit>
sirsi maat gate --all --lint-only
sirsi maat gate --base <base-commit> --head <head-commit> --lint-only
sirsi maat gate --report
```

`--pre-push` consumes Git's pushed-ref records from stdin and checks every branch update
with content. Empty/whitespace-only input is refused. A new branch requires an explicit
`--new-branch-base`; the destination's default branch cannot be inferred safely from
local `origin/*` refs. Tags and deletions contain no branch content. Range checks inspect
blobs at the requested head commit, not uncommitted working-tree edits. Gitleaks uses
the committed base policy/ignore files and refuses a range that changes them; it never
discovers mutable working-tree suppressions. Any skipped verifier (no traceability
script, no exemptions file, gitleaks not installed) makes a `--pre-push` or
`--base`/`--head` run incomplete and therefore nonzero. Supplying only one of `--base`
and `--head` is an error; it never silently becomes a survey. A bare
`sirsi maat gate` with no range, `--pre-push`, `--lint-only` or `--report` is refused:
it could enforce nothing, and exit 0 would read as a pass.

`--lint-only` is retained as an explicit FinalWishes compatibility mode. It runs
only the source-pattern linter, not traceability, exemption-growth or secrets
checks, and must not be treated as a full gate pass. It requires either
`--all` or both `--base` and `--head`; a bare `--lint-only` is refused rather
than falling back to a working-tree survey. The inspected current
FinalWishes pre-push hook calls default range mode; this flag is available for
callers that explicitly require lint-only behavior. `--all` makes that lint
inspect the whole tree; otherwise it checks changed files in the exact range.
Git subprocesses suppress system and global Git configuration, while continuing
to honor the target repository's local Git configuration.

`--report` prints an optional heuristic review worksheet. It is read-only and
does not inspect the repository, run checks, or configure hooks. Each finding
names its heuristic letter. A reviewed exception may be noted with
`trust-boundary: <reason>` on the flagged line or its preceding line; doing so
suppresses the matching heuristic finding but does not establish policy
approval. With `--json`, report output has `scope` and `report` only; it does not
carry gate `ok` or `complete` fields.

## Interpreting results

Each verifier reports `pass`, `fail`, or `skip`. A skip means the check was not
performed, not that it passed. The JSON form keeps the same nonzero process exit
status on a failed result. See the
[`trustboundary` package notes](../../internal/maat/trustboundary/README.md)
for check scope and known limitations. CI and the repository's adopted policy
remain authoritative.

This command has no hook-install mode. `--pre-push` only consumes ref records
when an already-configured caller explicitly invokes it.
