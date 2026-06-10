// Package selfupdate — selfheal.go
//
// The remediation half of ADR-023: the AMFI-safe atomic replace contract for
// a drifted CLI binary. Detection lives in selfupdate.go; this file performs
// the fix that answers the `sirsi diagnose` binary-drift finding.
//
// Background (the bug this fixes): on macOS, `cp`-ing a fresh Go binary OVER an
// existing one leaves a stale code-signing cdhash bound to the old inode, so
// the next exec is SIGKILL'd (137) by AMFI — silently killing LaunchAgents and
// heartbeats. `sirsi` has been its own #1 crash source on this host because of
// exactly this. The safe contract is: write a fresh inode, codesign it, then
// atomically rename it over the old one.
//
// SAFETY (PARAMOUNT) — the three guardrails (claude-home router 035409):
//  1. Detect ≠ apply. HasDrift-style detection is read-only and safe for
//     non-interactive contexts; SafeReplace is the apply primitive and never
//     prompts itself — the CALLER previews + confirms (Rule A1, preview≠apply).
//  2. Atomic, no half-states. Stage to a `.new` sibling, codesign that, then
//     rename(2) over the target (atomic on a single filesystem). An interrupt
//     mid-operation leaves the OLD working binary in place, never a gap.
//  3. Allow-list, not deny-list. SafeReplace writes ONLY to the known CLI bin
//     dirs (~/.local/bin, /opt/homebrew/bin, ~/go/bin, homebrew prefixes). Any
//     other path — emphatically including anything inside a `.app` bundle
//     (Rule A19 absolute) — is refused. Adding a path is a reviewed code change,
//     never a runtime override.
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// ErrAppBundleProtected is returned when a replace targets a path inside a
// macOS .app bundle. A19 forbids agent writes there — no exception. Reported
// explicitly (ahead of the generic allow-list error) so the A19 violation is
// loud and unambiguous.
var ErrAppBundleProtected = errors.New("refusing to modify a binary inside a .app bundle (Rule A19) — rebuild/relaunch the app instead")

// ErrPathNotAllowed is returned when a replace targets any path outside the
// hardcoded CLI bin allow-list.
var ErrPathNotAllowed = errors.New("refusing to replace a binary outside the known CLI bin dirs (allow-list, Rule A19 spirit)")

// ErrHomebrewManaged is returned when a replace targets a Homebrew-managed
// binary. SafeReplace must NEVER hand-overwrite a brew install (it would leave
// brew's manifest inconsistent and the next `brew` op would fight it) — the
// caller should instruct `brew upgrade` instead (binding-review confirm-item,
// claude-home 185740).
var ErrHomebrewManaged = errors.New("refusing to replace a Homebrew-managed binary — run `brew upgrade sirsi` instead")

// healExecFn runs an external command and returns combined output. Injectable
// (Rule A16) so SafeReplace's contract is unit-tested without mutating the host.
// Guarded by healExecMu per Rule A21 (concurrency-safe injectable mocks): a
// package-level function pointer swapped by a test while another goroutine reads
// it is a data race, so all access goes through getHealExecFn/setHealExecFn.
var (
	healExecMu sync.RWMutex
	healExecFn = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}
)

func getHealExecFn() func(string, ...string) ([]byte, error) {
	healExecMu.RLock()
	defer healExecMu.RUnlock()
	return healExecFn
}

// setHealExecFn swaps the runner under the write lock and returns the previous
// one (test-only; restore with setHealExecFn(old)).
func setHealExecFn(fn func(string, ...string) ([]byte, error)) func(string, ...string) ([]byte, error) {
	healExecMu.Lock()
	defer healExecMu.Unlock()
	old := healExecFn
	healExecFn = fn
	return old
}

// allowedBinDirsFn returns the allow-list of directories SafeReplace may write
// to. Injectable (Rule A16) so tests can point it at a temp dir instead of the
// real CLI bin locations. Production value is the hardcoded standard set below.
var allowedBinDirsFn = defaultAllowedBinDirs

// defaultAllowedBinDirs is the hardcoded allow-list — the standard
// user-managed CLI install locations. NOT a config value: extending it is a
// reviewed code change, per guardrail #3.
func defaultAllowedBinDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "go", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
	}
}

// guardCLIPath enforces guardrail #3 + A19: dst must live directly in one of
// the allow-listed CLI bin dirs, and never inside a .app bundle.
func guardCLIPath(dst string) error {
	abs, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dst, err)
	}
	// Explicit, loud A19 check first (clearer than a bare allow-list miss).
	if strings.Contains(abs+string(os.PathSeparator), ".app"+string(os.PathSeparator)) {
		return ErrAppBundleProtected
	}
	dir := filepath.Dir(abs)
	if slices.Contains(allowedBinDirsFn(), dir) {
		return nil
	}
	return fmt.Errorf("%w: %s not in %v", ErrPathNotAllowed, dir, allowedBinDirsFn())
}

// SafeReplace atomically replaces the binary at dst with the fresh binary at
// src using the AMFI-safe contract (stage .new → codesign → rename over dst).
//
// dst MUST be an allow-listed CLI path (guardrail #3 / A19). The caller owns
// the A1 preview+confirm before invoking this. On non-darwin the codesign step
// is skipped (the cdhash problem is macOS-only); the staged-rename still
// applies so the same path works for linux CLI installs.
func SafeReplace(src, dst string) (err error) {
	if guardErr := guardCLIPath(dst); guardErr != nil {
		return guardErr
	}
	// Never hand-overwrite a Homebrew-managed binary — delegate to `brew
	// upgrade` (binding-review confirm-item). The allow-list includes
	// /opt/homebrew/bin as a valid CLI location, but a binary brew actually
	// manages there must not be replaced out from under brew's manifest.
	if DetectMethod(dst) == MethodHomebrew {
		return ErrHomebrewManaged
	}
	srcInfo, statErr := os.Stat(src)
	if statErr != nil {
		return fmt.Errorf("source binary %s: %w", src, statErr)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("source %s is a directory, not a binary", src)
	}

	// Stage a fresh inode beside the target. The fresh inode is what clears the
	// stale-cdhash binding; codesign signs THIS inode before it goes live.
	staged := dst + ".new"
	_ = os.Remove(staged) // clear any leftover from a prior interrupted run
	if copyErr := copyFile(src, staged); copyErr != nil {
		return fmt.Errorf("stage %s: %w", staged, copyErr)
	}
	// On any failure past this point, don't leave the staged file behind.
	defer func() {
		if err != nil {
			_ = os.Remove(staged)
		}
	}()
	if chmodErr := os.Chmod(staged, 0o755); chmodErr != nil {
		return fmt.Errorf("chmod %s: %w", staged, chmodErr)
	}

	// codesign --force --sign - the STAGED inode (macOS only) before it goes
	// live, so the binary is validly signed the instant rename makes it active.
	if runtime.GOOS == "darwin" {
		if out, csErr := getHealExecFn()("codesign", "--force", "--sign", "-", staged); csErr != nil {
			return fmt.Errorf("codesign %s: %w (%s)", staged, csErr, strings.TrimSpace(string(out)))
		}
	}

	// Atomic swap: rename(2) over the target on the same filesystem. If this
	// fails, the old binary is untouched — never a gap (guardrail #2).
	if renameErr := os.Rename(staged, dst); renameErr != nil {
		return fmt.Errorf("rename %s -> %s: %w", staged, dst, renameErr)
	}
	return nil
}

// DriftTarget is an allow-listed CLI copy of the binary whose on-disk content
// differs from the running process — a candidate for self-heal. Present is the
// on-disk hash, Expected is the running-process hash. (Content sha256 is the
// portable drift signal; on macOS the cdhash that AMFI checks is derived from
// this same content, so a content match means a clean, re-signable binary.)
type DriftTarget struct {
	Path     string `json:"path"`
	Present  string `json:"present"`
	Expected string `json:"expected"`
}

// FileHash returns the hex sha256 of the file at path.
func FileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DetectCLIDrift returns the allow-listed CLI copies whose content differs from
// selfHash (the running binary's hash). Read-only and SAFE for non-interactive
// contexts (CI, hooks, the menubar tick). Idempotent: a copy that already
// matches selfHash is NOT returned (no rewrite of a clean binary). selfPath is
// skipped so we never list the running binary as drifted against itself.
func DetectCLIDrift(selfPath, selfHash string) []DriftTarget {
	selfClean := filepath.Clean(selfPath)
	var targets []DriftTarget
	seen := map[string]bool{}
	for _, dir := range allowedBinDirsFn() {
		path := filepath.Clean(filepath.Join(dir, "sirsi"))
		if seen[path] || path == selfClean {
			continue
		}
		seen[path] = true
		fi, err := os.Stat(path)
		if err != nil || fi.IsDir() {
			continue
		}
		present, err := FileHash(path)
		if err != nil || present == selfHash {
			continue // unreadable, or already converged (idempotent)
		}
		targets = append(targets, DriftTarget{Path: path, Present: present, Expected: selfHash})
	}
	return targets
}

// copyFile copies src to a fresh dst inode (dst must not already exist).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
