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

## Cumulative source-contract followup, 2026-10-25

Main's actual combined candidate73face source verifier failed under native
8ce53d9108204332a4d35474c6c95971: the commercial source contract still required
retired `gh release create` option strings although accepted4010 executes an
explicit POST and rechecks the remote tag itself. This is a verifier integration
defect, not permission to bypass validation or add dead marker strings.

SSA owns a minimal followup on preserved4010: replace only the retired option
requirements with the real explicit POST/private-draft/exact tag and commit/
projected response/validated retained identity source invariants. Keep the
unchanged publish-last, paired readback, tag, local identity, Apple trust and
no-clobber checks. Add inert controls that execute the actual source verifier
against isolated source fixtures: canonical success and mutations of each new
requirement must respectively pass and fail. Supplied publisher source is never
executed by those controls; no GitHub/Apple/build/service or credential mutation.
Retain the old-verifier red and final green logs, rerun the35 publisher fixtures,
then submit this exact additive donor for review and Main's full136 composition.

Actual qualification: native60ff568edc7e4c13b30d4d5d2db3182a retained the
old-verifier red in `/private/tmp/ssa-publisher136-contract.PC5CgbAr/`: canonical
publisher rejected because the guard required retired `--draft`. The exact same
two-method/eight-mutation fixture then passed with the corrected guard under
native375db3bd107142ee89af92b5bb81a8c7, alongside all35 unchanged publisher
fixtures, actual cumulative source verifier, Bash syntax and diff check. Raw
green `/private/tmp/ssa-publisher136-contract.QfKnBgx7/`. Source fixtures create
and automatically remove private temporary link trees; publisher execution and
network/Apple commands are absent. This does not add live API or integrated136
runtime credit. Publisher and existing publisher-test bytes remain identical4010.
