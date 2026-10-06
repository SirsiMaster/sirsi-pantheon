# RA-P05 — Review and bind (A34)

Owner: sirsi-software-admin (SSA) for review, Ra for router-code decisions. Source: `scripts/bind/sirsi-bind.sh`, `scripts/router/sirsi-claude-worker.sh`. Status: Ra's understanding, to be confirmed by SSA (sent 2026-10-05).

## Logical view
```mermaid
flowchart TD
  PR[Author opens PR with CI evidence] --> R[SSA independent review on the head SHA]
  R -- CHANGES_REQUESTED --> FX[Author fixes - new head] --> R
  R -- APPROVE --> B[sirsi-bind.sh records the approval]
  B --> K{Any CHANGES_REQUESTED on this head not followed by APPROVE?}
  K -- yes or API error --> REF([Refuse: fail closed, escalate to owner])
  K -- no --> MG[Merge] --> REL[Release train]
  PR -. router code .-> RA[Ra verifies, merges, deploys: no SSA review]
  R -. security or privacy .-> OWN[Owner decision card]
```
## Data view
```mermaid
flowchart LR
  GH[(GitHub reviews on head SHA)] -->|gh pr view --json reviews| BIND[sirsi-bind.sh]
  BIND -->|approval record| LEDGER[(Router ledger)]
  OVR[owner override: PR number and finding text] --> BIND
```
## Failure and recovery
- A directive that predates a rejection cannot clear it; only a new head with a new review, or a named owner override (`--override-pr`, `--override-finding`).
- GitHub API error: treated as uncleared.
- SHA (sirsi-hardware-admin) role in review: unmapped, OPEN.
