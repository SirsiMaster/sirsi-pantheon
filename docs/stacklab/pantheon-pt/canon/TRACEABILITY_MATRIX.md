# Traceability matrix — Pantheon PT

| Requirement | Source | Verification/evidence | State |
|---|---|---|---|
| Token-fenced router completion | `internal/routerstore`, `internal/router`, `internal/mcp` | PR944 mainline CI: backend-aware tests and MCP route coverage | candidate-bound; release CI receipt required |
| Truthful lane dispatch and recovery | `internal/router`, `internal/routerboard`, `cmd/sirsi` | `router ping`, worker acknowledgement, ledger dispatch, and `router reopen` tests | candidate-bound; runtime receipt required |
| Pressure sampling cannot indefinitely block dispatch or multiply cold-cache probes | `internal/router/backpressure.go`; `docs/ROUTER_HOST_PRESSURE_UNKNOWN_POLICY.md` | Separate three-second native command contexts; bounded shared flock; nine-process known/unknown/crash controls and lock-removal negative; independent aggregate native output/time observations | Composed retained source and focused race pass; M5/Studio samples pass; actual M1 sample and all loaded-consumer upgrade continuity remain open |
| Lease ownership across remint and threadless calls | `internal/routerstore` | `TestIdentityItemOwnershipSurvivesSessionRemintSameAgentThread`, `TestCachedSessionSurvivesThreadlessInvocation` (both directions) | candidate-bound; runtime receipt required |
| Bounded Apollo tool worker claims only its nominated item and may answer only with a declared, stamped lead grant | `internal/routerstore/lease.go`, `cmd/sirsi/gemma_worker.go`, `docs/stacklab/APOLLO_21_BOUNDED_TOOL_TURN_RECIPE.md` | `TestRemoteClaimItemSelectsOnlyItsNamedRecipientWork`, `TestGemmaWorkerExactItemNeverFallsBackToOlderInboxWork`, `TestGemmaWorkerApolloAnswerRoleIsDeclaredAndStamped` | source-bound; router-service rollout and fresh live qualification required |
| One engine across supported surfaces | `cmd/sirsi`, `internal/*`, native sources | identity and package-inventory evidence | source-bound; runtime proof required |
| Project-scoped native workflows remain repairable | `macapp/Sources/SirsiMenubar/{Views,SirsiEngine}.swift` | Finder selection validates a Git root/worktree and preserves the current root on error | source-bound; native test verifies worktree admission |
| Apollo/Mercury/Ra/Ma'at native workspace | `contracts/stacklab/pantheon-native-workspace-v1.json` | Swift workspace and Mercury tests; fixture captures; existing-monitor read | candidate built/tested locally; signed release and installed proof required |
| No stale activity or duplicate case action after navigation | `ObservationFreshness.swift`, `WorkspaceSession.swift`, `MaatOperationState.swift` | focused Swift expiry/cache/operation-lifetime tests and isolated keyboard/scroll interaction | candidate evidence only; installed runtime and signed release remain separate |
| Bounded native I/O | runner and native output-bound tests | focused normal/race evidence | source lineage accepted; execution evidence separate |
| Python-free product path | package inventory and plugin wiring | copied-package inventory | open until independently run |
| Deterministic cask bytes | `internal/caskrelease/cask.go`, `cmd/sirsi/cask_release.go`, release workflow | exact renderer and remote-tap readback receipt | source-bound; lifecycle proof open |
| Development/commercial artifact separation | `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`, build scripts, Makefile | explicit mode and `-dev` filename contract | source-bound; package execution required |
| Signed/notarized release | release workflow | Developer ID/notary receipt | OPEN |
| Install/upgrade/rollback/uninstall | cask lifecycle plan | installed-host evidence | OPEN |
