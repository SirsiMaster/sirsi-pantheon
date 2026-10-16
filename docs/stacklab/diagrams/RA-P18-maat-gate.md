# RA-P18 — Ma'at pre-push gate and CI

Owner: claude-pantheon. Source: `.githooks/pre-push`, `.githooks/gate-lock.sh`,
`.githooks/gate-advise.sh`, `internal/setup/maatgate.go`, `scripts/bind/sirsi-bind.sh`,
`scripts/bind/router-rejection.jq`, `internal/maat/knownfail/`,
`.github/workflows/maat-advise.yml`.
Revision: main @ `5027fd7e` (#1015, merged 2026-10-06) or any later commit — the
#1015 catalog-consult wiring and `gate_run`/`gate_advise` are present from this
commit forward; reverify against the exact head you are reading, not this pin,
if main has moved (codex-pantheon review of an earlier draft, router item
20261007-040907: a revision note is a point-in-time claim, not a standing one).

## Logical view
```mermaid
flowchart TD
  A[sirsi setup / install] --> B[ArmMaatGate: git config core.hooksPath .githooks]
  B --> C[git push]
  C --> D{Rails lock held and no MAAT_WINDOW_OVERRIDE?}
  D -- yes --> R[Refuse: wait or owner sets MAAT_WINDOW_OVERRIDE=1] --> C
  D -- no --> E{Tag-only or ref-deletion push?}
  E -- yes --> F[Fast pass, nothing to check]
  E -- no --> GL{Host-wide gate-lock free? .githooks/gate-lock.sh}
  GL -- held by another push --> GLW[Wait up to MAAT_GATE_LOCK_WAIT_SECS, reclaim if holder dead] --> GL
  GL -- free --> G[gate_run: gofmt + vet + lint + diff-scoped build/test, MAAT_DEPTH tier]
  G -- fail --> GA[gate_advise asks cmd/maat-advise: known signature -> cause+fix, unknown -> how to register] --> H[Push refused locally] --> C
  G -- pass --> I[Push succeeds] --> J[CI on the PR: same checks + canon guard + secrets scan]
  J -- red --> JA[maat-advise.yml: known signature -> cause+fix on run + PR comment, unknown -> how to register] --> K[PR blocked, author fixes, new head]
  J -- green --> L[SSA independent review]
  L -- CHANGES_REQUESTED --> K
  L -- APPROVE --> M{sirsi-bind.sh: any reviewer's MOST RECENT review on this PR — on any head, not just current — is CHANGES_REQUESTED? Also checks router-rejection.jq for a current router-channel rejection.}
  M -- yes, no override --> O[Bind refused, fails closed — A34] --> K
  M -- yes, named owner override --> OV[override-pr and override-finding flags recorded in the bind body] --> BINDREVIEW
  M -- no --> BINDREVIEW[sirsi-bind.sh posts an APPROVE review as the sirsi-bind bot, pinned to the exact head SHA]
  BINDREVIEW --> RERUN[Re-runs the binding-hold check run for that head so it re-reads the review and clears]
  RERUN --> BOUND[PR is bound: independently reviewed plus approving review recorded, NOT merged yet]
  BOUND --> MERGE[Separate step: an agent runs gh pr merge as the SirsiMaster identity. ADR-041 — sirsi-bind never merges, pushes, or edits branch protection]
  MERGE --> Q[Release train tags + deploys]
```
Local hook is advisory (`--no-verify` skips it, `MAAT_WINDOW_OVERRIDE=1` bypasses the
rails-lock wait); CI + the bind script are the backstop that cannot be skipped by a
local flag (A28). The bind and the merge are two distinct steps performed by two
distinct identities (`sirsi-bind[bot]` reviews; `SirsiMaster` merges) — conflating
them documents an enforced end-to-end path that does not exist (codex-pantheon
review, router item 20261007-040907).

## Data view
```mermaid
flowchart LR
  INSTALL[sirsi setup/install] -->|core.hooksPath| HOOK[.githooks/pre-push]
  HOOK -->|reads| LOCK[(rails.lock)]
  HOOK -->|queue| GATELOCK[(.githooks/gate-lock.sh: host-wide gate queue)]
  GATELOCK -->|changed pkgs| LOCALCHECK[gate_run: fmt/vet/lint/build/test]
  LOCALCHECK -->|failure log, gate_advise| CATALOG[(internal/maat/knownfail/catalog.json)]
  PUSH[git push] --> CI[GitHub Actions: Lint, Test, Build, Canon guard, Secrets scan]
  CI -->|status checks| GH[(GitHub PR)]
  CI -->|on failure, workflow_run| MAATADVISE[.github/workflows/maat-advise.yml]
  MAATADVISE -->|cmd/maat-advise| CATALOG
  MAATADVISE -->|cause+fix or how-to-register| GH
  SSA[sirsi-software-admin] -->|APPROVE / CHANGES_REQUESTED, any head| GH
  BIND[sirsi-bind.sh] -->|gh api pulls/reviews: every reviewer's latest verdict, any head| GH
  BIND -->|sirsi router dump + router-rejection.jq: current router-channel rejection| ROUTERSTORE[(router store)]
  BIND -->|posts pinned APPROVE/REQUEST_CHANGES as sirsi-bind bot, reruns binding-hold| GH
  MERGER[agent, SirsiMaster identity] -->|gh pr merge, only after bind recorded| GH
  GH --> TRAIN[release-train.sh] --> TAGS[(git tags)] --> INSTALLS[(installed sirsi binaries)]
  WAKE[Wake loop: lane session consumer output] -.unknown signature.-> CATALOG
  CATALOG -->|known match| ANSWER[cause + fix, at once]
  CATALOG -->|unknown signature| REGISTER[sirsi maat known-failures register]
```

## State view
```mermaid
stateDiagram-v2
  [*] --> HookArmed: setup/install ran ArmMaatGate
  [*] --> HookDisarmed: fresh clone, never armed
  HookDisarmed --> HookArmed: core.hooksPath set
  HookArmed --> LocalChecked: git push (not --no-verify)
  LocalChecked --> CIRunning: push accepted
  CIRunning --> CIGreen: all checks pass
  CIRunning --> CIRed: any check fails
  CIGreen --> AwaitingReview: PR open
  AwaitingReview --> ChangesRequested: SSA rejects
  AwaitingReview --> Approved: SSA approves
  ChangesRequested --> AwaitingReview: new head pushed
  Approved --> Bound: bind succeeds — sirsi-bind[bot] records APPROVE pinned to this head, binding-hold rerun clears
  Approved --> BindRefused: any reviewer's MOST RECENT review (any head) or the current router verdict is a rejection, uncleared
  BindRefused --> AwaitingReview: that reviewer submits a new review, or a named owner override (A34 clause b)
  Bound --> Merged: a separate `gh pr merge` by the SirsiMaster identity — bind and merge are distinct steps/identities
  Merged --> Released: release-train tags and deploys
```

## Failure and recovery
- Fresh clone with the hook never armed: pushes locally unchecked; CI is the only
  backstop until `sirsi setup`/installer runs (A28 — "armed, not just shipped").
- Rails-lock held during a measurement window: push refused unless the owner
  explicitly overrides with `MAAT_WINDOW_OVERRIDE=1` (deliberate bypass, not a bug).
- The pre-push hook and CI now consult the known-failure catalog on every failing
  gate step (#1015), before anyone — human or model — spends time or tokens on it.
  `gate_run`/`gate_advise` (`.githooks/gate-advise.sh`) wraps each hook step; on
  failure it asks `cmd/maat-advise`. A known signature prints cause + fix at once;
  an unknown one prints how to record it with `sirsi maat known-failures register`.
  CI gets the same advice via `.github/workflows/maat-advise.yml`, which runs on
  `workflow_run` failure and posts cause+fix (or the how-to-register prompt) as a
  PR comment, using only catalog text — never the PR's own code. The wake loop
  (RA-P02) and `sirsi maat known-failures match` remain a second, independent
  consumer for lane-session output outside the hook/CI path. An unknown signature
  stays open until a PR ships a fix with a guard test (`resolve` requires
  `fixed_in` + an existing guard test) — see RA-P02.
- A34 is the hard rule at bind time: the binder groups every review on the PR by
  reviewer, takes each reviewer's MOST RECENT verdict (regardless of which head it
  was submitted against — new commits alone do not clear a prior rejection), and
  refuses APPROVE if any of those latest verdicts is `CHANGES_REQUESTED`. It
  separately checks `sirsi router dump` + `router-rejection.jq` for a current
  router-channel rejection of the same PR (PR #927 merged past exactly this gap on
  2026-10-01). Either kind clears only via (a) that same reviewer/channel issuing a
  newer resolving verdict, or (b) `--override-pr <n> --override-finding "<text>"`
  naming the exact PR and finding. The binder fails closed (treats an API error as
  uncleared) and escalates to the owner.
- Unsure (flagged by Ra, unresolved here): no model-backed Ma'at gate exists yet —
  the catalog match is signature-only, never a model judgment; no catalog entry
  applies its own fix; whether `--no-verify`/`MAAT_WINDOW_OVERRIDE` should exist at
  all (owner has kept them as deliberate, log-visible escape hatches, not removed
  them).
