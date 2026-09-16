# PT-WING-001 — Stack Lab G1 Intake Submission

**Submitted:** 2026-09-11
**Product:** Sirsi Pantheon
**State:** `source-candidate` / `intake`
**Authority:** current local Pantheon process as Horus direct client and Ra subclient; remote worker authority remains authenticated and receipt-bound.

## Exact intake objects

| Object | Path | SHA-256 |
|---|---|---|
| Catalog | `docs/apple-stack-lab/pantheon-pt-wing-001.catalog.json` | `37af24e6d27622a0137edad1da5687894f8e0924e472b85548a369dea7126c93` |
| PT-R01 | `docs/apple-stack-lab/pantheon-pt-r01-route-receipt.recipe.json` | `26da6b65c6222c4ff186ff9f47a84cb887657856c7619c8a7916d2eac78a2860` |
| PT-R02 | `docs/apple-stack-lab/pantheon-pt-r02-sne-native-v2.recipe.json` | `f38c0b36a0b4b26449bd3b03d4ec037c95bc2d41739339e6bf66eab829f917ca` |
| PT-R03 | `docs/apple-stack-lab/pantheon-pt-r03-qualification.recipe.json` | `42169c4800e2babc009bca1a69dcfeb3fbb5c8964938ee1a84534e8ac9f9e306` |
| PT-R04 | `docs/apple-stack-lab/pantheon-pt-r04-worker-plane.recipe.json` | `2a536ba4cea760ec821b154ca38f15bfbc4a3d0eef73f76d00928d73df54dfd6` |
| PT-R05 | `docs/apple-stack-lab/pantheon-pt-r05-package-inventory.recipe.json` | `bce2435b546fc493368bf9d2991a4e62cb96c73c58ae1898f79d23a7bd757638` |
| PT-R06 | `docs/apple-stack-lab/pantheon-pt-r06-release-boundary.recipe.json` | `3ecb67afdb4c43b85dfe91673e3bf48cab26148f9d9fc71799b29be92a80822f` |
| Wing request | `docs/apple-stack-lab/PANTHEON_WING_REQUEST.md` | `9a697c2083d4d3baaed09ff4c594428cff0675d636ff3df885fbaa960fa989ad` |

The catalog source/storage paths are regular non-symlink files. The recipes are deliberately non-executing intake documents; they request no model start, package install, service mutation, signing, notarization, or release promotion.

## Environment contract

Pantheon does not treat M1, M5, or any other physical device as canonical. It records the current Mac's capabilities and environment profile, then binds route, authority, and receipts to that session. The SNE lane has now replaced physical-host authority with `ROLE-RECEIPT-001.G1`, defined by `/Users/thekryptodragon/Development/sirsi-inference/docs/engineering/stacklab/HOST_NEUTRAL_ROLE_RECEIPT_CONTRACT.md` (SHA-256 `0f248299b8b75595544d0d5deef2b7993e71bb609196db6c3e307998272b90c8`). The prior wing index remains historical context; M1/M5 are host-profile labels only.

## Static boundary

`jq` parsing, source-path regular/non-symlink checks, SHA-256 recomputation, and `git diff --check` passed for this intake. No Go/Swift tests, builds, package assembly, cask operation, runtime/service action, host qualification, signing, notarization, or install action was performed. G1 asks only for metadata validation; all later gates require their own authority and exact receipts.

**Next review:** independent exact-object review of the catalog, recipes, and role-receipt binding. **Next implementation gate:** bind an independently reviewed, unexpired role receipt for the current operation; no router migration, host mutation, service start, or execution is authorized by this intake.
