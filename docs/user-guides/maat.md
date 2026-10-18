# Ma'at — Quality Gate

Ma'at is the sole authority on code quality across the Pantheon. She runs governance audits, enforces policies, measures coverage, and provides quality scores.

## Commands

### Full governance audit
```bash
sirsi maat audit              # Full assessment with streaming progress
sirsi maat audit --skip-test  # Skip test execution (faster)
```

Checks: code coverage, formatting (gofmt), static analysis (go vet), canon compliance, and dependency health. Streams per-package progress so you see what's happening.

### Policy enforcement
```bash
sirsi maat scales             # Detect infrastructure policy drifts
sirsi maat scales --fix       # Auto-fix detected drifts
```

Enforces YAML-defined policies for code standards, dependency versions, and project structure.

### Autonomous healing
```bash
sirsi maat heal               # Auto-remediate governance failures
sirsi maat heal --fix --full  # Full remediation pass
```

Delegates to Isis for actual fixes: formatting, linting, vetting, and coverage gaps.

### Dynamic measurement
```bash
sirsi maat pulse              # Real-time metrics snapshot
sirsi maat pulse --json       # JSON output to .pantheon/metrics.json
```

Captures current quality metrics for dashboards and CI integration.

### Decision ledger — live and drillable
```bash
sirsi maat decisions                        # recent decisions, newest first
sirsi maat decisions --kind "reservation grant" --since 24h
sirsi maat decisions show <id>               # full record: what, who, why, evidence
```

Every grant/refuse, conflict check, guard verdict, window-gate block, and CI
pause/resume Ma'at makes is recorded to `~/.sirsi/maat/decisions.jsonl` (one
host's file — see `internal/maat/decision/README.md` for the cross-host
follow-up). `sirsi maat reserve|release|conflict-check` write here natively.

### GitHub submission attribution (Phase 1)
```bash
sirsi maat submit --kind release --repo OWNER/REPO --ref v1.2.3
sirsi maat submit --kind merge --repo OWNER/REPO --ref <sha> --json
```

Every Sirsi agent pushes to GitHub as the one SirsiMaster account, so GitHub
itself can't tell which lane tagged, released, or merged something. `maat
submit` records who actually did it: the requester is derived from your
registered session marker (`sirsi thread register`) and checked against the
declared agent registry — never a flag or a self-declared name. Phase 1
enforces a hardcoded per-repo allowlist (currently: only `mercury` may submit
for `sirsi-mercury`/`sirsi-photon`); repos with no policy defined are admitted
with attribution. Every grant or refusal is written to the decision ledger
above. Exits `97` on refusal (same convention as `reserve`/`conflict-check`).

This is registry-declaration eligibility (the same boundary
`dispatch.ValidateAgent` enforces elsewhere), not live-session cryptographic
authentication — a compromised local account can still forge its own marker
file. Phase 1 is attribution, not a security perimeter. It does not watch
GitHub and does not mutate any tag, release, or PR; that's an explicitly
separate, independently-reviewed later phase.

## Pre-Push Gate

Ma'at runs automatically on every `git push` via the pre-push hook:

```
𓆄 Ma'at pre-push gate... [Pantheon | depth: fast]
```

Three depth tiers:
- **fast** (default): gofmt + go vet + build + tests on changed packages (~10-30s)
- **standard**: fast + Ma'at coverage + canon check (`MAAT_DEPTH=standard git push`)
- **deep**: full test suite + full Ma'at assessment (`MAAT_DEPTH=deep git push`)

## Feather Weight Score

Ma'at scores projects on a 0-100 scale. A module scoring below 85 is considered "not yet canon" and cannot be included in a stable release.

## Output
```bash
sirsi maat audit --json       # JSON audit report
sirsi maat pulse --json       # JSON metrics snapshot
```


## Reservation concurrency and upgrades

Reservation changes use an atomic comparison at the shared router database.
Concurrent changes are reread and rechecked before a grant is returned. Status
and holder reads calculate expired leases without writing an old snapshot back.
Recent and older records remain available; admission no longer prunes history.
Generated reservation IDs include a random suffix, so simultaneous work by the
same holder receives distinct tickets. Explicit duplicate IDs are rejected.

A conflicting database snapshot may be retried up to 32 times. A transport
failure is returned immediately: the write may already have committed. Inspect
that ticket in the canonical ledger before retrying; never interpret an error
or missing ticket as permission to launch work.

The release must deploy the server first, then clients. All serving instances
must run the CAS implementation and reject legacy writes to `maat:reservations`
before upgraded clients resume admission. Route traffic fully to the new
revision and drain old revisions; mixed server revisions do not provide the
fence. Old clients receive an upgrade error for mutations. New clients pointed
at an old server fail because the method is absent. Neither case falls back to
unfenced writes or a second ledger. Old clients must be upgraded before normal
admitted work resumes; existing work keeps its normal TTL and may lose its lease
if its old heartbeat client cannot update it, so schedule a controlled window
and drain active work before cutover. No grace period extends leases.

Keep a canonical snapshot before cutover. Independently prove interleaved
reserve, heartbeat, release and expiry reads from enrolled clients on two hosts,
retaining exact server/client hashes and every reservation ID through cleanup.
A source test pass alone does not establish deployed acceptance. Rollback must
keep the server fence; restoring an unfenced server reintroduces lost updates.
