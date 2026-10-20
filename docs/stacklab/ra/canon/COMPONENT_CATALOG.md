# Component catalog — Ra / Horus

| Component | Source surface | Boundary | Release evidence/state |
|---|---|---|---|
| Canonical resolver | `internal/routerstore/resolve.go` | URL/service or explicit test DB | one-authority fix; verified locally |
| Dispatch facade | `internal/dispatch` | all production reads/writes | package tests; source-bound |
| Registry | `internal/router/registry.go` | identity and adapter metadata | origin/service drift doctor |
| Independent civic control adapter | `docs/router-service/worker-workspaces/kfca/CLAUDE.md`, `claude-kfca` profile | neutral private coordination CWD distinct from declared civic repo; no product/civic context ingestion | source contract fixture; deployment and actual queue receipt pending |
| Service | `cmd/sirsi router serve`, `internal/routerstore/serve.go` | Cloud Run/Cloud SQL | deployed historically; fresh readback open |
| Remote client | `internal/routerstore/remote.go` | session/nonce/runtime auth | contract tests |
| Relay | `cmd/sirsi/routerrelaycmd.go` | host token holder; no DB copy | M1/M5 host receipt |
| Scoped native transport | `scripts/maat/fabric-run.sh`, registry Codex consumer profiles | exact enrolled IP proxy, pinned SSH, no bearer export; both ends supervised | literal-argv/refusal fixtures; actual scoped SSA M1/Studio terminals2311a0d6/d3f5f7aa; per-lane adoption remains tracked |
| Leases | `tasklease.go`, `items.go` | fenced claim/renew/close | adversarial tests |
| Studio capacity aliases | `internal/maat/schedule/resource.go`, shared atomic scheduler | bare legacy/canonical names share capacity; original IDs/spellings and host binding retained | independent source inspection/native race fixtures; actual deployment still required |
| Wake/informer substrate | `wake.go`, `wakeevents.go`, `horus` | bounded delivery, no blind spawn | shipped substrate; informer increment next |
| Provider quota retry hold | `providerquota.go`, `lanestate.go`, `RunWakeLoop`, `router provider-available` | anchored failed-consumer facts, host-local durable generation, bounded lock, no account or credential mutation | native focused/race controls; full CI, signed controller adoption and runtime suppression proof pending; recipe `docs/provider-quota-retry-hold.md` |
| Native observer generation | `wake.go`, `threads.go`, `adoptworker_test.go`, `wakeidentity_test.go` | exact persisted machine/PID/start; no predecessor adoption; heartbeat before dispatch | focused race controls, outage hold/recovery and terminal preservation; successor release/queue qualification pending |
| Installed executable adoption | `wakeimage*.go`, `InstallWakeLaunchAgent` | kernel-loaded mapping compared to the direct installed regular name; installer resolves existing aliases before persisting argv | official119 M5 all17 loaded-image readbacks and sequential queue receipts; M1 seven current, two busy consumers preserved; symlink installer regression red-before-green in successor |
| Bounded executor cleanup | `CloseThread`, `wakeidentity_test.go` | inherited lane wake mechanism is not proof that an executor is a resident observer; published observer state remains guarded | actual119 false-positive terminal a819ee7a preserved, dead executor closed through supported lifecycle; bounded executor negative-control red-before-green in successor, not yet deployed |
| A2A/MCP | `cmd/sirsi-router-mcp` | read/mutate façade over dispatch | package tests |
| Horus | `internal/horus`, dashboard/fleet | derived observation only | source-bound/live per host |
| Stack Lab wing | `docs/stacklab/ra/canon` | docs and receipts | this release |
| Release consumer preservation | `scripts/release-train.sh`, `scripts/release-consumer-preservation.test.sh` | no forced controller restarts or automatic measurement-window override; install is not loaded-image proof | exact former-source RED/current guard GREEN, admitted terminal3ec1e715; live adoption remains separate |
| Relay bounded discovery | `internal/routerstore/spool.go`, `spool_test.go` | every discovery pass waits existing cancellable poll; lane forwards remain independent | deterministic continuous-wake RED31e5a5e, repeated native race GREENdbfb0713; protected PostgreSQL follow-up pending |
| Direct task source-wake acknowledgement | `internal/routerstore/tasklease.go`, `tasklease_direct_wake_test.go`, `docs/stacklab/DIRECT_TASK_CLAIM_WAKE_ACK.md` | successful exact fenced claim ACKs only its matching pending/leased ledger source inside the same transaction; no terminal-history rewrite or grant change | native RACE3 PASS8.458s terminal4082d458; full/PG CI and deployment pending, old pending/DONE wake debt retained |
| Completed task wake exclusion | `wakeevents.go`, `reconcile.go`, `docs/stacklab/COMPLETED_TASK_WAKE_EXCLUSION.md` | proven exact DONE sources are nonexecuting; no fabricated ACK, expiry/failure mutation or continuation suppression | same-source Ra344cc4dc; native/full/PG qualification and deployment pending |
| Release/rollback | `scripts/release-train.sh`, recipe | immutable hash/readback | candidate contract |

Logical cloud identity is specified in [CLOUD_EXECUTOR_IDENTITY.md](CLOUD_EXECUTOR_IDENTITY.md).
It uses existing scoped host tokens, not OS hostname mutation or new authority.
Source/fixture qualification does not establish configured runtime readiness or
M5-off continuity; retain those separate operational gates in the matrix.

Owner: Ra/Horus Fabric lane. No component may create a second authority.
