# Trust-Boundary gate (`sirsi maat gate`)

Refuses a push that carries one of the eight trust-boundary defect classes
A–H, a failing Rule-29 verifier, or a self-admitted exemption. Canon:
[ADR-076](../ADR-076-TRUST-BOUNDARY-LAW.md) (the law, the checklist).

## In a pre-push hook (one line)

```sh
sirsi maat gate --pre-push || exit 1
```

Git feeds the pushed refs on stdin; the gate checks **every** branch ref being
pushed and fails if any fails. Tags and deletions carry nothing and pass. A new
branch is checked from its merge-base with `origin/main`; if no base can be
resolved the push is **refused** with a message — fetch origin or run the gate
with an explicit `--base`.

Arm the repo's hooks once (covers all its worktrees):

```sh
sirsi maat gate --install
```

## By hand

```sh
sirsi maat gate --base origin/main --head HEAD        # a range (what CI or a reviewer runs)
sirsi maat gate --all --lint-only                      # survey the working tree
sirsi maat gate --all --lint-only --base X --head Y    # survey the whole tree AT Y
sirsi maat gate --report                               # print the A–H checklist for your PR body
sirsi maat gate --base X --head Y --json               # JSON on stdout only; exit status is the verdict
```

## What you will see

```
  ✅ traceability         commit traceability verification passed: 1 post-boundary ...
  ✅ exemption-growth     94 exemptions, none added
  ⏭️  secrets              gitleaks not installed (brew install gitleaks) — CI's secrets scan still gates ...
  ❌ trust-boundary-lint  3 changed files at 5a07852f, 1 finding(s); silence a reviewed false positive with `trust-boundary: <reason>` ...
     api/internal/forms/pack.go:81 [A] make() sized by a parsed client number — loop/allocation bound derived from a client-supplied number; ...
  𓁵 𓆄 trust-boundary gate failed — do not --no-verify past this; fix or allowlist with a reason
```

Everything in a range is read from the **pushed commit**, not your working
tree: fixing a file without committing it does not make the push pass, and an
uncommitted experiment does not make it fail.

| Step | Fails when |
|---|---|
| `traceability` | the repo's `scripts/verify-commit-traceability.sh` **as committed at the base** exits non-zero, or the script itself changed in the range (a push may not attest itself — land verifier changes in their own reviewed PR) |
| `exemption-growth` | `scripts/traceability-historical-exemptions.txt` at the head contains a hash that is not in it at the base — even if another hash was removed |
| `secrets` | gitleaks finds a secret in the range (skipped with a warning if gitleaks is not installed) |
| `trust-boundary-lint` | any A–H finding in a changed file; a file that cannot be read or Go that does not parse |

## Fixing a finding

Fix the code, or — when a reviewer agrees it is a false positive — record why
on the line or the line above:

```go
// trust-boundary: n is clamped to serverMaxRows by validateLimit above
rows := make([]Row, n)
```

The letter in `[A]` names the rule; the exact heuristic behind each letter and
its limits are listed in
[`internal/maat/trustboundary/README.md`](../../internal/maat/trustboundary/README.md).
The lint is a tripwire, not a proof: the A–H checklist in your PR is still
answered item by item.
