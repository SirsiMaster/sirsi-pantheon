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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// protectedPrefixes are paths that MUST NEVER be deleted.
// This list is HARDCODED and CANNOT be overridden by configuration,
// flags, CLI arguments, or user input. (Rule A1)
var protectedPrefixes = map[string][]string{
	"darwin": {
		"/System/",
		"/usr/",
		"/bin/",
		"/sbin/",
		"/private/var/db/",
		"/Library/Extensions/",
		"/Library/Frameworks/",
	},
	"linux": {
		"/boot/",
		"/etc/",
		"/usr/",
		"/bin/",
		"/sbin/",
		"/lib/",
		"/lib64/",
		"/proc/",
		"/sys/",
		"/dev/",
		"/var/lib/dpkg/",
		"/var/lib/rpm/",
	},
}

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
func ValidatePath(path string) error {
	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve path %q: %w", path, err)
	}

	// Check platform-specific protected prefixes
	if prefixes, ok := protectedPrefixes[runtime.GOOS]; ok {
		for _, prefix := range prefixes {
			if strings.HasPrefix(absPath, prefix) {
				return fmt.Errorf("BLOCKED: %q is under protected path %q", absPath, prefix)
			}
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
		return size, nil
	}

	// Trash mode (macOS)
	if useTrash && runtime.GOOS == "darwin" {
		return size, moveToTrash(path)
	}

	// Direct delete
	if info.IsDir() {
		return size, os.RemoveAll(path)
	}
	return size, os.Remove(path)
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

// moveToTrash uses macOS Finder to move a file to Trash.
// This preserves the "Put Back" functionality.
func moveToTrash(path string) error {
	absPath, _ := filepath.Abs(path)
	script := fmt.Sprintf(
		`tell application "Finder" to delete POSIX file %q`,
		absPath,
	)
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}
