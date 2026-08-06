package thoth

// ADR-016 Phase 2: Thin wrappers that delegate to sirsi-thoth npm binaries.
//
// Sprint Plan (Rule 14):
//   1. Define mutex-protected function pointers for exec.Command (Rule A16, A21)
//   2. Provide LookPath + RunCommand abstractions for testability
//   3. Implement TryInit, TrySync, TryCompact that shell out to npm binaries
//   4. Each returns (delegated bool, err error) so callers can fall back to Go
//   5. The Go implementation remains intact as fallback -- no harm (Rule A12)
//
// Pattern: follows journal.go's exec.Command usage, with Rule A21 mutex guards.

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// ── Injectable Providers (Rule A16 + A21) ──────────────────────────

var (
	lookPathMu sync.RWMutex
	lookPathFn = exec.LookPath
)

func getLookPathFn() func(string) (string, error) {
	lookPathMu.RLock()
	defer lookPathMu.RUnlock()
	return lookPathFn
}

func setLookPathFn(fn func(string) (string, error)) {
	lookPathMu.Lock()
	defer lookPathMu.Unlock()
	lookPathFn = fn
}

var (
	runCmdMu sync.RWMutex
	runCmdFn = defaultRunCmd
)

func getRunCmdFn() func(name string, args ...string) (stdout string, stderr string, err error) {
	runCmdMu.RLock()
	defer runCmdMu.RUnlock()
	return runCmdFn
}

func setRunCmdFn(fn func(name string, args ...string) (string, string, error)) {
	runCmdMu.Lock()
	defer runCmdMu.Unlock()
	runCmdFn = fn
}

func defaultRunCmd(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// ── Binary availability check ──────────────────────────────────────

// BinaryAvailable reports whether a sirsi-thoth binary is on PATH.
func BinaryAvailable(name string) bool {
	lookup := getLookPathFn()
	_, err := lookup(name)
	return err == nil
}

// ── Delegate: thoth-init ───────────────────────────────────────────

// TryDelegateInit attempts to run thoth-init via the npm binary.
// Returns (true, nil) on success, (true, err) on binary failure,
// or (false, nil) if the binary is not installed (caller should fall back).
func TryDelegateInit(opts InitOptions) (delegated bool, err error) {
	if !BinaryAvailable("thoth-init") {
		return false, nil
	}

	args := []string{}
	if opts.RepoRoot != "" && opts.RepoRoot != "." {
		args = append(args, opts.RepoRoot)
	}
	if opts.Yes {
		args = append(args, "--yes")
	}
	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}
	if opts.Language != "" {
		args = append(args, "--language", opts.Language)
	}
	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}

	run := getRunCmdFn()
	stdout, stderr, err := run("thoth-init", args...)
	if err != nil {
		return true, fmt.Errorf("thoth-init failed: %w\nstderr: %s", err, strings.TrimSpace(stderr))
	}

	// Print the binary's output (it handles its own formatting)
	if stdout != "" {
		fmt.Print(stdout)
	}
	return true, nil
}

// ── Delegate: thoth-sync ──────────────────────────────────────────

// TryDelegateSync attempts to run thoth-sync via the npm binary.
// Returns (true, nil) on success, (true, err) on binary failure,
// or (false, nil) if the binary is not installed.
func TryDelegateSync(opts SyncOptions) (delegated bool, err error) {
	if !BinaryAvailable("thoth-sync") {
		return false, nil
	}

	args := []string{}
	if opts.RepoRoot != "" {
		args = append(args, "--path", opts.RepoRoot)
	}
	if opts.UpdateDate {
		args = append(args, "--update-date")
	}

	run := getRunCmdFn()
	_, stderr, err := run("thoth-sync", args...)
	if err != nil {
		return true, fmt.Errorf("thoth-sync failed: %w\nstderr: %s", err, strings.TrimSpace(stderr))
	}
	return true, nil
}

// ── Delegate: thoth-compact ───────────────────────────────────────

// TryDelegateCompact attempts to run thoth-compact via the npm binary.
// Returns (true, nil) on success, (true, err) on binary failure,
// or (false, nil) if the binary is not installed.
func TryDelegateCompact(opts CompactOptions) (delegated bool, err error) {
	if !BinaryAvailable("thoth-compact") {
		return false, nil
	}

	args := []string{}
	if opts.RepoRoot != "" {
		args = append(args, "--path", opts.RepoRoot)
	}
	if opts.Summary != "" {
		// The npm binary writes memory.yaml ITSELF, so the Go-side payload
		// filter in appendSessionDecisions never sees this path — hook payloads
		// rode straight through delegation into "Session Decisions" (claude-home,
		// router item 20260730-040045: "npm delegation skips it entirely").
		// Sanitize at the boundary so both writers receive the same summary.
		clean := SanitizeSummary(opts.Summary)
		if strings.TrimSpace(clean) == "" {
			return true, nil // nothing decision-shaped; do not invoke a writer at all
		}
		args = append(args, "--summary", clean)
	}
	if opts.MaxAge > 0 {
		args = append(args, "--max-age", fmt.Sprintf("%d", opts.MaxAge))
	}
	if opts.MaxKeep > 0 {
		args = append(args, "--max-keep", fmt.Sprintf("%d", opts.MaxKeep))
	}

	run := getRunCmdFn()
	_, stderr, err := run("thoth-compact", args...)
	if err != nil {
		return true, fmt.Errorf("thoth-compact failed: %w\nstderr: %s", err, strings.TrimSpace(stderr))
	}
	return true, nil
}
