# v0.24.164 forward route; v163 abandoned as a delivery route

Recorded by Ra from SSA's decision
`20261109-070443-sirsi-software-admin-ra-065909-full-release-decision-one-qualified164-v163-route-aba`,
which answered Ra's `20261109-065909` source review. This is a documentation
record only. It does not dispatch a workflow, create a tag or draft, upload an
asset, or claim that any signer, artifact or repair is operational.

## Decision

1. **One qualified v0.24.164 publication.** The owner approved exactly this
   forward version and this artifact set: app DMG, app PKG, the required
   pretoolguard DMG, the matching cask, and their verification receipts. There
   is no npm or other package, no copied key, no second publisher, and no blind
   retry.
2. **v163 is abandoned as the route for delivering the current repair.** Its
   history is retained:
   - the immutable tag `v0.24.163` (target `a5967913`);
   - the empty draft `432359892` (zero assets);
   - the failed runs;
   - both claim refs.
   Do not dispatch `recover-v163-after-consumed.yml` or reuse a consumed claim.
   The workflow stays in the repository as a historical, never-dispatched
   mechanism; retiring it would be a separate reviewed change.
3. **Old obligations stay open.** This disposition does not assert that every
   historical controller, signing-job or secret-custody issue is reconciled.
   Those recorded obligations remain open and may still prohibit an unsafe new
   job (see [v163-exact-draft-recovery-disposition.md](v163-exact-draft-recovery-disposition.md)).
4. **The carrying commit is not pinned yet.** It is pinned after integration,
   independent review and exact protected checks. The single-publisher rule and
   exact signing-claim/job custody hold before any new release attempt is
   consumed. No earlier release-train command is authorized for immediate
   dispatch by this record.
5. **SSA executes the release.** SSA, the sole release executor, owns
   preparation, signing and publication. Installation on M1, M5 and Studio is a
   later hand-off, only after publication, exact artifact verification, explicit
   host scopes and Ra's own fresh admission.

## Observed state when this was written (2026-11-10)

| Release | State | Target | Assets |
|---|---|---|---|
| v0.24.162 | **Published** 2026-11-03T01:44:10Z | `b298daac4c5fa3fb34fa0f454d3e906f96fcea04` | `SirsiPantheon-0.24.162-arm64.dmg`, `SirsiPantheon-0.24.162-arm64.pkg` |
| v0.24.163 | Draft, abandoned as a delivery route | `a5967913` | none |
| v0.24.164 | **Unreleased.** No release record exists. | — | — |

Read with `gh release view`. A per-tag read is not proof that no claim exists
elsewhere. Do not present v0.24.164 as published or installed, or the repair as
operational, until SSA's publication and artifact verification say so.
