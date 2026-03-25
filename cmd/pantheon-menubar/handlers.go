// Package main — pantheon-menubar
//
// handlers.go — Menu click handlers that dispatch to Pantheon CLI subcommands.
//
// Each handler spawns the corresponding `pantheon` CLI command in the background.
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

// PantheonHandlers returns the set of available menu actions.
func PantheonHandlers() []Handler {
	pantheon := findPantheonBinary()
	return []Handler{
		{Name: "Scan (Weigh)", Command: pantheon, Args: []string{"weigh"}},
		{Name: "Judge", Command: pantheon, Args: []string{"judge"}},
		{Name: "Guard", Command: pantheon, Args: []string{"guard"}},
		{Name: "Ka (Ghost Hunt)", Command: pantheon, Args: []string{"ka"}},
		{Name: "Mirror (Dedup)", Command: pantheon, Args: []string{"mirror"}},
		{Name: "Ma'at (QA)", Command: pantheon, Args: []string{"maat"}},
	}
}

// QuickActions returns quick-access menu actions.
func QuickActions() []Handler {
	pantheon := findPantheonBinary()
	return []Handler{
		{Name: "Start Watchdog", Command: pantheon, Args: []string{"guard", "--watch"}},
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

// findPantheonBinary locates the pantheon binary.
func findPantheonBinary() string {
	// Check PATH first
	if p, err := exec.LookPath("pantheon"); err == nil {
		return p
	}
	// Check Homebrew location
	if p, err := exec.LookPath("/opt/homebrew/bin/pantheon"); err == nil {
		return p
	}
	// Check local bin
	if p, err := exec.LookPath("./bin/pantheon"); err == nil {
		return p
	}
	// Fallback to just "pantheon" and hope it's in PATH
	return "pantheon"
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
