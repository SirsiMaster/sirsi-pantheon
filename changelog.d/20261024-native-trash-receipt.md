Correct the existing native Empty Trash flow: bounded read-only pages, explicit
selected batches, retained nondeleting containers, and actual Swift-encoded
64KiB input limits matching the unchanged shared runner. Preflight only the
complete selected scope, then perform reviewed-only nonrecursive deletion with
truthful completed/review-required/partial/failed receipts. Keep existing
platform APIs unchanged. Swift freezes explicit consent, executes once through
runCaptured, never paints failed/incomplete capture green, and retains the
operation receipt across a failed read-only remaining-items review. Large folders
can be browsed for descendant selection instead of hitting a whole-Trash cap.
Each next batch requires fresh explicit review and confirmation; no --yes fallback.

Browse v3 retains one public Darwin directory stream in a serialized non-main
I/O actor: at most32 rows/34 readdir calls and34KiB name bytes per page, actual
work reporting, UUID/sequence/generation fencing and owned close lifecycle.
Blocked rows do not poison siblings or Next. No cross-open cookie, EOF rescan,
daemon, token registry, private symbol or extra child runner. b966's real cursor
qualification failed; its immutable evidence is retained and stateless CLI
browsing explicitly refuses. Go select/apply remain sole deletion authority.
Slow native I/O cannot publish after cancellation/dismissal; close stays pending
until acknowledged rather than claiming a stalled kernel call was interrupted.

Correct the invalid-name fixture after actual 070 qualification showed APFS
rejects creating a 0xff leaf. The production raw-name decoder now has a pure
inert-byte fixture boundary; real filesystem fixtures cover five supported
created names, while malformed raw bytes must still yield blocked metadata.
No invalid kernel-created leaf or current-child passing qualification is claimed.

Source-only corrective checkpoint; all new tests/build/race and installed UX
qualification are unrun. No real Trash, privilege, model or service action was
performed. See docs/stacklab/pantheon-pt/canon/TRASH_RECEIPT.md for cooperative-owner
limits, fixture selectors and independent qualification handoff.
