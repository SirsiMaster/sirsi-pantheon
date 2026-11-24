# Completion policy: production source candidate

Classification: platform-foundation. This change is source preparation only;
independent review, admitted execution and normal distribution remain required.

## Authority and data flow

`validator/agent_completion_gate.py` is newly authored production protocol
source, not the retained entry-fixture helper normalized into authority. Go
embeds these exact bytes and invokes `/usr/bin/python3 -I -S -c <source>`.
There is no external helper selection, reopen, legacy-mode fallback or import
from `entryfixture`. `SIRSI_COMPLETION_GATE_SCRIPT`, `SIRSI_ROUTER_REPO`, HOME,
PATH and the API's retained canonical-router-root argument do not select policy.
This delegates algorithm custody to the shipped Go artifact; a source hash
identifies bytes but does not by itself approve a release or installed binary.

Close resolves the invoking Git worktree using fixed `/usr/bin/git`, removes
Git environment redirects and global/system configuration, and requires that
root to equal the nearest physical `.git` marker boundary above the actual
invoking directory. A marker must be a directory or regular gitfile; a symlink
or special marker refuses without falling through to an ancestor. An upward or
sibling `core.worktree` redirect cannot substitute another root. Nested callers,
linked worktrees and relocated
installations need no username-specific source root. Explicit `Enforce` callers
still own independent root resolution. The contract is repo-owned policy input,
not a trusted executable. No contract command is ever executed by the helper.

The proof is a contained, bounded (4 MiB), regular-file descriptor read with
no-follow traversal. Completion consumes only that captured stdin object and
the exact item ID. Proof classification must exactly equal the repo contract's
classification (the same value used by `init-proof`, with no per-proof override).
It requires completed/unblocked evidence, all closure and
working-status sections, required canon coverage, and every contract command
name exactly once with its exact command and passing result. Extra passing
checks are allowed. Duplicate JSON keys, duplicate command identities and
nonfinite constants refuse. The helper validates declarations, not receipt
authenticity, actual tests, customer outcomes or whole-lane completion.

CLI/MCP actor resolution, session claim ownership and durable completion lease
fences are untouched. Existing `--ack`/`--blocked` result semantics are unchanged;
they are not proof of implementation completion. Contract-free bare completion
also retains its existing behavior. Validator output is discarded by Go, with
fixed bounded errors, a 30-second deadline and 2-second pipe wait. No descendant
cleanup guarantee is implied; configured Python/Git/system-library provenance,
ancestor rename, concurrent in-place proof writes, and a transactionally stable
contract/canon/proof snapshot remain outside this repair.

## Standalone distribution mechanism (not performed)

`sirsi router completion-validator` prints the embedded source SHA-256.
`sirsi router completion-validator export <absent-file>` explicitly writes the
same bytes with mode 0600. Existing files, symlink parents/leaves and absent
parents refuse. Export neither chooses a tools directory nor installs anything.
Use physical parent paths (for example `/private/tmp`, not its `/tmp` alias).
An I/O failure may leave a partial new file and reports failure; it never
removes a possibly replaced destination. The exported file is not auto-trusted
by close. An authorized distributor may separately place reviewed exported
bytes at its approved tools path; that release/custody operation is not done.

Standalone commands retain `check-contract`, `init-proof`, `validate` (including
draft/blocked evidence and optional latest-proof discovery), and
`validate-completion --expected-item-id <exact-id>` with proof on stdin.
Completion never opens `--proof`; that argument is metadata. Diagnostic
`validate --proof-stdin` uses capture too. `init-proof` creates an absent proof,
never overwrites one, and refuses path-traversing item IDs. Its scaffold is
intentionally incomplete and must not be treated as verified evidence.

## Qualification commands and evidence boundaries

After fresh placement/admission, targeted commands are:

```
go test ./internal/completionproof/... -count=1
go test ./cmd/sirsi -run '^(TestCompletionValidatorExplicitExportCommand|TestRouterCloseCompletionAuthorization)$' -count=1
go test ./internal/routermcp -run '^TestHandleCloseCompletionAuthorization$' -count=1
```

The package suite exercises identical exported/embedded bytes and outcomes,
all standalone modes, every required-command identity, exact item ID, capture
with opposite pathname replacement in both directions, fixed Git/Python versus
forged PATH/root/helpers, Python startup controls, no-follow capture and bounded
proof/output/deadline behavior. The entry fixtures exercise actual CLI/MCP
status transitions, refusals with unchanged rows/claims, and nonselection of a
forged external helper; they no longer inject a Python capture wrapper. Capture
causality is covered separately by the package tests, not falsely inferred from
an entry-fixture receipt. All fixtures are synthetic and owned temporary state.

The pre-existing contradictory `cmd/sirsi/completionproof_test.go` expectation
was replaced by a causal caller-helper nonselection control: a private success
script would mark its execution and accept invalid `{}` evidence under the old
selector; embedded policy must reject that evidence without executing it.
No full-suite success is claimed. Historical item 152, historical
helper custody, configured interpreter/library provenance and actual standalone
distribution are separate obligations. Nothing here installs the helper or
certifies a production completion.

The standalone `init-proof` scaffold deliberately contains blank evidence and
does not validate until filled; diagnostic draft/blocked support does not make
an untouched scaffold valid. Contract roles are repo-owned extensible labels;
the five `READ_REQUIRED` roles select mandatory reads, while other roles still
require their declared files to exist. This change does not introduce a closed
role vocabulary or infer relevance of a document from its filename.

Owner-readable mirrors for this internal source note are not yet synchronized;
record their actual destination and source commit before claiming three-home
publication. This is an explicit publication remainder, not an implied sync.
