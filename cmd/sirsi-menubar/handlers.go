// Package main — sirsi-menubar
//
// handlers.go — binary discovery for the TUI bridge.
package main

import "github.com/SirsiMaster/sirsi-pantheon/internal/setup"

// findSirsiBinary returns the absolute path to the `sirsi` binary. The
// LaunchAgent strips PATH, so we delegate to setup.SirsiBinaryPath which
// resolves via sibling of os.Executable() first — bulletproof against the
// `exec: "sirsi": executable file not found in $PATH` regression.
func findSirsiBinary() string { return setup.SirsiBinaryPath() }
