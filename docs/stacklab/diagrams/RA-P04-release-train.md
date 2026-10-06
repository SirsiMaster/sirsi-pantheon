# RA-P04 — Release train

Owner: Ra. Source: `scripts/release-train.sh`, `scripts/release-prep-changelog.py`. Revision: main `c1a78077`. Observed: v0.24.69 run on 2026-10-05.

## Logical view
```mermaid
flowchart TD
  A[release-train.sh version, optional deploy-service] --> B{Tag already exists?}
  B -- yes --> STOP1([STOP])
  B -- no --> C[Changelog prep on release/version from origin/main]
  C --> D[Push - confirm remote head equals local]
  D --> E[Open PR - wait for CI]
  E -- CI failed --> STOP2([STOP])
  E -- green --> F[Squash-merge - confirm MERGED]
  F --> G{deploy-service?}
  G -- yes --> H[gcloud run deploy sirsi-router from origin/main - require 100% traffic on new revision]
  G -- no --> I
  H --> I[Tag origin/main, push tag]
  I --> J[Wait for the Release workflow to finish green]
  J --> K[brew upgrade on the M1, check version]
  K --> L[brew upgrade on the M5 over SSH, check version]
  L --> M[Kickstart every wake loop on both Macs]
```
Every step is a hard stop. Service deploy precedes the tag so a new Store method exists server-side before clients ship.

## Data view
```mermaid
flowchart LR
  CL[CHANGELOG Unreleased] --> PREP[release-prep-changelog.py] --> PR[(release PR)]
  PR --> MAIN[(origin/main)]
  MAIN -->|source| RUN[(Cloud Run revision)]
  MAIN -->|tag| WF[Release workflow] --> CASK[(Homebrew cask)]
  CASK --> M1[M1 binary]
  CASK --> M5[M5 binary over SSH]
```
## Failure and recovery
- Tag collision (observed 2026-10-05: another line tagged v0.24.70-86): STOP, no force. Reconciliation is by merging the lines (PR #983), not by retagging.
- Combined push that fails lint no longer proceeds: each step verifies its own result.
- Rollback of a bad release: OPEN, not scripted.
