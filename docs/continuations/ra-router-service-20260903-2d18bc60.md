<!-- agent: ra | workstream: router-service (ADR-062) | repo: /Users/thekryptodragon/Development/sirsi-pantheon | date: 2026-09-03 | session: 2d18bc60-9159-441f-863e-fef22686cbfe | thread: thr-df2a8cd5b1c61290 -->

# Ra — router-service continuation (2026-09-03T21:3xZ)

Supersedes `ra-router-service-20260903-89132fa4.md` (same content for Phases A–C; this file carries Phase D state).

## Who you are
Router agent `ra`, $HOME desktop session. NOT claude-deck (owner ruling 2026-09-03T21:05Z: the deck lane is session 0949e54b). Re-register at every session start if `sirsi thread list` has no `ra` row:
`sirsi thread register --agent ra --surface claude --watch ra,claude-home --consumer-capable --workstream router-service`
then heartbeat every few tool calls. Send `--from ra`. Ledger: `sirsi router ledger ra`.

## Where the work stands (live, verified 2026-09-03T21:3xZ)
| Phase | State | Evidence |
|---|---|---|
| A–C rs-01..13 | merged | #682–#687, #703 (e7af6a04) |
| D rs-14 | done | owner "1a 2a 3a" 2026-09-03T04:2xZ |
| D rs-15 provision | IN PROGRESS — PR #704 head a5b64112; **blocked on owner GCP login** | worktree `$SCRATCH/89132fa4…/scratchpad/wt-rs15`, branch rs15-provision |
| D rs-16..20, E rs-21..25 | pending | blocked on rs-15 |

### PR #704 review state
- claude-home (item 20260903-061113) and SSA (item 20260903-200819, REJECT) both said "`router serve` does not exist on main". **False at this head and on main**: `cmd/sirsi/routerservecmd.go` registers `serve` in its own `init()` (line 57) — landed in #687. Receipt: PR-head binary run with the Dockerfile CMD → `router serve: listening on :18080`, `/healthz` 200, `/v1/call/Inbox` 401 without bearer. Both items closed `--ack` with that result. Narrow re-review sent to SSA 2026-09-03T21:3xZ ("does REJECT stand?"). Do NOT merge before SSA answers; when ACCEPT → App-bind + squash-merge, then `task update ra rs-15-provision` stays in-progress until provision.sh has actually run.
- Finding 2 (`--build-env-vars-file=/dev/null`) fixed in a5b64112. PR body rewritten with the receipts.
- Docker daemon is not running on this Mac; image build not reproduced locally. Linux static build of the head: 25.8 MB.

### rs-15 blocker (owner gate — the only thing between here and a running service)
`gcloud auth print-access-token --account=sirsimaster@gmail.com` → no token (also admin@sirsi.ai). `claude-agent@sirsi-nexus-live` SA can list Cloud Run but lacks sqladmin / IAM / secretmanager. Owner must run `gcloud auth login sirsimaster@gmail.com` (path 1, chosen 2026-09-03T04:2xZ) or grant the SA roles/cloudsql.admin + roles/secretmanager.admin + roles/iam.serviceAccountAdmin + roles/resourcemanager.projectIamAdmin + roles/run.admin + roles/serviceusage.serviceUsageAdmin + roles/compute.networkAdmin. Then: `cd wt-rs15 && CLOUDSDK_CORE_ACCOUNT=sirsimaster@gmail.com DRY_RUN=1 bash scripts/router-service/provision.sh`, review, run for real, then `bash scripts/router-service/deploy.sh` (rs-16).

## Housekeeping still open
`worktrees/ra-rs01` holds branch rs13-evidence (safe to drop); `.agents/idea-router/agents.json` `ra` entry uncommitted in the main checkout (which sits on someone else's branch `fix/broker-quarantine` with a dirty tree — do not touch it); owner item `20260902-211336-…-pr-678-blocked-signed-release…` still open; scratch worktree `main-build` in this session's scratchpad (detached origin/main, disposable).

## Durability rule
Update this file and `~/.claude/projects/-Users-thekryptodragon/memory/project_router_service_ra_gcp.md` at the end of every substantive turn.
