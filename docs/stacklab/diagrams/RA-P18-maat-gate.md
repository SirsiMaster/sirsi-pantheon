# RA-P18 — Ma'at pre-push gate and CI

Owner: claude-pantheon. Source: `.githooks/pre-push`, `internal/setup/maatgate.go`,
`scripts/bind/sirsi-bind.sh`, `internal/maat/knownfail/`. Revision: main `fd645827`.

## Logical view
```mermaid
flowchart TD
  A[sirsi setup / install] --> B[ArmMaatGate: git config core.hooksPath .githooks]
  B --> C[git push]
  C --> D{Rails lock held and no MAAT_WINDOW_OVERRIDE?}
  D -- yes --> R[Refuse: wait or owner sets MAAT_WINDOW_OVERRIDE=1] --> C
  D -- no --> E{Tag-only or ref-deletion push?}
  E -- yes --> F[Fast pass, nothing to check]
  E -- no --> G[gofmt + vet + lint + diff-scoped build/test, MAAT_DEPTH tier]
  G -- fail --> H[Push refused locally] --> C
  G -- pass --> I[Push succeeds] --> J[CI on the PR: same checks + canon guard + secrets scan]
  J -- red --> K[PR blocked, author fixes, new head]
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
  HOOK -->|changed pkgs| LOCALCHECK[fmt/vet/lint/test]
  PUSH[git push] --> CI[GitHub Actions: Lint, Test, Build, Canon guard, Secrets scan]
  CI -->|status checks| GH[(GitHub PR)]
  SSA[sirsi-software-admin] -->|APPROVE / CHANGES_REQUESTED| GH
  BIND[sirsi-bind.sh] -->|gh pr view --json reviews, current head SHA| GH
  BIND -->|merge or refuse| GH
  GH --> TRAIN[release-train.sh] --> TAGS[(git tags)] --> INSTALLS[(installed sirsi binaries)]
  LOCALCHECK -.on failure, unknown signature.-> CATALOG[(internal/maat/knownfail/catalog.json)]
  CATALOG -->|known match| ANSWER[Published answer to the lane]
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
- A known failure signature (local check or CI) is matched against
  `internal/maat/knownfail/catalog.json`; a match publishes the existing answer to
  the lane without a model call. An unknown signature is registered via
  `sirsi maat known-failures register` and stays open until a PR ships a fix with a
  guard test (`resolve` requires `fixed_in` + an existing guard test) — see RA-P02.
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
