// Package jackal — artifact purge scanner.
//
// ScanArtifacts walks project directories looking for known build artifact
// directories (node_modules, target, venv, etc.) and reports their sizes.
// PurgeArtifacts removes selected artifacts, freeing disk space.
package jackal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/cleaner"
	"github.com/SirsiMaster/sirsi-pantheon/internal/oplog"
)

// ArtifactType identifies the kind of build artifact directory.
type ArtifactType string

const (
	ArtifactNodeModules ArtifactType = "node_modules"
	ArtifactTarget      ArtifactType = "target"      // Rust
	ArtifactBuild       ArtifactType = "build"       // generic
	ArtifactDist        ArtifactType = "dist"        // frontend
	ArtifactVenv        ArtifactType = "venv"        // Python
	ArtifactDotVenv     ArtifactType = ".venv"       // Python (alt)
	ArtifactDotBuild    ArtifactType = ".build"      // Swift
	ArtifactPods        ArtifactType = "Pods"        // iOS
	ArtifactDerived     ArtifactType = "DerivedData" // Xcode
)

// knownArtifacts maps directory names to their artifact type.
var knownArtifacts = map[string]ArtifactType{
	"node_modules": ArtifactNodeModules,
	"target":       ArtifactTarget,
	"build":        ArtifactBuild,
	"dist":         ArtifactDist,
	"venv":         ArtifactVenv,
	".venv":        ArtifactDotVenv,
	".build":       ArtifactDotBuild,
	"Pods":         ArtifactPods,
	"DerivedData":  ArtifactDerived,
}

// artifactConfirmers are directories that must live alongside a project
// marker to be considered an artifact (avoids false positives).
var artifactConfirmers = map[ArtifactType][]string{
	ArtifactTarget:   {"Cargo.toml", "Cargo.lock"},
	ArtifactBuild:    {"package.json", "build.gradle", "CMakeLists.txt", "Makefile"},
	ArtifactDist:     {"package.json", "webpack.config.js", "vite.config.js", "vite.config.ts"},
	ArtifactDotBuild: {"Package.swift"},
}

// recentThreshold is the age below which an artifact is marked Recent.
const recentThreshold = 7 * 24 * time.Hour

// ProjectArtifact represents a single discovered artifact directory.
type ProjectArtifact struct {
	ProjectName string // parent directory name
	ProjectPath string // full path to project
	ArtifactDir string // full path to artifact dir
	Type        ArtifactType
	Size        int64
	ModTime     time.Time
	IsRecent    bool // modified within 7 days
}

// PurgeResult holds the aggregated results of an artifact scan.
type PurgeResult struct {
	Artifacts []ProjectArtifact // sorted by size descending
	TotalSize int64
	ScanTime  time.Duration
}

// ScanArtifacts walks the given root directories looking for known
// build artifact directories. It returns them sorted by size descending.
// Roots that do not exist are silently skipped.
func ScanArtifacts(roots []string) (*PurgeResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var (
		mu        sync.Mutex
		artifacts []ProjectArtifact
		wg        sync.WaitGroup
	)

	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		wg.Add(1)
		go func(root string) {
			defer wg.Done()
			found := scanRoot(ctx, root)
			mu.Lock()
			artifacts = append(artifacts, found...)
			mu.Unlock()
		}(root)
	}

	wg.Wait()

	// Deduplicate: if a parent artifact contains a child (e.g. node_modules
	// inside node_modules), keep only the outermost.
	artifacts = deduplicateArtifacts(artifacts)

	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Size > artifacts[j].Size
	})

	var total int64
	for _, a := range artifacts {
		total += a.Size
	}

	return &PurgeResult{
		Artifacts: artifacts,
		TotalSize: total,
		ScanTime:  time.Since(start),
	}, nil
}

// scanRoot walks a single root directory looking for artifact dirs.
func scanRoot(ctx context.Context, root string) []ProjectArtifact {
	var results []ProjectArtifact

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden directories at root level (except known artifacts)
		if strings.HasPrefix(name, ".") && knownArtifacts[name] == "" {
			continue
		}

		projectDir := filepath.Join(root, name)
		// Check if this root-level dir itself is an artifact (unlikely but handle it)
		if artType, ok := knownArtifacts[name]; ok {
			if confirmedArtifact(root, artType) {
				if a, ok := measureArtifact(ctx, root, projectDir, artType); ok {
					results = append(results, a)
				}
			}
			continue
		}

		// Walk one level into the project looking for artifact dirs
		found := scanProject(ctx, projectDir, 0)
		results = append(results, found...)
	}

	return results
}

// maxScanDepth limits how deep we recurse into project trees.
const maxScanDepth = 3

// scanProject looks for artifact directories inside a project directory.
func scanProject(ctx context.Context, projectDir string, depth int) []ProjectArtifact {
	if depth > maxScanDepth {
		return nil
	}

	select {
	case <-ctx.Done():
		return nil
	default:
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil
	}

	var results []ProjectArtifact

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Never descend into .git
		if name == ".git" {
			continue
		}

		childPath := filepath.Join(projectDir, name)

		if artType, ok := knownArtifacts[name]; ok {
			if confirmedArtifact(projectDir, artType) {
				if a, ok := measureArtifact(ctx, projectDir, childPath, artType); ok {
					results = append(results, a)
				}
			}
			continue // don't recurse into artifact dirs
		}

		// Skip hidden dirs
		if strings.HasPrefix(name, ".") {
			continue
		}

		// Recurse into subdirectories (monorepos, workspaces)
		sub := scanProject(ctx, childPath, depth+1)
		results = append(results, sub...)
	}

	return results
}

// confirmedArtifact checks whether the parent directory contains a project
// marker that confirms this is a real artifact (not a coincidental dir name).
// Types without confirmers (node_modules, venv, Pods, DerivedData) are always confirmed.
func confirmedArtifact(parentDir string, artType ArtifactType) bool {
	markers, needsConfirm := artifactConfirmers[artType]
	if !needsConfirm {
		return true
	}
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(parentDir, marker)); err == nil {
			return true
		}
	}
	return false
}

// measureArtifact calculates the size and recency of an artifact directory.
func measureArtifact(ctx context.Context, projectDir, artifactDir string, artType ArtifactType) (ProjectArtifact, bool) {
	var totalSize int64
	var latestMod time.Time

	err := filepath.Walk(artifactDir, func(path string, info os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err != nil {
			return nil // skip unreadable entries
		}
		// Skip .git inside artifacts
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			totalSize += info.Size()
			if info.ModTime().After(latestMod) {
				latestMod = info.ModTime()
			}
		}
		return nil
	})

	if err != nil || totalSize == 0 {
		return ProjectArtifact{}, false
	}

	isRecent := time.Since(latestMod) < recentThreshold

	return ProjectArtifact{
		ProjectName: filepath.Base(projectDir),
		ProjectPath: projectDir,
		ArtifactDir: artifactDir,
		Type:        artType,
		Size:        totalSize,
		ModTime:     latestMod,
		IsRecent:    isRecent,
	}, true
}

// deduplicateArtifacts removes artifacts whose ArtifactDir is a subdirectory
// of another artifact's ArtifactDir (keeps the outermost).
func deduplicateArtifacts(artifacts []ProjectArtifact) []ProjectArtifact {
	if len(artifacts) <= 1 {
		return artifacts
	}

	// Sort by path length (shortest first = outermost)
	sort.Slice(artifacts, func(i, j int) bool {
		return len(artifacts[i].ArtifactDir) < len(artifacts[j].ArtifactDir)
	})

	var kept []ProjectArtifact
	for _, a := range artifacts {
		nested := false
		for _, k := range kept {
			if strings.HasPrefix(a.ArtifactDir, k.ArtifactDir+string(os.PathSeparator)) {
				nested = true
				break
			}
		}
		if !nested {
			kept = append(kept, a)
		}
	}
	return kept
}

// PurgeArtifacts deletes the given artifact directories via the cleaner safety
// layer. Every path is validated against protected-path rules before deletion.
// If useTrash is true, uses DeleteFileReversible which refuses to permanently
// delete when the platform has no trash support. Pass useTrash=false only for
// explicit permanent deletion with user confirmation.
// Returns a CleanResult compatible with the existing rendering system.
func PurgeArtifacts(artifacts []ProjectArtifact, useTrash bool) (*CleanResult, error) {
	result := &CleanResult{}

	for _, a := range artifacts {
		var freed int64
		var err error
		if useTrash {
			freed, err = cleaner.DeleteFileReversible(a.ArtifactDir, false)
		} else {
			freed, err = cleaner.DeleteFile(a.ArtifactDir, false, false)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("purge %s: %w", a.ArtifactDir, err))
			result.Skipped++
			continue
		}
		result.BytesFreed += freed
		result.Cleaned++
		oplog.Log("purge", a.ArtifactDir, freed)
	}

	return result, nil
}

// moveToTrash is removed — all deletion now goes through cleaner.DeleteFile
// which validates paths and uses platform.Current().MoveToTrash() with no
// permanent-delete fallback. See internal/cleaner/safety.go.

// DefaultPurgeRoots returns the default directories to scan for artifacts.
// Only directories that exist are returned.
func DefaultPurgeRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	candidates := []string{
		filepath.Join(home, "Development"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "Documents"),
	}

	var roots []string
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			roots = append(roots, c)
		}
	}
	return roots
}
