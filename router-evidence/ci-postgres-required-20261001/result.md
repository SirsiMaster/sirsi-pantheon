author: codex-pantheon
addressed_to: claude-pantheon
repo: /Users/thekryptodragon/Development/sirsi-pantheon
estimated_duration: 30 minutes

/plan: publish fail-closed prerequisite correction, provision PostgreSQL explicitly in CI, independently review, and run the actual PostgreSQL suite.
/goal: required CI cannot be green when its PostgreSQL leg did not execute.

Verified current GitHub main scripts/ci-postgres.sh blob 8956b83df5abaf0169520801b1dba5288cc7a793: missing initdb/pg_ctl/psql exits 0. Current CI workflow blob c89a89fb8d5a470de82efc8d172550f2bd8b7520 runs Test on macos-14 and invokes that script without prerequisite provisioning. This confirms the source defect independently; historical job log was not independently reread here.

Prepared candidate: router-evidence/ci-postgres-required-20261001/ci-postgres.sh. It emits ::error:: and exits 1 for a missing tool. bash -n passes. PATH=/usr/bin:/bin bash candidate prints the initdb error and returns 1 before cluster creation. This is a negative control, not a PostgreSQL suite execution receipt.

Publication work remains: apply candidate to scripts/ci-postgres.sh, correct workflow's skips-loudly comment, explicitly provision PostgreSQL and add its bin directory to GITHUB_PATH, add a missing-tool negative control, and add required changelog/traceability. Independently review and capture a real PostgreSQL green receipt. Governed by A35 and ADR-062 rs-07b; do not present mere fail-closed behavior as dual-backend acceptance.

Managed lane boundaries: git fetch failed writing .git/FETCH_HEAD (Operation not permitted); gh API network failed; registration failed inspecting /bin/ps (operation not permitted). Router task ci-postgres-required-20261001 claimed under inherited thread thr-b9302c79c6136524, lease b92dc308d553c671057035cb3021f498. Existing pr944-cf94db73-review reclaim returned no claimable task, so its fenced completion has not been repaired.
