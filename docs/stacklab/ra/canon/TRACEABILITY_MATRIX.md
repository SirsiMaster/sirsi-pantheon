# Traceability matrix — Ra / Horus

| Requirement | Source | Evidence in this release | State |
|---|---|---|---|
| One production authority | `routerstore.Resolve`, `dispatch.Open` | explicit-local-fallback negative control | verified locally |
| Runnable predicate joins three sources | `internal/routerstore/runnable.go` | package tests | source-bound |
| Claims are fenced/idempotent | `tasklease.go`, `facade.go` | lease/expiry/duplicate tests | source-bound |
| One physical Studio capacity pool | shared scheduler alias comparisons and conservative capacity reads | both-direction/history/concurrent alias fixtures; ADR-118 | source reviewed; canonical service/native client readback pending |
| Wake transitions are durable | `wakeevents.go`, `reconcile.go` | event synthesis/terminal retry tests | source-bound |
| Upgrade reaches running controllers, not only installed files | `wakeimage*.go`, `RunWakeLoop`, `InstallWakeLaunchAgent` | official120 M5 all17 automatically adopted and M1 all9 kernel-current after natural busy boundary; retained-vnode/hash readback; alias-installer red-before-green | all3 signed App/root CLI120 installed; actual Ra M1/Codex Pantheon M5 sequential A/B; full-fabric and Studio queue qualification remain open |
| Successful bounded execution can clean up its own registration | `CloseThread`, `wakeidentity_test.go` | actual119 false-positive retained; official120 actual M1/M5/Studio bounded controls exit0 with closed executors/released reservations; real observer refusal retained | bounded native lifecycle verified; not model or all-lane workload proof |
| Observer remains natively owned after restart | `adoptWorkerThread`, `Heartbeat`, `CloseThread` | exact generation reuse, persisted readback, checked heartbeat, transient recovery and terminal/no-revival controls | source-bound accident guard; separate raw-service consumer identity not claimed |
| A2A identity/body/correlation/errors | `dispatch`, `routerstore` | contract assessment and tests | source-bound |
| Read acknowledgement is distinct | `AckItem`, schema v22+ | recipient-only ack tests | source-bound |
| Cloud canonical service | `router serve`, GCP recipe | historical deploy receipts; current readback pending permissions | open-live |
| Codex least-privilege delivery | `router relay`, spool contract | M1/M5 relay evidence | host-verified prior receipt |
| Restricted-worker native execution | `scripts/maat/fabric-run.sh`, exact-IP proxy profiles | all declared Codex profile guards, POSIX argv/refusal tests; scoped SSA M1/Studio success; external-domain denial | source/transport verified; each lane still needs its own actual destination receipts |
| M1/M5 fleet parity | relay/client/service | current fresh cross-host proof | partial |
| Fresh third-machine join | token/env/register/claim/close | hermetic rehearsal script | rehearsal pending |
| Horus/CLI/MCP same read model | `routerboard`, `router-mcp`, Horus | source + package tests | source-bound |
| Preserve active consumers during release | `scripts/release-train.sh`, `scripts/release-consumer-preservation.test.sh`, `docs/stacklab/RELEASE_CONSUMER_PRESERVATION.md` | exact former-source RED/current GREEN under3ec1e715; negative controls run hosted CI | source tested; per-host loaded-image adoption still independently required |
| Rollback/revocation | deploy/cutover scripts | rollback fixture; fresh cloud rehearsal pending | partial |
| Commercial product closure | commercialization gate | pilot/platform-foundation classification | open |
| Scoped cloud logical host survives ephemeral OS hostnames | `routerstore.ThreadOwnerHost`, `NewRemoteStore`, thread registration/save | exact120 baseline403, corrected registration/heartbeat/claim/completion/close and foreign-host/lane refusal; native two-package race PASS | client candidate only; private environment readiness and M5-off proof remain open |
