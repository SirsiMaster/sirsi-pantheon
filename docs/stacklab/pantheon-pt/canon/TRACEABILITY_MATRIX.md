# Traceability matrix — Pantheon PT

| Requirement | Source | Verification/evidence | State |
|---|---|---|---|
| Token-fenced router completion | `internal/routerstore`, `internal/router`, `internal/mcp` | PR944 mainline CI: backend-aware tests and MCP route coverage | candidate-bound; release CI receipt required |
| Truthful lane dispatch and recovery | `internal/router`, `internal/routerboard`, `cmd/sirsi` | `router ping`, worker acknowledgement, ledger dispatch, and `router reopen` tests | candidate-bound; runtime receipt required |
| Lease ownership across remint and threadless calls | `internal/routerstore` | `TestIdentityItemOwnershipSurvivesSessionRemintSameAgentThread`, `TestCachedSessionSurvivesThreadlessInvocation` (both directions) | candidate-bound; runtime receipt required |
| One engine across supported surfaces | `cmd/sirsi`, `internal/*`, native sources | identity and package-inventory evidence | source-bound; runtime proof required |
| Project-scoped native workflows remain repairable | `macapp/Sources/SirsiMenubar/{Views,SirsiEngine}.swift` | Finder selection validates a Git root/worktree and preserves the current root on error | source-bound; native test verifies worktree admission |
| Bounded native I/O | runner and native output-bound tests | focused normal/race evidence | source lineage accepted; execution evidence separate |
| Python-free product path | package inventory and plugin wiring | copied-package inventory | open until independently run |
| Deterministic cask bytes | `internal/caskrelease/cask.go`, `cmd/sirsi/cask_release.go`, release workflow | exact renderer and remote-tap readback receipt | source-bound; lifecycle proof open |
| Development/commercial artifact separation | `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`, build scripts, Makefile | explicit mode and `-dev` filename contract | source-bound; package execution required |
| Signed/notarized release | release workflow | Developer ID/notary receipt | OPEN |
| Install/upgrade/rollback/uninstall | cask lifecycle plan | installed-host evidence | OPEN |
