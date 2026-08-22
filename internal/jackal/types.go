// Package jackal implements the local scan engine for Sirsi Anubis.
// Named after the jackal form of Anubis — the hunter that patrols
// and cleans individual machines.
package jackal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Category represents a scan rule category.
type Category string

const (
	CategoryGeneral        Category = "general"
	CategoryVirtualization Category = "vms"
	CategoryDev            Category = "dev"
	CategoryAI             Category = "ai"
	CategoryIDEs           Category = "ides"
	CategoryCloud          Category = "cloud"
	CategoryStorage        Category = "storage"
)

// Severity indicates how safe it is to delete the finding.
type Severity string

const (
	SeveritySafe    Severity = "safe"    // Always safe to delete (caches, logs, temp)
	SeverityCaution Severity = "caution" // Probably safe, but review first (build artifacts)
	SeverityWarning Severity = "warning" // May cause issues if deleted (config, data)
)

// Finding represents a single artifact discovered by a scan rule.
type Finding struct {
	// Rule name that generated this finding
	RuleName string

	// Category this finding belongs to
	Category Category

	// Human-readable description
	Description string

	// Absolute path to the artifact
	Path string

	// Size in bytes (0 if unknown or directory total)
	SizeBytes int64

	// Number of files (for directory findings)
	FileCount int

	// How safe is this to delete
	Severity Severity

	// When the artifact was last modified
	LastModified time.Time

	// Whether this requires sudo to clean
	RequiresSudo bool

	// Whether this is a directory (vs single file)
	IsDir bool

	// Advisory is a one-line explanation of what this finding means
	// and what happens if it's cleaned. Written for the user, not the developer.
	Advisory string

	// Remediation describes what Sirsi will do to fix this finding.
	// Examples: "Move to Trash", "git branch -D", "docker image prune", "git gc"
	Remediation string

	// CanFix indicates whether Sirsi has an automated remediation for this finding.
	// If false, the finding is informational — flagged but not actionable.
	CanFix bool

	// Breaking indicates whether remediation could affect running services or workflows.
	Breaking bool
}

// CleanResult reports the outcome of a clean operation.
type CleanResult struct {
	// How many findings were cleaned
	Cleaned int

	// How many bytes were freed
	BytesFreed int64

	// How many findings were skipped (protected, errors)
	Skipped int

	// Errors encountered during cleaning
	Errors []error
}

// ScanOptions controls how scanning behaves.
type ScanOptions struct {
	// Only scan these categories (empty = all)
	Categories []Category

	// Minimum file age to report (0 = no minimum)
	MinAgeDays int

	// Home directory override (for testing)
	HomeDir string

	// Manifest is the shared Horus filesystem index.
	// When set, rules query the index instead of walking the filesystem.
	// This enables the "walk once, share many" optimization (ADR-008).
	Manifest Manifest

	// DeepScan explicitly permits rules to walk when the platform manifest has
	// no answer. Resident and ordinary product scans leave this false; forensic
	// callers may opt in deliberately.
	DeepScan bool

	// OnProgress is called after each rule completes during scanning.
	// ruleName is the rule that finished, found is the number of findings,
	// size is the total bytes found by this rule, done/total track overall progress.
	OnProgress func(ruleName string, found int, size int64, done, total int)
}

// Manifest is Pantheon's bounded filesystem-discovery contract. On macOS the
// production implementation consumes Spotlight metadata rather than building a
// second broad filesystem index.
type Manifest interface {
	DirSizeAndCount(dir string) (int64, int)
	DirSize(dir string) int64
	Exists(path string) bool
	Glob(pattern string) []string
	FindDirsNamed(root, name string, maxDepth int) []string
}

// CleanOptions controls how cleaning behaves.
type CleanOptions struct {
	// If true, don't actually delete anything
	DryRun bool

	// If true, move to Trash instead of deleting (macOS)
	UseTrash bool

	// If true, skip confirmation prompts
	Confirm bool
}

// ScanRule is the interface every scan rule must implement.
// This is the core abstraction of the Jackal engine.
//
// Rules MUST follow these invariants (ANUBIS_RULES.md Rule A2):
//   - Scan() has ZERO side effects — read-only filesystem access
//   - Clean() requires explicit confirmation — never auto-deletes
//   - EstimateSize() is best-effort — may return 0 if unknown
type ScanRule interface {
	// Name returns the unique identifier for this rule (e.g., "system_caches").
	Name() string

	// DisplayName returns the human-readable name (e.g., "System & Application Caches").
	DisplayName() string

	// Category returns which category this rule belongs to.
	Category() Category

	// Description returns a brief description of what this rule scans for.
	Description() string

	// Platforms returns which platforms this rule applies to.
	// Valid values: "darwin", "linux", "windows"
	Platforms() []string

	// Scan discovers artifacts and returns findings.
	// This method MUST have zero side effects.
	Scan(ctx context.Context, opts ScanOptions) ([]Finding, error)

	// Clean removes the artifacts identified by findings.
	// This method MUST respect CleanOptions (dry-run, trash, confirm).
	Clean(ctx context.Context, findings []Finding, opts CleanOptions) (*CleanResult, error)
}

// ExpandPath resolves ~ and environment variables in a path.
func ExpandPath(path string, homeDir string) string {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(homeDir, path[2:])
	}
	return os.ExpandEnv(path)
}

// PlatformMatch checks if the current platform is in the list.
func PlatformMatch(platforms []string) bool {
	for _, p := range platforms {
		if p == runtime.GOOS {
			return true
		}
	}
	return false
}

// FormatSize returns a human-readable size string.
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
