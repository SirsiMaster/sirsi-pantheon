### Fixed

Aligned the full-app Info.plist minimum to macOS 13.0, matching the existing native deployment target. The canonical Cask preserves its exact historical template through v0.24.153, including those release trains' prereleases; arm64 and macOS Ventura guards begin with the v0.24.154 release train and its prereleases. Existing version validation remains in force, and historical rollback continues to consume the exact retained signed PKG rather than regenerate its app metadata. No binary architecture or build behavior changed.

Refs: `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`, `macapp/Package.swift`, `cmd/sirsi-menubar/bundle/Info.plist`, `internal/caskrelease/cask.go`, `cmd/sirsi/update_app_darwin.go`, `docs/ADR-032-MAC-FIRST-PLATFORM-ROADMAP.md`
