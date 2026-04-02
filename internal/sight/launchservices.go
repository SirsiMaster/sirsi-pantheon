// Package sight provides Spotlight and Launch Services management.
// Extracts ghost-detection logic from Ka into a dedicated module
// that can also rebuild the Launch Services database.
package sight

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// CommandRunner executes a system command and returns its error.
// Inject a mock for testing without side effects.
type CommandRunner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

// StreamingRunner is an optional extension of CommandRunner that supports
// streaming stdout line-by-line instead of buffering the entire output.
// defaultRunner implements this; test mocks fall back to Output().
type StreamingRunner interface {
	CommandRunner
	StreamLines(name string, args ...string) (func() (string, bool), func() error, error)
}

// GhostRegistration represents an app registered in Launch Services
// whose .app bundle no longer exists on disk.
type GhostRegistration struct {
	BundleID string
	Path     string // Original .app path
	Name     string
}

// SightResult contains the results of a Spotlight/Launch Services audit.
type SightResult struct {
	GhostRegistrations []GhostRegistration
	TotalGhosts        int
	LaunchServicesSize int64
	CanFix             bool
}

// Scan queries Launch Services for ghost app registrations.
func Scan() (*SightResult, error) {
	return ScanWith(defaultRunner{})
}

// ScanWith is the injectable version of Scan.
func ScanWith(p CommandRunner) (*SightResult, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("sight: only supported on macOS")
	}

	result := &SightResult{CanFix: true}

	// Dump Launch Services database (can be very large — 30s timeout).
	// Use streaming when available to avoid buffering 20-50MB in memory.
	lsregister := "/System/Library/Frameworks/CoreServices.framework/Versions/A/Frameworks/LaunchServices.framework/Versions/A/Support/lsregister"

	var ghosts []GhostRegistration
	var err error
	if sr, ok := p.(StreamingRunner); ok {
		ghosts, err = parseLSRegisterStream(sr, lsregister, p)
	} else {
		// Fallback for test mocks that only implement CommandRunner.
		var out []byte
		out, err = p.Output(lsregister, "-dump")
		if err == nil {
			ghosts = parseLSRegisterDump(string(out), p)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("sight: lsregister dump failed: %w", err)
	}

	result.GhostRegistrations = ghosts
	result.TotalGhosts = len(ghosts)

	return result, nil
}

// Fix rebuilds the Launch Services database, removing ghost registrations.
// This is a DESTRUCTIVE operation — it resets all file associations.
func Fix(dryRun bool) error {
	return FixWith(dryRun, defaultRunner{})
}

// FixWith is the injectable version of Fix for testing.
func FixWith(dryRun bool, runner CommandRunner) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("sight: only supported on macOS")
	}

	if dryRun {
		return nil // Dry run — don't actually rebuild
	}

	lsregister := "/System/Library/Frameworks/CoreServices.framework/Versions/A/Frameworks/LaunchServices.framework/Versions/A/Support/lsregister"

	// Kill the Launch Services database and rebuild
	if err := runner.Run(lsregister, "-kill", "-r", "-domain", "local", "-domain", "system", "-domain", "user"); err != nil {
		return fmt.Errorf("sight: lsregister rebuild failed: %w", err)
	}

	// Restart Finder to pick up changes
	_ = runner.Run("killall", "Finder") // Non-fatal if Finder restart fails

	return nil
}

// ReindexSpotlight triggers a Spotlight re-index for the boot volume.
func ReindexSpotlight(dryRun bool) error {
	return ReindexSpotlightWith(dryRun, defaultRunner{})
}

// ReindexSpotlightWith is the injectable version for testing.
func ReindexSpotlightWith(dryRun bool, runner CommandRunner) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("sight: only supported on macOS")
	}
	if dryRun {
		return nil
	}

	if err := runner.Run("mdutil", "-E", "/"); err != nil {
		return fmt.Errorf("sight: Spotlight reindex failed (may need sudo): %w", err)
	}
	return nil
}

type defaultRunner struct{}

func (r defaultRunner) Run(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run()
}
func (r defaultRunner) Output(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// StreamLines starts a command and returns an iterator over stdout lines.
// The first return is a next function: call it repeatedly to get (line, ok).
// The second return is a cleanup function that waits for the process.
func (r defaultRunner) StreamLines(name string, args ...string) (func() (string, bool), func() error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024)

	next := func() (string, bool) {
		if scanner.Scan() {
			return scanner.Text(), true
		}
		return "", false
	}
	cleanup := func() error {
		defer cancel()
		return cmd.Wait()
	}
	return next, cleanup, nil
}

// parseLSRegisterStream extracts ghost registrations by streaming lsregister
// output line-by-line, avoiding buffering the full 20-50MB dump in memory.
func parseLSRegisterStream(sr StreamingRunner, lsregister string, p CommandRunner) ([]GhostRegistration, error) {
	next, cleanup, err := sr.StreamLines(lsregister, "-dump")
	if err != nil {
		return nil, err
	}
	defer func() { _ = cleanup() }()

	var ghosts []GhostRegistration
	seen := make(map[string]bool)

	// Accumulate lines per block (separated by dashes).
	var block []string
	separator := "--------------------------------------------------------------------------------"

	processBlock := func(lines []string) {
		var bundleID, path, name string
		hasApp := false
		hasBundleID := false

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "bundle id:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					bundleID = strings.TrimSpace(parts[1])
					hasBundleID = true
				}
			}
			if strings.HasPrefix(line, "path:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					path = strings.TrimSpace(parts[1])
				}
			}
			if strings.HasPrefix(line, "name:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					name = strings.TrimSpace(parts[1])
				}
			}
			if strings.Contains(line, ".app") {
				hasApp = true
			}
		}

		if !hasBundleID || !hasApp || bundleID == "" || path == "" {
			return
		}
		if strings.HasPrefix(bundleID, "com.apple.") {
			return
		}
		if !strings.Contains(path, ".app") {
			return
		}

		appPath := path
		if idx := strings.Index(path, ".app"); idx > 0 {
			appPath = path[:idx+4]
		}

		if seen[bundleID] {
			return
		}

		if err := p.Run("test", "-d", appPath); err != nil {
			seen[bundleID] = true
			if name == "" {
				name = bundleID
			}
			ghosts = append(ghosts, GhostRegistration{
				BundleID: bundleID,
				Path:     appPath,
				Name:     name,
			})
		}
	}

	for {
		line, ok := next()
		if !ok {
			break
		}
		if strings.TrimSpace(line) == separator {
			if len(block) > 0 {
				processBlock(block)
				block = block[:0]
			}
			continue
		}
		block = append(block, line)
	}
	// Process final block.
	if len(block) > 0 {
		processBlock(block)
	}

	return ghosts, nil
}

// parseLSRegisterDump extracts ghost registrations from lsregister output.
func parseLSRegisterDump(dump string, p CommandRunner) []GhostRegistration {
	var ghosts []GhostRegistration
	seen := make(map[string]bool)

	blocks := strings.Split(dump, "--------------------------------------------------------------------------------")

	for _, block := range blocks {
		if !strings.Contains(block, "bundle id:") {
			continue
		}
		if !strings.Contains(block, ".app") {
			continue
		}

		var bundleID, path, name string

		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)

			if strings.HasPrefix(line, "bundle id:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					bundleID = strings.TrimSpace(parts[1])
				}
			}
			if strings.HasPrefix(line, "path:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					path = strings.TrimSpace(parts[1])
				}
			}
			if strings.HasPrefix(line, "name:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					name = strings.TrimSpace(parts[1])
				}
			}
		}

		if bundleID == "" || path == "" {
			continue
		}

		// Skip Apple system apps
		if strings.HasPrefix(bundleID, "com.apple.") {
			continue
		}

		// Check if the .app exists
		if !strings.Contains(path, ".app") {
			continue
		}

		// Extract .app path
		appPath := path
		if idx := strings.Index(path, ".app"); idx > 0 {
			appPath = path[:idx+4]
		}

		// Check if already seen
		if seen[bundleID] {
			continue
		}

		// Check if .app exists on disk
		if err := p.Run("test", "-d", appPath); err != nil {
			seen[bundleID] = true
			if name == "" {
				name = bundleID
			}
			ghosts = append(ghosts, GhostRegistration{
				BundleID: bundleID,
				Path:     appPath,
				Name:     name,
			})
		}
	}

	return ghosts
}
