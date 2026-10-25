# Publishing one verified release pair

The release workflow publishes the existing signed/notarized DMG and PKG with
`scripts/publish-release-assets.py`. It does not build, sign or install them.
Supply the exact version, peeled source commit, artifact directory, signed app,
and a fresh create-only receipt path. The remote tag must already name the
reviewed source. Use the normal release workflow; do not run a second publisher
while another publication attempt is unresolved.

An existing matching draft is selected by its exact ID after complete bounded
discovery. If none exists, one authenticated API creation requests a private
draft with generated release notes. The returned HTTP201 ID/tag/draft fields
are validated and retained; every subsequent check uses that ID, not a new
walk through release-list pages.

Both remote artifacts must match the complete local signed bytes before the
draft becomes public. The published pair is downloaded and verified again.
The helper never clobbers or deletes an asset. An already-published retry is
read-only and refuses incomplete or different bytes.

## When creation or readback fails

Keep the exclusive receipt and the original signed files. `creation_attempted`
with `creation_outcome: unknown` is not proof of absence or permission for a
second create. A retained `release_id` identifies the selected record even if
its later GET fails. Reconcile the actual private release by exact ID and
observed state through the owning release operator before another action. Do
not delete, recreate, upload over an asset or re-sign to make a failed receipt
look successful. A failed public readback may follow successful publication;
`published: null` preserves that uncertainty.

## Maintainer verification

`python3 -B -m unittest discover -s scripts -p test_release_publication.py -v`
runs an in-process fake transport. Its guard forbids real subprocess, Apple
tool and network execution. Fixtures check moving post-create page boundaries,
creation response status/shape, selected-ID continuity, unknown outcomes,
signatures, byte readbacks, ordering and temporary download cleanup. These
fixtures do not claim a real GitHub release has succeeded; the exact next
release must supply that observed proof.

The source plan and original135 failure are recorded in
`docs/stacklab/pantheon-pt/canon/PUBLISHER_CREATED_ID_CORRECTION_20261024.md`.
Classification remains platform foundation; no whole-product GA claim.
