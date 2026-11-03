# Guard release recovery (operator custody)

This procedure is for a stopped or ambiguous *existing* release attempt. It is
not authority to start a second publisher, alter signer jobs, install a hook, or
repeat a signing request. Preserve all prior receipts, job directories, draft
assets, tags and failure logs. Do not remove or replace release assets.

1. Establish the exact tag, peeled commit, GitHub run and attempt and the
   retained reservation receipt's release ID; verify the
   original publisher and its native executor are terminal. Coordinate the
   existing sole release custodian and obtain a fresh host-specific safe window
   and own admission for any build, mount, signing or publication work. A
   missing receipt, an idle process snapshot, or a floor grant does not prove
   isolation. If a publisher is active, stop here and hand off to its custodian.
2. Reconcile each attempted signer request by exact UUID with the enrolled
   signer custodian: input digest, host/client identity, queue/work/outbox state,
   terminal receipt and any returned output. Preserve unknown or unrelated
   entries. An unknown result is not a failed result and must not be resubmitted.
   The runner's per-job intent is created before remote staging; its state is
   deliberately `enqueue-unknown`, not evidence of enqueue or termination.
   A directory input has no single-file digest in that intent. The attempt
   artifact retains intents when upload runs, but a lost runner or failed upload
   can leave that evidence unavailable; recover from the exact existing runner
   and signer custodian rather than assuming no job. Successful outbox evidence
   remains on the signer for reconciliation. A client timeout or lost lease
   can leave the independent signer running after the client exits: report this
   uncertainty to the host/window owners and do not claim safe handback until
   the exact job is reconciled. These receipts are not a host-wide scheduler fence.
   Reconcile each attach/detach by retained plist, device identity and actual
   host observation; do not attach a second image while any outcome is unknown.
3. If the initial attempt has a complete run-scoped custody artifact and its
   immediately preceding publication receipt proves guard detach completed, a
   normal rerun restores the exact app archive, DMG, PKG, image and manifest and
   does not sign again. The publisher checks the tag, signatures, staples,
   image contents, draft identity and every existing asset's bytes before
   uploading a missing asset; published releases are read-only. If any of these
   checks fails, stop and retain its new receipt.
4. If the reservation outcome is unknown or the receipt or custody artifact is missing, automatic rerun deliberately
   refuses. The operator must use retained run logs, the initial attempt's
   terminal step outcomes, exact signer job custody and mount evidence to decide
   whether signing and publication definitely never started. Record the evidence
   and independent review. Use authenticated exact-ID or complete bounded
   same-tag release discovery to identify an ambiguous draft; do not recreate
   an uncertain draft or select arbitrarily among duplicates. If any outcome remains unknown, resolve it with the
   existing custodian; neither absence of an output nor a timeout is proof that
   nothing ran. If signing definitely never began, the sole custodian can perform
   a new qualified first-attempt build/sign/package sequence under a fresh own
   admission, retaining a complete new exact-input custody set before publishing.
   Do not push the same tag to create a fresh workflow run: the cross-run guard
   refuses another run for an existing tag even if an earlier run failed.
5. If signing or partial publication already occurred, recover the **original**
   signed assets and manifest from their retained custody, not a rebuild. Verify
   source/tag identity, all hashes, signatures and staple, signer terminal
   receipts and detached mounts. Restore the set into an empty private directory
   using `scripts/release-custody.py restore`; then the sole qualified publisher
   can invoke `scripts/publish-release-assets.py` under a fresh host-appropriate
   admission and measurement-safe window, with the exact version, peeled
   commit, restored `--directory` and `--app`, `--include-guard`, the retained
   `--guard-manifest`, the independently reconciled exact `--reservation`
   receipt, and a new exclusive `--receipt`. That helper reads current
   draft/public state, refuses conflicting bytes and never clobbers assets.
   Record its receipt and separately verify the normal cask/publication outcome.

GitHub's run-scoped artifact service is the custody authority for artifacts
retrieved by the automated path; co-stored hashes are not an independent
attestation. A different custody source or missing proof requires explicit
independent reconciliation before use. Successful publication is not evidence
that the guard or installer has been installed or that a loaded GUI works.
