package rules

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/cleaner"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// ═══════════════════════════════════════════
// DEVELOPER FRAMEWORKS — node_modules, cargo, go, python
// ═══════════════════════════════════════════

// NewNodeModulesRule finds stale node_modules directories in dev projects.
func NewNodeModulesRule() jackal.ScanRule {
	return &findRule{
		name:        "node_modules",
		displayName: "Node Modules",
		category:    jackal.CategoryDev,
		description: "node_modules directories in development projects (often 100MB+ each)",
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{
			"~/Development",
			"~/code",
			"~/projects",
			"~/src",
		},
		maxDepth:   4,
		minAgeDays: 14,
	}
}

// NewGoModCacheRule scans the Go module cache.
func NewGoModCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "go_mod_cache",
		displayName: "Go Module Cache",
		category:    jackal.CategoryDev,
		description: "Go module download cache",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/go/pkg/mod/cache",
		},
	}
}

// NewPythonCachesRule scans Python caches and pip.
func NewPythonCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "python_caches",
		displayName: "Python Caches",
		category:    jackal.CategoryDev,
		description: "pip cache, conda packages, and __pycache__ directories",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/pip",
			"~/miniconda3/pkgs",
			"~/anaconda3/pkgs",
		},
	}
}

// NewRustTargetRule finds Rust build target directories.
func NewRustTargetRule() jackal.ScanRule {
	return &findRule{
		name:        "rust_targets",
		displayName: "Rust Build Targets",
		category:    jackal.CategoryDev,
		description: "Rust compilation target directories (often 1GB+ each)",
		platforms:   []string{"darwin", "linux"},
		targetName:  "target",
		searchPaths: []string{
			"~/Development",
			"~/code",
			"~/projects",
			"~/src",
		},
		maxDepth:   3,
		minAgeDays: 7,
		matchFile:  "Cargo.toml", // Only if parent has Cargo.toml
	}
}

// NewDockerRule scans Docker Desktop data.
func NewDockerRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "docker_desktop",
		displayName: "Docker Desktop",
		category:    jackal.CategoryDev,
		description: "Docker Desktop cache, images, and build cache",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Containers/com.docker.docker/Data/vms",
			"~/Library/Group Containers/group.com.docker/cache",
		},
	}
}

// NewXcodeDerivedDataRule scans Xcode derived data.
func NewXcodeDerivedDataRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "xcode_derived_data",
		displayName: "Xcode Derived Data",
		category:    jackal.CategoryIDEs,
		description: "Xcode build caches and derived data",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Developer/Xcode/DerivedData/*",
		},
		minAgeDays: 7,
	}
}

// ═══════════════════════════════════════════
// findRule — searches for named directories within search paths
// ═══════════════════════════════════════════

// findRule searches for directories by name within search paths.
// Used for node_modules, target, .next, etc.
type findRule struct {
	name        string
	displayName string
	category    jackal.Category
	description string
	platforms   []string
	targetName  string          // Directory name to find (e.g., "node_modules")
	searchPaths []string        // Root directories to search
	maxDepth    int             // Maximum search depth
	minAgeDays  int
	matchFile   string          // Optional: parent must contain this file
	severity    jackal.Severity // Default severity (empty = SeveritySafe)
}

func (r *findRule) effectiveSeverity() jackal.Severity {
	if r.severity != "" {
		return r.severity
	}
	return jackal.SeveritySafe
}

func (r *findRule) Name() string              { return r.name }
func (r *findRule) DisplayName() string       { return r.displayName }
func (r *findRule) Category() jackal.Category { return r.category }
func (r *findRule) Description() string       { return r.description }
func (r *findRule) Platforms() []string       { return r.platforms }

func (r *findRule) Scan(ctx context.Context, opts jackal.ScanOptions) ([]jackal.Finding, error) {
	var findings []jackal.Finding
	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	for _, searchPath := range r.searchPaths {
		root := jackal.ExpandPath(searchPath, homeDir)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		// Horus path: find directories by name from in-memory index.
		if opts.Manifest != nil {
			matches := opts.Manifest.FindDirsNamed(root, r.targetName, r.maxDepth)
			for _, path := range matches {
				// matchFile check
				if r.matchFile != "" {
					parentDir := filepath.Dir(path)
					if _, err := os.Stat(filepath.Join(parentDir, r.matchFile)); os.IsNotExist(err) {
						continue
					}
				}

				size, fileCount := opts.Manifest.DirSizeAndCount(path)
				if size == 0 {
					continue
				}

				// Get modtime from stat (needed for age filtering).
				info, err := os.Stat(path)
				if err != nil {
					// Permission denied — use manifest data.
					findings = append(findings, jackal.Finding{
						RuleName:    r.name,
						Category:    r.category,
						Description: r.displayName,
						Path:        path,
						SizeBytes:   size,
						FileCount:   fileCount,
						Severity:    r.effectiveSeverity(),
						IsDir:       true,
					})
					continue
				}

				// Age filter
				if r.minAgeDays > 0 {
					cutoff := time.Now().AddDate(0, 0, -r.minAgeDays)
					if info.ModTime().After(cutoff) {
						continue
					}
				}

				findings = append(findings, jackal.Finding{
					RuleName:     r.name,
					Category:     r.category,
					Description:  r.displayName,
					Path:         path,
					SizeBytes:    size,
					FileCount:    fileCount,
					Severity:     r.effectiveSeverity(),
					LastModified: info.ModTime(),
					IsDir:        true,
				})
			}
			continue // skip filesystem walk for this root
		}

		// Fallback: filesystem walk (no manifest available).
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if d.IsDir() && d.Name() != r.targetName {
				rel, _ := filepath.Rel(root, path)
				depth := strings.Count(rel, string(filepath.Separator)) + 1
				if depth > r.maxDepth {
					return filepath.SkipDir
				}
			}

			if d.IsDir() && d.Name() == r.targetName {
				if r.matchFile != "" {
					parentDir := filepath.Dir(path)
					if _, err := os.Stat(filepath.Join(parentDir, r.matchFile)); os.IsNotExist(err) {
						return filepath.SkipDir
					}
				}

				size, fileCount := dirSizeAndCount(path)
				if size == 0 {
					return filepath.SkipDir
				}

				info, _ := d.Info()

				findings = append(findings, jackal.Finding{
					RuleName:     r.name,
					Category:     r.category,
					Description:  r.displayName,
					Path:         path,
					SizeBytes:    size,
					FileCount:    fileCount,
					Severity:     r.effectiveSeverity(),
					LastModified: info.ModTime(),
					IsDir:        true,
				})

				return filepath.SkipDir
			}

			return nil
		})
		if err != nil && err != context.Canceled {
			continue
		}
	}

	return findings, nil
}

func (r *findRule) Clean(ctx context.Context, findings []jackal.Finding, opts jackal.CleanOptions) (*jackal.CleanResult, error) {
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
