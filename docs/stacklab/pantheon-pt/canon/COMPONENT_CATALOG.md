# Component catalog — Pantheon PT

| Component | Source surface | Write boundary | Evidence / next action |
|---|---|---|---|
| Go engine and CLI | `cmd/sirsi`, `internal/` | product commands | focused/full test receipts |
| Native surfaces | `macapp/`, menubar/TUI packages | bounded child processes | output/identity evidence |
| Native workspace shell | `PantheonDesktopView.swift`, `ControlCenter.swift`, `Views.swift`; recipe `contracts/stacklab/pantheon-native-workspace-v1.json` | local navigation preference; operation authority stays with the existing engine | Swift workspace tests; 900/1180/1440-point fixture renders; installed release remains SSA-owned |
| Mercury native observer | `MercuryMonitor.swift`, `MercuryWorkspaceView.swift` | fixed local GET plus existing Ra/hardware reads | bounded response/timeout, stale sample controls, opt-in existing-monitor decode test |
| Apollo/Ma'at plan continuity | `ApolloRunPlannerView.swift`, `MaatCasebookView.swift` | typed plan navigation; no repair inferred | machine/route/capacity drift controls; plan shown in guidance |
| Native project selection | `macapp/Sources/SirsiMenubar/{Views,SirsiEngine}.swift` | explicit user-selected Git root; invalid selection preserves the prior root | Git/worktree admission test and native retry guidance |
| Package | `.goreleaser.yaml`, package inventory | create-only build roots | unsigned inventory evidence |
| Cask | `internal/caskrelease/`, `cmd/sirsi/cask_release.go`, release workflow | create-only remote-tap publication | canonical bytes, remote readback, and lifecycle proof |
| Release boundary | `.github/workflows/release.yml`, `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`, release scripts | explicit development/commercial artifact selection and remote release/tag authority | static mode contract, then signing/notary and remote receipts |

Owner: Pantheon PT lane. Release state: source lineage is cataloged; signing, cask lifecycle, and installed-host evidence remain open.
