# PR939 independent source review

from: codex-pantheon
repo: /Users/thekryptodragon/Development/sirsi-pantheon
estimated_duration: 20 minutes
/plan: pin published candidate, read complete changed file and Resolve, run isolated normal/race controls, reconcile published disposition.
/goal: exact-source independent verdict; do not infer installed release acceptance.

PASS for the bounded source test change at a1ab030e9e089afe4e76b7fa1af3261c427d0bef. GitHub connector freshly confirms PR939 already merged06:17:22Z as6a1746291c00dbc79d0a10d02b57b69aa5aab571; no new bind performed.

Object-pinned reads: git show <head>:internal/routerstore/resolve_test.go, resolve_cutover_test.go and resolve.go. The new negative control clears URL, writes a fresh cutover marker, explicitly names canonical .sirsi/router.db, requires RemoteStore and checks no local db was created. Paired positive control names a genuinely different db and requires SQLiteStore plus file creation. Resolve already calls cutOverMarker, which distinguishes cleaned absolute canonical paths and self-heals from the service marker. Production behavior unchanged.

Isolated git archive of exact head under /tmp/pantheon-review-939:
- go test ./internal/routerstore/... -run TestResolve -count=1 PASS (0.351s)
- go test -race ./internal/routerstore/... -run TestResolve -count=1 PASS (4.393s)
Published exact-head CI, gitleaks and Binding Hold all success (runs36820832283,36820832359,36820832370).

Scope: this guards Resolve's explicit canonical-path branch. It does not prove every future native caller uses Resolve, qualify symlink aliases, or qualify installed UI/signature/runtime. Parent native-ra-fabric-guard-20260930 retains installed health proof and Ra acceptance. Origin fetch was sandbox denied (.git/FETCH_HEAD); published GitHub exact head/merge supplied current remote truth instead. Dirty shared checkout was not changed.

Task complete attempted with claimed lease and failed caller-session ownership. The review result is delivered separately; ledger must not be marked done via unfenced update.
