package rules

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/cleaner"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// baseScanRule provides shared functionality for simple path-based rules.
type baseScanRule struct {
	name        string
	displayName string
	category    jackal.Category
	description string
	platforms   []string
	paths       []string        // Paths to scan (supports ~ expansion)
	excludes    []string        // Paths to exclude
	minAgeDays  int             // Minimum age in days (0 = no minimum)
	severity    jackal.Severity // Default severity for findings (empty = SeveritySafe)
}

func (r *baseScanRule) effectiveSeverity() jackal.Severity {
	if r.severity != "" {
		return r.severity
	}
	return jackal.SeveritySafe
}

func (r *baseScanRule) Name() string              { return r.name }
func (r *baseScanRule) DisplayName() string       { return r.displayName }
func (r *baseScanRule) Category() jackal.Category { return r.category }
func (r *baseScanRule) Description() string       { return r.description }
func (r *baseScanRule) Platforms() []string       { return r.platforms }

func (r *baseScanRule) Scan(ctx context.Context, opts jackal.ScanOptions) ([]jackal.Finding, error) {
	var findings []jackal.Finding
	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	minAge := r.minAgeDays
	if opts.MinAgeDays > 0 {
		minAge = opts.MinAgeDays
	}

	for _, pattern := range r.paths {
		expanded := jackal.ExpandPath(pattern, homeDir)

		// Resolve glob — use Horus index if available, then fallback to disk for files.
		var matches []string
		if opts.Manifest != nil {
			matches = opts.Manifest.Glob(expanded)
		}
		// Horus Phase 2 is directory-only. If no directories matched or Horus is absent,
		// fallback to a real filesystem glob to catch individual files.
		if len(matches) == 0 {
			var err error
			matches, err = filepath.Glob(expanded)
			if err != nil {
				continue
			}
		}

		for _, match := range matches {
			// Check excludes
			if r.isExcluded(match, homeDir) {
				continue
			}

			// Get file info — use Horus Exists for quick check, then Lstat for age.
			// Age filtering still needs real stat (manifest doesn't store modtime).
			info, err := os.Lstat(match)
			if err != nil {
				// If Lstat fails but manifest says it exists, it might be permission-denied.
				if opts.Manifest != nil && opts.Manifest.Exists(match) {
					// Use manifest data instead.
					size, fileCount := opts.Manifest.DirSizeAndCount(match)
					if size > 0 {
						findings = append(findings, jackal.Finding{
							RuleName:    r.name,
							Category:    r.category,
							Description: r.displayName,
							Path:        match,
							SizeBytes:   size,
							FileCount:   fileCount,
							Severity:    r.effectiveSeverity(),
							IsDir:       true,
						})
					}
				}
				continue
			}

			// Check minimum age
			if minAge > 0 {
				cutoff := time.Now().AddDate(0, 0, -minAge)
				if info.ModTime().After(cutoff) {
					continue
				}
			}

			size := info.Size()
			isDir := info.IsDir()
			fileCount := 1
			if isDir {
				if opts.Manifest != nil {
					// Horus: O(1) hash lookup
					size, fileCount = opts.Manifest.DirSizeAndCount(match)
				} else {
					// Fallback: combined filesystem walk
					size, fileCount = dirSizeAndCount(match)
				}
			}

			// Skip empty directories/files
			if size == 0 {
				continue
			}

			findings = append(findings, jackal.Finding{
				RuleName:     r.name,
				Category:     r.category,
				Description:  r.displayName,
				Path:         match,
				SizeBytes:    size,
				FileCount:    fileCount,
				Severity:     r.effectiveSeverity(),
				LastModified: info.ModTime(),
				IsDir:        isDir,
			})
		}
	}

	return findings, nil
}

func (r *baseScanRule) Clean(ctx context.Context, findings []jackal.Finding, opts jackal.CleanOptions) (*jackal.CleanResult, error) {
	result := &jackal.CleanResult{}

	for _, f := range findings {
		freed, err := cleaner.DeleteFile(f.Path, opts.DryRun, opts.UseTrash)
		if err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Cleaned++
		result.BytesFreed += freed
	}

	return result, nil
}

func (r *baseScanRule) isExcluded(path string, homeDir string) bool {
	for _, exclude := range r.excludes {
		expanded := jackal.ExpandPath(exclude, homeDir)
		matched, _ := filepath.Match(expanded, path)
		if matched {
			return true
		}
		// Also check prefix match for directories
		if len(expanded) > 0 && expanded[len(expanded)-1] != '*' {
			expandedDir := expanded
			if filepath.IsAbs(path) && filepath.IsAbs(expandedDir) {
				rel, err := filepath.Rel(expandedDir, path)
				if err == nil && rel != ".." && !filepath.IsAbs(rel) {
					return true
				}
			}
		}
	}
	return false
}

// dirSizeAndCount walks a directory once and returns total size and file count.
// Uses filepath.WalkDir (Go 1.16+) which avoids os.Stat per entry — significantly
// faster than filepath.Walk on directories with thousands of files.
// Capped at maxDirWalkFiles to prevent unbounded walks (B11).
const maxDirWalkFiles = 100_000

func dirSizeAndCount(dir string) (int64, int) {
	var totalSize int64
	count := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			count++
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
			}
			if count >= maxDirWalkFiles {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return totalSize, count
}
