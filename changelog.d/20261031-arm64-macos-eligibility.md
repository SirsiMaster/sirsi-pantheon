### Fixed

Aligned the full app minimum to macOS 13.0 and constrained the canonical Homebrew Cask to arm64 and macOS Ventura or newer, matching the existing native deployment target and published payload. No binary architecture or build behavior changed.

Refs: `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`, `macapp/Package.swift`, `internal/caskrelease/cask.go`
