# Traceability matrix — Pantheon PT

| Requirement | Source | Verification/evidence | State |
|---|---|---|---|
| One engine across supported surfaces | `cmd/sirsi`, `internal/*`, native sources | identity and package-inventory evidence | source-bound; runtime proof required |
| Bounded native I/O | runner and native output-bound tests | focused normal/race evidence | source lineage accepted; execution evidence separate |
| Python-free product path | package inventory and plugin wiring | copied-package inventory | open until independently run |
| Deterministic cask bytes | `internal/caskrelease/cask.go`, `cmd/sirsi/cask_release.go`, release workflow | exact renderer and remote-tap readback receipt | source-bound; lifecycle proof open |
| Development/commercial artifact separation | `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`, build scripts, Makefile | explicit mode and `-dev` filename contract | source-bound; package execution required |
| Signed/notarized release | release workflow | Developer ID/notary receipt | OPEN |
| Install/upgrade/rollback/uninstall | cask lifecycle plan | installed-host evidence | OPEN |
