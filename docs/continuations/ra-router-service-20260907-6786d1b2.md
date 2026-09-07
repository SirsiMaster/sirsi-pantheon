<!-- agent: ra | workstream: router-service (ADR-062) | repo: /Users/sirsimasterdev/Development/sirsi-pantheon (M1) | date: 2026-09-07 | session: 6786d1b2-1c1f-4213-bd0b-f0f1d0d651fb | thread: thr-df2a8cd5b1c61290 -->

# Ra — router-service continuation (2026-09-07T17:2xZ) — TRANSFERRED TO THE M1

Supersedes `ra-router-service-20260903-2d18bc60.md`. Owner 2026-09-07: "transfer this session to m1". The M5 (Astra) belongs to SNE; Ra runs on the M1 from now on.

## Who you are, where you run
Router agent `ra`. Host: M1 (`sirsimasterdev@192.168.1.180`, M1 Pro 16 GiB, macOS 26.6.2). Start Claude Code from `$HOME` (`/Users/sirsimasterdev`) so `~/.claude/projects/-Users-sirsimasterdev/memory/` loads. PATH for non-login shells: `/opt/homebrew/bin:$HOME/.local/bin`.
- `sirsi` = `~/.local/bin/sirsi`, built from main 68568e9d on 2026-09-07 (`go build -o ~/.local/bin/sirsi ./cmd/sirsi`).
- Repo `~/Development/sirsi-pantheon` on branch `main` (was detached at 92ad980 before the move). `gh` is LOGGED IN on the M1 as SirsiMaster since 2026-09-07T20:xxZ (https protocol) — push and PRs work from here.
- Session transcript copied to `~/.claude/projects/-Users-sirsimasterdev/6786d1b2-1c1f-4213-bd0b-f0f1d0d651fb.jsonl` (try `claude --resume 6786d1b2-1c1f-4213-bd0b-f0f1d0d651fb` from `$HOME`; if the app refuses, resume from this file + memory).
- Memory rsynced from the M5 (`~/.claude/projects/-Users-thekryptodragon/memory/` → M1). Paths inside still say `/Users/thekryptodragon`; read them as the M5.

## The ledger still lives on the M5 (until rs-18)
The `ra` ledger and every router item are in the M5 store `~/.sirsi/router.db` (this is exactly what ADR-062 fixes; the service is not deployed yet). From the M1, relay every router/thread verb:
```
ssh thekryptodragon@192.168.1.155 'export PATH=/opt/homebrew/bin:$HOME/.local/bin:$PATH; sirsi router ledger ra'
```
Thread registration for `ra` must also be on the M5 store (reuse the id, never mint): `… sirsi thread register --agent ra --surface claude --thread thr-df2a8cd5b1c61290 --watch ra,claude-home --consumer-capable --workstream router-service`, then `sirsi thread heartbeat --thread thr-df2a8cd5b1c61290` relayed the same way. The M1's own `~/.sirsi/router.db` (Aug 29) is NOT the ledger.

## Where the work stands (verified 2026-09-07T17:2xZ)
| Phase | State | Evidence |
|---|---|---|
| A–C rs-01..13 | merged | #682–#687, #703 (e7af6a04) |
| D rs-14 | done | owner "1a 2a 3a" 2026-09-03 |
| D rs-15 provision | scaffolding MERGED — **PR #704 → 68568e9d** (SSA ACCEPT at head 556bf92e, item 20260907-153958; sirsi-bind App review 2026-09-07T17:03Z; binding-hold run 33808373927 green). Ledger row stays `in-progress`: provision.sh has not run. | `scripts/router-service/{provision,deploy,grant-provisioner}.sh`, Dockerfile |
| D rs-16..20, E rs-21..25 | pending | blocked on rs-15 |

### rs-15 — the one thing Ra cannot do itself
Owner runs `scripts/router-service/grant-provisioner.sh` as sirsimaster@gmail.com (grants the provisioning roles to `claude-agent@sirsi-nexus-live`). On the M1: gcloud installed via `brew install --cask google-cloud-sdk`; SA key at `~/.config/gcloud/sirsi-nexus-live-claude-agent.json` (mode 600). After the grant:
```
cd ~/Development/sirsi-pantheon
export CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=~/.config/gcloud/sirsi-nexus-live-claude-agent.json
DRY_RUN=1 bash scripts/router-service/provision.sh   # review, then run without DRY_RUN
bash scripts/router-service/deploy.sh                # rs-16
```
Probe first: `gcloud projects get-iam-policy sirsi-nexus-live --flatten=bindings --filter='bindings.members:claude-agent'` shows whether the grant landed. Region us-central1. cloud-sql-proxy 2.25.4 + psql 18.6 (`/opt/homebrew/opt/libpq/bin`) are installed on the M1.

**2026-09-07T20:3xZ probe (session 84ab1eaa, M1):** grant NOT landed — `sql instances list` denied, `secrets list` / `services list` / `iam service-accounts list` OK (pre-existing roles). `DRY_RUN=1 provision.sh` died at step 4 (read a secret the dry run had only echoed) — fixed by `secret_get` (dry-run placeholder); dry run now exits 0, negative control exits 1.

## Housekeeping
M5 main checkout still on foreign dirty branch `fix/broker-quarantine` — never work there. `worktrees/ra-rs01` (branch rs13-evidence) droppable. Owner item `20260902-211336-…-pr-678-blocked-signed-release…` still open. Charter ADR-063 (174dc7c8) in force: complete = verified release at rs-25; tokens are not progress.
