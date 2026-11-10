# v0.24.163 exact-draft recovery disposition

> **Superseded as a delivery route (2026-11-09).** v163 is abandoned for
> delivering the current repair; one qualified v0.24.164 publication replaces
> it. Everything below is kept as history. Do not dispatch
> `recover-v163-after-consumed.yml`. See
> [v164-forward-route-decision.md](v164-forward-route-decision.md).

This is a narrow proposed recovery, not a claim that the original error was a
page shift or that GitHub release listings are atomic. Run 41592161041 attempt
1 failed at draft reservation before identity import and build/sign steps. Its
retained artifact 13210139655 records a5967913c68c03be435ab009f24df01e908681b3,
v0.24.163, release ID 432359892, `create_outcome_unknown`, and the error
`ambiguous post-create release discovery`. The exact historical reason for that
error is not recoverable from the retained receipt. Independent authenticated
readback subsequently found that exact draft, no assets, and one observed
same-tag listing entry. That does not prove a global atomic snapshot.

The owner authorized the established single publisher to continue using this
exact draft and original tag, without repeating either creation. The recovery
must independently reread the retained artifact by ID, the exact failed run and
skipped signing steps, tag peel, draft identity, assets, and bounded release
listing immediately before its create-only one-shot claim. Any actual conflict,
unknown outcome, interruption or mismatched identity stops the continuation;
the claim and all signer/custody evidence remain for reconciliation. The
reviewed controller commit and one recovery run must be bound by the workflow.

## Explicit one-release version exception

The tagged source's `VERSION` is **0.24.162**, not 0.24.163. The normal A13
release train updates `VERSION` and changelog before tagging; that did not
happen here. For this already-created immutable tag and existing draft only,
the owner-authorized recovery supplies `0.24.163` through the existing build
scripts' explicit version arguments. This is an exception, not a source bump or
evidence of normal release-train compliance. App, CLI, PKG and cask versions
must be independently verified from actual release artifacts; notes must not
claim the source itself contained a v163 version bump. A normal subsequent
main VERSION/changelog correction remains separate work.
