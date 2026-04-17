// Package main — sirsi-menubar
//
// handlers.go — Menu click handlers that dispatch to Sirsi CLI subcommands.
//
// Each handler spawns the corresponding `sirsi` CLI command in the background.
// Output is logged but not displayed (menu bar app is non-blocking).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Handler wraps a menu action with name and execution logic.
type Handler struct {
	Name    string
	Command string
	Args    []string
}

// SirsiHandlers returns the set of available menu actions.
func SirsiHandlers() []Handler {
	sirsiBin := findSirsiBinary()
	return []Handler{
		{Name: "Scan (Weigh)", Command: sirsiBin, Args: []string{"weigh"}},
		{Name: "Judge", Command: sirsiBin, Args: []string{"judge"}},
		{Name: "Guard", Command: sirsiBin, Args: []string{"guard"}},
		{Name: "Ka (Ghost Hunt)", Command: sirsiBin, Args: []string{"ka"}},
		{Name: "Mirror (Dedup)", Command: sirsiBin, Args: []string{"mirror"}},
		{Name: "Ma'at (QA)", Command: sirsiBin, Args: []string{"maat"}},
	}
}

// RaHandlers returns Ra orchestration menu actions.
func RaHandlers() []Handler {
	sirsiBin := findSirsiBinary()
	return []Handler{
		{Name: "𓇶 Ra Deploy", Command: sirsiBin, Args: []string{"ra", "deploy"}},
		{Name: "𓇶 Ra Kill All", Command: sirsiBin, Args: []string{"ra", "kill"}},
		{Name: "𓇶 Ra Collect", Command: sirsiBin, Args: []string{"ra", "collect"}},
		{Name: "𓇶 Ra Status", Command: sirsiBin, Args: []string{"ra", "status"}},
	}
}

// QuickActions returns quick-access menu actions.
func QuickActions() []Handler {
	sirsiBin := findSirsiBinary()
	return []Handler{
		{Name: "Start Watchdog", Command: sirsiBin, Args: []string{"guard", "--watch"}},
	}
}

// Execute runs the handler command in the background.
func (h *Handler) Execute() error {
	cmd := exec.Command(h.Command, h.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// OpenBuildLog opens the build log in the default browser.
func OpenBuildLog() error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "docs", "build-log.html")
	return exec.Command("open", path).Start()
}

// OpenCaseStudies opens the case studies in the default browser.
func OpenCaseStudies() error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "docs", "case-studies.html")
	return exec.Command("open", path).Start()
}

// OpenCommandCenter launches `sirsi ra watch` in a new terminal.
func OpenCommandCenter() error {
	sirsiBin := findSirsiBinary()
	cmd := exec.Command(sirsiBin, "ra", "watch")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// OpenScopeLog opens the log file for a given Ra scope.
func OpenScopeLog(scopeName string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(home, ".config", "ra", "logs", scopeName+".log")
	return exec.Command("open", "-a", "Console", logPath).Start()
}

// findSirsiBinary locates the sirsi binary.
func findSirsiBinary() string {
	// Check PATH first
	if p, err := exec.LookPath("sirsi"); err == nil {
		return p
	}
	// Check Homebrew location
	if p, err := exec.LookPath("/opt/homebrew/bin/sirsi"); err == nil {
		return p
	}
	// Check local bin
	if p, err := exec.LookPath("./bin/sirsi"); err == nil {
		return p
	}
	// Fallback to just "sirsi" and hope it's in PATH
	return "sirsi"
}

// findRepoRoot locates the sirsi-pantheon repo root.
func findRepoRoot() (string, error) {
	// Try git rev-parse first
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	// Fallback to known location
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home dir: %w", err)
	}
	candidate := filepath.Join(home, "Development", "sirsi-pantheon")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("cannot find sirsi-pantheon repo root")
}
