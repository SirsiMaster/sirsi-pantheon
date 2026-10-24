# Commercial pair publication — existing release boundary correction

## Scope and owner

The operator needs one complete, signed commercial Pantheon update, never a
public assetless/partial release or silently replaced download. This corrects
the existing release-artifact recipe's publication transaction; it introduces
no product, signing authority, credential store, installer or release service.
SSA retains sole release custody. Source tests do not prove a real publication.

`commercial-sign-notary-publication-route` is implemented by the existing
release workflow/builders/enrolled signing client plus
`scripts/publish-release-assets.py`. The canonical Go cask renderer/verifier,
portable Go build, native payload inventory, signing modes, Apple notarization
and always-run login-keychain restoration remain unchanged. Governing signing
boundaries remain ADR-073/074.

## Transaction

1. The workflow's successful native commercial DMG and PKG build remains a
   prerequisite. Check the remote tag's peeled commit, not targetCommitish's
   mutable branch name. Hash both bounded regular local files. No leaf links or
   special files are accepted.
2. Reverify the already-built app/DMG using strict Apple Developer ID Application
   Team9D382WV988 requirements, PKG using Apple's trusted Installer chain for
   the same Team, and both staple tickets using the existing native validators.
   Retain these actual verification reports with exact final asset hashes.
   This does not replace or fabricate a notary submission acceptance receipt;
   the existing builders/signer retain submission/signing authority and records.
3. Create an absent release **as a draft**, requiring the existing tag. Auth,
   network or malformed lookup failure is unknown, not absence. An existing
   draft may resume only after every existing asset matches the reviewed bytes.
   Upload only missing members; never overwrite or delete an asset.
   A tag404 requires complete authenticated list discovery in twenty-record
   pages, still bounded to ten pages and1MiB per response. Historical release
   bodies count toward that unchanged byte limit. A full tenth page cannot
   prove absence; oversize, duplicate or incomplete discovery refuses before
   creation. Select a discovered draft only after an exact-ID readback; retain
   that ID for subsequent checks rather than discovering a replacement.
4. Require exactly one nonempty same-version arm64 DMG and PKG. Download each
   into a fresh helper-owned directory and compare size and SHA256 to the local
   verified files. Recheck local hashes and remote tag before publication.
5. Only then change draft=false. Read back the public state and exact asset
   bytes. Cask publication remains the existing dependent Go-controlled job and
   runs only after the native job succeeds.

Every GitHub command selects SirsiMaster/sirsi-pantheon explicitly. Same-tag
workflow runs serialize without cancelling an in-flight signing/publication
transaction. Temporary readback directories close/clean on success and error;
the existing always-run keychain cleanup remains independent of publication
or receipt-upload success. The workflow retains a uniquely named per-attempt
JSON verification receipt even on failure; no credential/environment dump.

## Retry and recovery

| Observed state | Allowed behavior |
| --- | --- |
| Absent | Stage a new private draft only after local verification. |
| Partial draft, existing bytes identical | Resume missing members without overwrite, then full paired readback. |
| Complete draft, identical bytes | Read back and publish; no asset upload. |
| Complete published record, identical bytes | Read-only verification success; no create/upload/edit. |
| Partial published record, duplicate/unexpected asset, mismatched bytes | Refuse. Preserve the record and evidence for explicit release-custodian recovery; no in-place repair. |
| Publish request/readback uncertain | Retain unknown publication state, not a claim that nothing became public. Recheck the same tag/pair before another attempt. |

Fresh signing/notary timestamps can legitimately produce different bytes on a
rebuild. That does not authorize replacing published assets: retain the original
verified artifact pair or use the normal successor release process. No force or
clobber escape hatch exists.

The helper assumes ordinary cooperative CI custody. Separate API calls are not
atomic against an independently privileged manual publisher or a malicious
same-UID process changing local files. Same-tag serialization, current-state
checks and before/after byte readback reduce accidental drift; no hostile-actor
transaction guarantee is claimed. Remote corruption after a final readback
requires a new observation. The helper does not mount/run downloaded payloads.

## Source qualification and rollback

`scripts/test_release_publication.py` uses only owned inert bytes and an
in-process fake gh/platform transport. A subprocess guard prevents accidental
real commands/network even if a fake call seam is omitted. Cases cover draft
order, fail/partial recovery, published idempotence/mismatch/partial ambiguity,
duplicate metadata, signature/Installer/staple/lookup/tag refusal, uncertain
publish transport and temporary readback cleanup.

The static commercial contract binds the helper and forbids overwrite flags.
Official packaging and publication Python invocations use explicit `python3 -B`
so the package metadata helper's import cannot add source-tree `__pycache__`
or `.pyc` files before canonical Go cask source-cleanliness checks. An owned
copied-helper/native-XML fixture runs the actual builder invocation with no
bytecode environment setting; no signing or packaging tool is executed.
Ma'at's existing source preflight fixture deliberately pins the successor
workflow bytes; static/source qualification is not installed signing or remote
publication proof. Main owns scheduled qualification; SSA owns actual release.

Rollback means reverting this source delta for a future reviewed workflow, not
automatically deleting a draft, touching a published pair, changing a tag, or
replaying a credential action. A failed draft remains private and recoverable.
