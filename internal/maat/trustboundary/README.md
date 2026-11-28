# Ma'at repository gate heuristics

`cmd/sirsi/maatgate.go` exposes `sirsi maat gate`; this package implements its
optional repository checks. These A–H source patterns were recovered from
historical gate work, but are not an accepted Pantheon law: ADR-076 is explicitly
unallocated on current `origin/main`. A clean lint is not a security
certification, and neither this package nor its report changes current canon.

## Behavior

- `Gate.Run` resolves explicit base/head revisions to commits and reads source
  blobs from the requested head, not the working tree. A partial explicit range
  fails; there is no fallback from a malformed range to a survey.
- `RangesFromPrePush` parses Git's four-field stdin format and returns every
  branch update with content. Tags/deletions carry no branch content; a new
  branch requires explicit `--new-branch-base` because local origin refs cannot
  safely identify the destination remote's default. Empty input is refused.
  `RunRanges` aggregates all branch refs, so one failed range fails the result.
- `--lint-only` intentionally skips traceability, exemption-growth and secrets
  checks. The flag is retained as an explicit FinalWishes compatibility mode;
  the inspected current FinalWishes hook invokes the default range mode. The
  lint-only mode must not be described as the full gate.
- `--report` only formats an optional heuristic worksheet. It does not locate
  or inspect a repository, configure hooks, run a verifier, or mutate state.
- Missing optional tools/files are reported as `skip`, not represented as
  verified; pre-push enforcement returns nonzero for incomplete ranges. A valid
  tag/deletion-only input is a no-op. Command time/output bounds and parsing
  failures fail closed.
- The secrets scan reads `.gitleaks.toml` and `.gitleaksignore` from the trusted
  base commit, verifies both are unchanged through candidate HEAD, and passes
  private temporary copies to Gitleaks. Mutable working-tree suppressions are
  not used; a policy-file change requires separate review.

## Checks and limitations

| Heuristic | Pattern family | Important limitation |
|---|---|---|
| A | Client-derived numeric values used as bounds | Intraprocedural and bounded propagation; not data-flow proof |
| B | Page/output emission inside loops | Syntax-pattern heuristic; not cost analysis |
| C | Request-shaped types carrying provenance-like fields | Field-name match only |
| D | Request handlers missing explicitly declared sibling gates | Only applies where a file declares `trust-boundary-gate:` |
| E | Merge writes to maps/documents | Does not prove persisted state semantics |
| F | Sensitive-looking fields in writes | Name-based; cannot identify unnamed PII |
| G | Digit stripping / silent meaning changes | Narrow patterns only |
| H | Masked verifier status, `--no-verify`, or depth caps | Does not prove algorithm completeness |

`trust-boundary: <reason>` on a flagged line or the line above suppresses its
finding. Treat that marker as a review note, not as automatic approval. The
fixtures test the detector's stated patterns; tests have not been run for this
candidate until a fresh Ma'at admission is granted.
