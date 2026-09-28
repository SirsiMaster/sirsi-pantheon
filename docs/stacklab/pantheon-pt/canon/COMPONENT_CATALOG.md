# Component catalog — Pantheon PT

| Component | Source surface | Write boundary | Evidence / next action |
|---|---|---|---|
| Go engine and CLI | `cmd/sirsi`, `internal/` | product commands | focused/full test receipts |
| Native surfaces | `macapp/`, menubar/TUI packages | bounded child processes | output/identity evidence |
| Native project selection | `macapp/Sources/SirsiMenubar/{Views,SirsiEngine}.swift` | explicit user-selected Git root; invalid selection preserves the prior root | Git/worktree admission test and native retry guidance |
| Package | `.goreleaser.yaml`, package inventory | create-only build roots | unsigned inventory evidence |
| Cask | `internal/caskrelease/`, `cmd/sirsi/cask_release.go`, release workflow | create-only remote-tap publication | canonical bytes, remote readback, and lifecycle proof |
| Release boundary | `.github/workflows/release.yml`, `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`, release scripts | explicit development/commercial artifact selection and remote release/tag authority | static mode contract, then signing/notary and remote receipts |

Owner: Pantheon PT lane. Release state: source lineage is cataloged; signing, cask lifecycle, and installed-host evidence remain open.
