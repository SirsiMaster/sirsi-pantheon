// Package cleaner implements the deletion engine and safety module for Sirsi Anubis.
// This is the LAST LINE OF DEFENSE before any file is deleted.
//
// SAFETY DESIGN (docs/SAFETY_DESIGN.md):
// - Protected paths are HARDCODED and CANNOT be overridden
// - Every deletion passes through ValidatePath before execution
// - Dry-run mode is enforced at this level
package cleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// protectedPrefixes are now provided by platform.Current().ProtectedPrefixes().
// Each platform implementation (Darwin, Linux, Mock) defines its own list.
// This ensures protected paths are always correct for the running OS
// and can be overridden in tests via platform.Set(&Mock{}).

// protectedSuffixes are file patterns that MUST NEVER be deleted.
var protectedSuffixes = []string{
	".keychain-db",
	".keychain",
}

// protectedNames are exact filenames/dirnames that MUST NEVER be deleted.
var protectedNames = []string{
	".git",
	".env",
	".ssh",
	".gnupg",
	"id_rsa",
	"id_ed25519",
}

// protectedExact are exact paths (relative to home) that MUST NEVER be deleted.
var protectedExact = []string{
	"Library/Keychains/login.keychain-db",
	"Library/Keychains/System.keychain",
	".config/anubis", // Own config directory
}

// protectedHomeDirs are directories directly under $HOME that MUST NEVER
// be deleted as a whole. Individual files inside them can be removed,
// but passing the directory itself to DeleteFile is blocked.
// This prevents a bug from doing os.RemoveAll(~/Desktop).
var protectedHomeDirs = []string{
	"Desktop",
	"Documents",
	"Downloads",
	"Pictures",
	"Music",
	"Movies",
	"Library",
}

// ValidatePath checks if a path is safe to delete.
// Returns an error if the path is protected.
//
// It validates BOTH the lexical absolute path AND the symlink-resolved real
// target: a symlink (or a symlinked parent directory) whose name looks innocent
// must not be able to smuggle a delete into a protected location it points at.
// The protected-path guarantee (Rule A1: "cannot be overridden") would otherwise
// be bypassable with a link named e.g. "cache" pointing into ~/.ssh, or a scan
// root that is itself a symlink into /System.
func ValidatePath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve path %q: %w", path, err)
	}

	// Lexical check first.
	if err := checkProtected(absPath); err != nil {
		return err
	}

	// Then the symlink-resolved real target. Resolve the full path when it
	// exists; otherwise resolve the parent and re-attach the base so a symlinked
	// parent directory is still caught even when the leaf is already gone.
	realPath := absPath
	if resolved, e := filepath.EvalSymlinks(absPath); e == nil {
		realPath = resolved
	} else if parent, pe := filepath.EvalSymlinks(filepath.Dir(absPath)); pe == nil {
		realPath = filepath.Join(parent, filepath.Base(absPath))
	}
	if realPath != absPath {
		if err := checkProtected(realPath); err != nil {
			return fmt.Errorf("%w (resolved via symlink from %q)", err, absPath)
		}
	}

	return nil
}

// checkProtected runs the protected prefix/suffix/name/exact matrix against a
// single absolute path. ValidatePath applies it to both the lexical path and the
// symlink-resolved real target.
func checkProtected(absPath string) error {
	// Live model substrate — a running SNE service has this directory open.
	// Checked here rather than in each caller because ValidatePath is the one
	// gate every delete path (DeleteFile, DeleteFileReversible, CleanFile)
	// already funnels through.
	if live, hit := ConflictsWithLiveModel(absPath); hit {
		return fmt.Errorf("BLOCKED: %q holds the live model substrate %q used by a running Sirsi engine", absPath, live)
	}

	// Check platform-specific protected prefixes
	for _, prefix := range platform.Current().ProtectedPrefixes() {
		if strings.HasPrefix(absPath, prefix) {
			return fmt.Errorf("BLOCKED: %q is under protected path %q", absPath, prefix)
		}
	}

	// Check protected suffixes
	for _, suffix := range protectedSuffixes {
		if strings.HasSuffix(absPath, suffix) {
			return fmt.Errorf("BLOCKED: %q matches protected pattern *%s", absPath, suffix)
		}
	}

	// Check protected names (anywhere in path)
	baseName := filepath.Base(absPath)
	for _, name := range protectedNames {
		if baseName == name {
			return fmt.Errorf("BLOCKED: %q is a protected file/directory", absPath)
		}
	}

	// Check protected exact paths (relative to home)
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		relPath, err := filepath.Rel(homeDir, absPath)
		if err == nil {
			for _, exact := range protectedExact {
				if relPath == exact || strings.HasPrefix(relPath, exact+"/") {
					return fmt.Errorf("BLOCKED: %q is a protected path", absPath)
				}
			}
			// Block deletion of user content root directories
			// (e.g., ~/Desktop, ~/Documents — individual files inside are OK)
			for _, dir := range protectedHomeDirs {
				if relPath == dir {
					return fmt.Errorf("BLOCKED: %q is a protected user directory", absPath)
				}
			}
		}
	}

	return nil
}

// DeleteFile removes a file or empty directory after safety validation.
// Returns the number of bytes freed.
// DEPRECATED: Use CleanFile with a DecisionLog for full audit trail.
func DeleteFile(path string, dryRun bool, useTrash bool) (int64, error) {
	// SAFETY: Validate path before ANY operation
	if err := ValidatePath(path); err != nil {
		return 0, err
	}

	// Get size before deletion
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // Already gone
		}
		return 0, fmt.Errorf("cannot stat %q: %w", path, err)
	}

	size := info.Size()
	if info.IsDir() {
		size = DirSize(path)
	}

	// Dry-run: report what would happen
	if dryRun {
		logging.Debug("Dry-run: would delete", "path", path, "size", size)
		return size, nil
	}

	// Trash mode — always prefer on platforms that support it
	if useTrash && platform.Current().SupportsTrash() {
		return size, platform.Current().MoveToTrash(path)
	}

	// Direct delete
	if info.IsDir() {
		logging.Warn("Deleting directory permanently", "path", path, "size", size)
		return size, os.RemoveAll(path)
	}
	logging.Warn("Deleting file permanently", "path", path, "size", size)
	return size, os.Remove(path)
}

// DeleteFileReversible removes a file only if the platform supports trash.
// On platforms without trash support, returns an error instead of permanently
// deleting. Use this for user-facing cleanup flows where reversibility is required.
func DeleteFileReversible(path string, dryRun bool) (int64, error) {
	if err := ValidatePath(path); err != nil {
		return 0, err
	}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("cannot stat %q: %w", path, err)
	}

	size := info.Size()
	if info.IsDir() {
		size = DirSize(path)
	}

	if dryRun {
		logging.Debug("Dry-run: would trash", "path", path, "size", size)
		return size, nil
	}

	if !platform.Current().SupportsTrash() {
		return 0, fmt.Errorf("cannot safely delete %q: platform does not support trash — use DeleteFile with useTrash=false for permanent deletion", path)
	}

	return size, platform.Current().MoveToTrash(path)
}

// CleanFile removes a file with full decision logging.
// Policy:
//   - Always requires human-confirmed decision (no auto-delete)
//   - dryRun previews without touching the file (Rule A1 — every destructive op
//     has a no-op preview), recording a "dry-run" decision instead of removing
//   - Always trash first on macOS (reversible)
//   - Records every action with path, size, hash, reason, timestamp
//   - Permanent delete only via explicit EmptyTrash after review
func CleanFile(path string, dryRun bool, reason string, groupID string, hash string, log *DecisionLog) (int64, error) {
	// SAFETY: Validate path before ANY operation
	if err := ValidatePath(path); err != nil {
		_ = log.Record(Decision{
			Path:   path,
			Action: "skip",
			Reason: fmt.Sprintf("blocked by safety: %v", err),
		})
		return 0, err
	}

	// Get size
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("cannot stat %q: %w", path, err)
	}
	size := info.Size()

	// Dry-run: report what WOULD be removed without touching it, and record a
	// "dry-run" decision so the ledger shows the preview rather than a removal
	// that never happened (Rule A1: a destructive op must have a no-op preview).
	if dryRun {
		logging.Debug("Dry-run: would clean", "path", path, "size", size)
		_ = log.Record(Decision{
			Path:       path,
			Size:       size,
			Action:     "dry-run",
			Reason:     reason,
			DupGroupID: groupID,
			SHA256:     hash,
		})
		return size, nil
	}

	// Always trash on platforms that support it (reversible)
	if platform.Current().SupportsTrash() {
		if err := platform.Current().MoveToTrash(path); err != nil {
			logging.Error("Failed to move to trash", "path", path, "error", err)
			return 0, fmt.Errorf("move to trash: %w", err)
		}
		logging.Info("Moved to trash", "path", path, "size", size)
		_ = log.Record(Decision{
			Path:       path,
			Size:       size,
			Action:     "trash",
			Reason:     reason,
			DupGroupID: groupID,
			SHA256:     hash,
			Reversible: true,
		})
		return size, nil
	}

	// Non-macOS: direct delete (not reversible)
	if err := os.Remove(path); err != nil {
		return 0, err
	}
	_ = log.Record(Decision{
		Path:       path,
		Size:       size,
		Action:     "delete",
		Reason:     reason,
		DupGroupID: groupID,
		SHA256:     hash,
		Reversible: false,
	})
	return size, nil
}

// DirSize calculates the total size of a directory recursively.
func DirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, keep counting
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// moveToTrash is now handled by platform.Current().MoveToTrash().
// See internal/platform/darwin.go for the macOS implementation.
