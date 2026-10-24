# Publisher created-draft identity correction

Owner: SSA, coordinated with Pantheon Main; estimated review delivery within one hour.
Task: `pantheon135-created-draft-identity-cure-20261024`.
Classification: `platform-foundation`. Baseline is immutable released
v0.24.135, commit `b0e17c427abaa6d2b8330cba222c3ca0c92ef064`, tree
`db4ab85a0ab770a3335a0def053e7fc307b1d9e2`. This is a donor for the next
normal release, not an edit to published135 or permission to republish its assets.

## Goal and evidence

Actual135 native `4a5668d496e943f8a482bfc7a7493b95` created one empty private
draft423038958, then failed `ambiguous release discovery metadata` before any
upload/publication. Exact-ID recovery `bfc15b6851a140589f269a8093f29c5b` used
that same draft and signed bytes successfully. Original raw discovery pages were
not retained, so their particular duplicate/type anomaly is unknown; this plan
does not claim GitHub pagination corruption or that a synthetic fixture explains
every service behavior.

The implementation must retain the identity returned by the single creation
operation, then perform complete selected-ID verification. Never discover a newly
created draft again through mutable pagination. Existing-draft lookup and full
bounded pre-create absence proof remain unchanged. A creation whose response is
unknown must never authorize another create or an upload.

## Plan

1. Show the released reader fails a deterministic post-create moving-page
   regression while its complete pre-create absence proof still runs.
2. Use the existing authenticated `gh api` create endpoint with explicit private
   draft, exact tag/commit/title and generated notes. Retain a bounded projected
   response ID/tag/draft from that operation, validate it, and bind all later
   reads to the selected ID. Mark unknown creation outcomes truthfully.
3. Cover malformed/mismatched/public/missing creation identities, HTTP or
   transport failures, selected-ID drift/disappearance, unchanged existing-draft
   discovery, signatures and private-byte-before-publication ordering.
4. Run the complete inert publisher fixture suite under native admission; no
   Apple tools, GitHub mutations, signing or installs from the fixtures. Show
   the causal regression fails on exact old source and passes on corrected
   source. Preserve raw logs and every original live failure.
5. Publish the exact donor, obtain independent review, register reusable source
   and evidence in Stack Lab, and integrate it into Main's single next release.
   Real API acceptance is proved only by that later authorized actual release.

## Commercialization Gate

- Buyer/user: Mac operators and release maintainers installing Pantheon.
- Pain: a successful draft creation followed by rediscovery failure requires
  manual recovery and delays a tested signed release.
- Workflow: build one signed pair, stage privately, verify full remote bytes,
  publish last, and verify public bytes.
- Value: lowers operator recovery risk; no separate paid feature or pricing
  validation is claimed.
- Trust boundary: existing GitHub release credentials and exact repository/tag;
  no new token, IAM, security setting, asset overwrite/delete or service action.
- Operational owner: SSA release publisher, Pantheon implementation owner, SHA
  for later supported installed-host qualification.
- Done evidence: old-source negative control, all inert tests, exact source
  review/catalogue, then actual next-release private/public readback. Source
  acceptance alone does not prove deployed publication or all-host completion.

Confidence: high in retaining the creation response as the causal correction;
the precise malformed/duplicate live listing record remains unknown.
Existing release/version and Stack Lab authority contracts continue to apply.
API contract: https://docs.github.com/en/rest/releases/releases#create-a-release
(`draft`, `target_commitish`, `generate_release_notes`, and returned release ID).
