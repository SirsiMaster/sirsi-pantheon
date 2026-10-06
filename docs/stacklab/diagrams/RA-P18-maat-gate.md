# RA-P18 — Ma'at pre-push gate and CI

Owner: claude-pantheon. Source: `.githooks/pre-push`, `.githooks/gate-lock.sh`,
`.githooks/gate-advise.sh`, `internal/setup/maatgate.go`, `scripts/bind/sirsi-bind.sh`,
`internal/maat/knownfail/`, `.github/workflows/maat-advise.yml`.
Revision: main, post-#1011, pending #1015.

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
  J -- green --> L[SSA independent review on the exact head SHA]
  L -- CHANGES_REQUESTED --> K
  L -- APPROVE --> M[sirsi-bind.sh: query reviews on current head]
  M --> N{Any CHANGES_REQUESTED on this exact head with no clearing APPROVE/override?}
  N -- yes --> O[Bind refused, fails closed — A34] --> K
  N -- no --> P[Bind: merge] --> Q[Release train tags + deploys]
```
Local hook is advisory (`--no-verify` skips it, `MAAT_WINDOW_OVERRIDE=1` bypasses the
rails-lock wait); CI + the bind script are the backstop that cannot be skipped by a
local flag (A28).

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
  SSA[sirsi-software-admin] -->|APPROVE / CHANGES_REQUESTED| GH
  BIND[sirsi-bind.sh] -->|gh pr view --json reviews, current head SHA| GH
  BIND -->|merge or refuse| GH
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
  Approved --> Merged: bind succeeds (no live CHANGES_REQUESTED on this head)
  Approved --> BindRefused: a CHANGES_REQUESTED on this exact head, uncleared
  BindRefused --> AwaitingReview: new head or named owner override
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
- A34 is the hard rule at bind time: a `CHANGES_REQUESTED` review on the current
  head SHA is never superseded by a standing bind directive. It clears only via (a)
  a new head + a new review resolving the finding, or (b) `--override-pr
  <n> --override-finding "<text>"` naming the exact PR and finding. The binder
  fails closed (treats an API error as uncleared) and escalates to the owner.
- Unsure (flagged by Ra, unresolved here): no model-backed Ma'at gate exists yet —
  the catalog match is signature-only, never a model judgment; no catalog entry
  applies its own fix; whether `--no-verify`/`MAAT_WINDOW_OVERRIDE` should exist at
  all (owner has kept them as deliberate, log-visible escape hatches, not removed
  them).
