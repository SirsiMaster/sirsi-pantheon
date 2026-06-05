// Package setup is the shared first-run wizard engine for Pantheon.
//
// One engine, two surfaces (Critical Design Decision #5 — "One Engine, Two
// Interfaces"): the `sirsi setup` CLI renders this engine, and the menubar
// config row spawns a terminal that runs that same CLI command. Both entry
// points therefore drive identical logic and can never drift. Keep this
// package free of prompts and rendering — it reports state and performs
// discrete actions; the caller owns interaction.
package setup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// Status is the resolved state of a dependency or wizard step.
type Status int

const (
	// StatusOK means the requirement is satisfied.
	StatusOK Status = iota
	// StatusMissing means it is not satisfied but is actionable.
	StatusMissing
	// StatusFailed means an action was attempted and failed.
	StatusFailed
	// StatusSkipped means it does not apply on this platform.
	StatusSkipped
)

// Dependency is an external tool Pantheon can use.
type Dependency struct {
	Name        string
	Description string
	Required    bool
	InstallCmd  string
}

// Dependencies returns the canonical external-tool list for the host.
//
// Thoth is intentionally absent: it ships inside the sirsi binary
// (`sirsi thoth`), so the old npm thoth-init/thoth-sync/thoth-compact shims
// reported three false "missing" dependencies the user already had via sirsi
// itself. The wizard should never tell a freshly-installed user they are
// missing tools that came in the same binary.
func Dependencies() []Dependency {
	return []Dependency{
		{Name: "git", Description: "Version control (Thoth, Ma'at, Ra)", Required: true, InstallCmd: "xcode-select --install"},
		{Name: "go", Description: "Go compiler (build from source, Ma'at coverage)", InstallCmd: "brew install go"},
		{Name: "golangci-lint", Description: "Linter (Ma'at pre-push gate, matches CI)", InstallCmd: "brew install golangci-lint"},
		{Name: "gh", Description: "GitHub CLI (Ma'at pipeline, Ra CI status)", InstallCmd: "brew install gh"},
		{Name: "python3", Description: "Ra deployment agent (scope orchestration)", InstallCmd: "brew install python3"},
	}
}

// Installed reports whether the dependency is on PATH, with a short version
// string when one is cheaply available.
func (d Dependency) Installed() (bool, string) {
	path, err := exec.LookPath(d.Name)
	if err != nil {
		return false, ""
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return true, ""
	}
	first := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	if len(first) >= 80 {
		return true, ""
	}
	return true, first
}

// Install runs the dependency's documented install command, streaming child
// output to the given writers. It returns an error if the command fails or the
// tool is still not on PATH afterward.
func (d Dependency) Install(stdout, stderr io.Writer) error {
	parts := strings.Fields(d.InstallCmd)
	if len(parts) == 0 {
		return fmt.Errorf("no install command for %s", d.Name)
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if _, err := exec.LookPath(d.Name); err != nil {
		return fmt.Errorf("installed but not on PATH (restart your terminal)")
	}
	return nil
}

// NeedsFDA reports whether a command name touches user directories that need
// Full Disk Access to scan without per-folder prompts.
func NeedsFDA(cmdName string) bool {
	switch cmdName {
	case "scan", "ghosts", "duplicates", "purge", "installer", "analyze", "clean":
		return true
	}
	return false
}

// FullDiskAccessGranted reports whether this process can read TCC-protected
// paths. On non-darwin hosts it is always true (not applicable).
func FullDiskAccessGranted() bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	// The TCC database directory is readable only with Full Disk Access.
	_, err := os.ReadDir("/Library/Application Support/com.apple.TCC")
	return err == nil
}

// OpenFullDiskAccessPane opens System Settings to the Full Disk Access pane.
// No-op on non-darwin.
func OpenFullDiskAccessPane() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles").Start()
}

// BinaryPath returns the resolved path of the running sirsi binary — the exact
// path the user must add to Full Disk Access.
func BinaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "sirsi"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

// RegisterAgentWake detects installed AI CLIs and records wake profiles in the
// router's agents.json, returning the agent ids newly added. An error from
// repo-root resolution (e.g. running outside a repo) is returned so the caller
// can report it as a soft warning rather than a fatal failure.
func RegisterAgentWake() ([]string, error) {
	root, err := router.FindRepoRoot()
	if err != nil {
		return nil, err
	}
	return router.DetectInstalledAgents(root)
}
