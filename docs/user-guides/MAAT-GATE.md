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
sirsi maat gate --base <base-commit> --head <head-commit>
sirsi maat gate --all --lint-only
sirsi maat gate --report
```

`--pre-push` consumes Git's pushed-ref records from stdin and checks every update
whose remote destination is a branch and whose local object ID is nonzero. This
includes a tag used as the source of a branch refspec. It fails closed when a new
branch has no resolvable merge base. Non-branch destinations and remote branch
deletions contain no branch content for this gate. Range checks inspect blobs at the
requested head commit, not uncommitted working-tree edits. Supplying only one
of `--base` and `--head` is an error; it never silently becomes a survey.

`--lint-only` is retained as a no-range, informational FinalWishes compatibility
mode. It runs only the source-pattern linter, not traceability, exemption-growth
or secrets checks, and must not be treated as a full gate pass. It cannot be
combined with explicit `--base`/`--head`; that combination is rejected before
checks run, including through the trust-boundary package API. The inspected
current FinalWishes pre-push hook calls default range mode; this flag remains
available for callers that explicitly require the no-range lint-only
compatibility behavior. `--all` makes that lint inspect the whole tree.

`--report` prints an optional heuristic review worksheet. It is read-only and
does not inspect the repository, run checks, or configure hooks. Each finding
names its heuristic letter. A reviewed exception may be noted with
`trust-boundary: <reason>` on the flagged line or its preceding line; doing so
suppresses the matching heuristic finding but does not establish policy
approval.

## Interpreting results

Each verifier reports `pass`, `fail`, or `skip`. A skip means the check was not
performed, not that it passed. For `--pre-push` when branch content is present
and for explicit `--base`/`--head` ranges, the CLI exits nonzero on a failed or
incomplete result, including any skipped check. Tag/deletion-only pre-push
input is a separate no-work success and does not claim a gate pass. In
machine-readable results, `ok` means no executed check failed; `complete` means
every check configured for the selected scope ran; `skipped` names checks that
did not run. A no-range survey may exit zero with `ok: true`, `complete: false`
and skipped checks, but is explicitly informational and not an enforcement
pass. No-range `--lint-only` can succeed when its selected lint completes, but
is informational and not a full gate pass. See the
[`trustboundary` package notes](../../internal/maat/trustboundary/README.md)
for check scope and known limitations. CI and the repository's adopted policy
remain authoritative.

This command has no hook-install mode. `--pre-push` only consumes ref records
when an already-configured caller explicitly invokes it.
