//go:build !darwin

// Package main — sirsi-gui (non-macOS stub).
//
// The native GUI surface is macOS-only (it wraps WebKit). On other platforms
// the CLI and TUI are the interactive surfaces; this stub keeps `go build ./...`
// green everywhere without pulling the WebKit/cgo dependency onto Linux.
package main

import "fmt"

func main() {
	fmt.Println("The Sirsi Pantheon GUI is macOS only. Use `sirsi tui` or the CLI on this platform.")
}
