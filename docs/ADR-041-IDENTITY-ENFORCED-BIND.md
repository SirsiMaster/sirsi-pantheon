# ADR-041 — Identity-Enforced Bind, Scoped To Authority-Model Paths

**Status:** Accepted (owner decision 2026-07-15 — "A, scoped as C")
**Deciders:** Cylton (owner), claude-home
**Related:** PANTHEON_RULES A25/A28 (binding review), ADR-037 (completion-proof — every exercised capability ships as a deterministic lever), ADR-039 (honest-gate autonomy). Provenance: the 2026-07-14/15 self-merge incidents (#213–#216, #217).

---

## Context

Canon says **claude-home binds**. GitHub required **zero approvals**. Bind had no mechanical existence.

The root cause is identity, and it is verifiable in one call:

```
$ gh api user --jq '{login,type}'
{"login":"SirsiMaster","type":"User"}
```

**One account.** Every agent — claude-home, claude-pantheon, claude-finalwishes, all of them — pushes, reviews, and merges as `SirsiMaster`. GitHub forbids approving your own PR, so `required_pull_request_reviews` on `main` was `null`: it could not be switched on without the owner personally approving every agent PR, which ends autonomous operation. Canon described an authority the platform could not enforce, and nothing in the system could distinguish a bound PR from an unbound one.

Two incidents proved it in one night:

1. **#213–#216** self-merged via `gh pr merge --admin`. Root cause `enforce_admins=false` — `--admin` skipped *every* required check. Set to `true`, live, and correct; keep it.
2. **#217** — the PR written to *stop* authority PRs self-merging — **merged at `01:09:12Z` carrying its own `binding-hold` label**, 4 minutes after the label was applied. No admin bypass; `enforce_admins=true` was already on and did nothing, because the required check was genuinely green. Its auto-hold applied the label with `secrets.GITHUB_TOKEN`, and [GitHub suppresses workflow runs triggered by that token](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow) to prevent recursion — so the gate never re-ran and never saw the label.

The lesson generalizes past the token bug: **any marker an agent can apply, the author can apply**, because they are the same account. A label is an honesty marker. PR #218 made the *hold* real; it still cleared on a `bound` label its own author could add. False confidence is worse than no gate — everyone believes authority PRs are held.

## Decision

**Bind is enforced by a second identity, and only on authority-model paths.**

Option **A** (second identity) implemented at option **C**'s scope (authority paths only). The paths that decide who may merge what must not be gradeable by the identity that changes them; ordinary product PRs must not pay for that.

1. **The primitive.** GitHub structurally forbids self-approval. That is the only thing here an author cannot forge, so the bind is pinned to it: on authority-model paths, `binding-hold` requires an **APPROVED review by a login ≠ the PR author**.
2. **The second identity** is the `sirsi-bind` GitHub App (Pull requests: write; Contents/Metadata: read). It approves; it never merges, pushes, or edits protection. Agents keep merging as `SirsiMaster` with full autonomy.
3. **The key is local, and that placement is the enforcement.** `~/.sirsi/bind-app.pem`, `chmod 600`, on the conduit host — **never** in GitHub Secrets. A key reachable from a workflow lets any PR mint the token and approve itself, restoring the exact circularity this removes. This constraint is not a preference; deleting it deletes the ADR.
4. **App-token events re-trigger workflows** — unlike `secrets.GITHUB_TOKEN` events. That asymmetry is precisely why a review works where #217's bot label did not, and why `pull_request_review` is a trigger on the gate.
5. **The bind pins to the head SHA.** An approval on an older commit does not count; pushing after a bind drops it. This makes "merged == reviewed" structurally true rather than nominal — the same loss class that cost #207 its reviewed fixes (compounded by `delete_branch_on_merge=false` letting commits land on merged branches and vanish silently).
6. **Scope — authority-model paths only:** `.github/`, `cmd/sirsi/`, `internal/router/`, `PANTHEON_RULES.md`, `docs/ADR-*`. Everything else merges autonomously, unchanged. The explicit opt-in `binding-hold` label is untouched and still holds any PR on request.
7. **`bound` as a clearing label is abolished.** It cleared a gate its author could clear. Do not reintroduce it.

## Consequences

**Now enforced by GitHub rather than convention**
- An authority-model PR cannot merge without an independent identity's approving review on the current head SHA.
- `SirsiMaster` — i.e. every agent — cannot satisfy that on its own PR.
- The #213–#217 class (an agent clearing its own hold through the automation it wrote) is closed.

**Deliberately unchanged**
- Ordinary product PRs stay fully autonomous. That is the scope decision, not an oversight.
- `enforce_admins=true` stays. It is necessary and was never sufficient.

**Honest limits — stated so nobody rebuilds false confidence**
- Any process on the conduit host that can read the private key can bind. The separation is **credential placement, not proof of a human**. It does not survive a compromised host, and it is not a claim that a person reviewed the diff.
- Bind means "a second identity recorded approval at this SHA." It does not mean "correct."
- Setup is an owner action by necessity: creating an App and granting review rights are access-control changes agents do not perform — and an agent that could mint this credential unilaterally would defeat the separation it exists to create. Runbook: `docs/runbooks/bind-identity-setup.md`.

**Rejected**
- **B — accept honor-system bind and document it.** Cheapest and honest, but leaves a green check meaning less than it looks like on exactly the paths that define authority.
- **Unscoped A** (identity bind on every PR). Ends autonomy on ordinary product work to solve a problem that only exists on authority paths.

## Verification

The gate is proven by the PR that carries it: **#218 touches `.github/`, so it holds itself**, and cannot merge until the `sirsi-bind` identity approves it — the change validating itself under its own rule. A unit check (`scripts/bind/binding-hold-selection.test.sh`) pins the selection logic — it extracts the jq program from the workflow rather than copying it, so the test cannot pass against a rotted gate. Cases: author approval rejected, stale-SHA approval rejected, non-APPROVED review rejected, author noise alongside a real bind still binds, independent head-SHA approval accepted.
