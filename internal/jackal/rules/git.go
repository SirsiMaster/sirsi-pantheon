package rules

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/cleaner"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// ═══════════════════════════════════════════
// GIT — stale branches, large objects, worktrees, pack bloat
// ═══════════════════════════════════════════

// gitRepoRule scans all git repos found under search paths and runs
// git-specific analysis on each. This is the base for all git rules.
type gitRepoRule struct {
	name        string
	displayName string
	description string
	platforms   []string
	searchPaths []string
	maxDepth    int
	analyzeRepo func(ctx context.Context, repoPath string) []jackal.Finding
	cleanFn     func(ctx context.Context, finding jackal.Finding, dryRun bool) (int64, error) // nil = delete file
}

func (r *gitRepoRule) Name() string              { return r.name }
func (r *gitRepoRule) DisplayName() string       { return r.displayName }
func (r *gitRepoRule) Category() jackal.Category { return jackal.CategoryDev }
func (r *gitRepoRule) Description() string       { return r.description }
func (r *gitRepoRule) Platforms() []string       { return r.platforms }

func (r *gitRepoRule) Scan(ctx context.Context, opts jackal.ScanOptions) ([]jackal.Finding, error) {
	var findings []jackal.Finding
	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	for _, sp := range r.searchPaths {
		root := jackal.ExpandPath(sp, homeDir)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		repos := findGitRepos(root, r.maxDepth)
		for _, repo := range repos {
			findings = append(findings, r.analyzeRepo(ctx, repo)...)
		}
	}
	return findings, nil
}

func (r *gitRepoRule) Clean(ctx context.Context, findings []jackal.Finding, opts jackal.CleanOptions) (*jackal.CleanResult, error) {
	result := &jackal.CleanResult{}
	for _, f := range findings {
		if r.cleanFn != nil {
			freed, err := r.cleanFn(ctx, f, opts.DryRun)
			if err != nil {
				result.Skipped++
				result.Errors = append(result.Errors, err)
				continue
			}
			result.Cleaned++
			result.BytesFreed += freed
		} else {
			freed, err := cleaner.DeleteFile(f.Path, opts.DryRun, opts.UseTrash)
			if err != nil {
				result.Skipped++
				result.Errors = append(result.Errors, err)
				continue
			}
			result.Cleaned++
			result.BytesFreed += freed
		}
	}
	return result, nil
}

// findGitRepos walks a root directory looking for .git directories.
func findGitRepos(root string, maxDepth int) []string {
	var repos []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		depth := strings.Count(strings.TrimPrefix(path, root), string(filepath.Separator))
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if d.IsDir() && d.Name() == ".git" {
			repos = append(repos, filepath.Dir(path))
			return filepath.SkipDir
		}
		// Skip node_modules, vendor, .cache to speed up walk
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == ".cache") {
			return filepath.SkipDir
		}
		return nil
	})
	return repos
}

// gitCmd runs a git command in the given repo and returns stdout.
func gitCmd(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// ── Stale Branches Rule ──────────────────────────────────────────────

// NewStaleBranchesRule finds local branches tracking deleted remote branches.
func NewStaleBranchesRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "git_stale_branches",
		displayName: "Git Stale Branches",
		description: "Local branches tracking deleted remote branches ([gone])",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: analyzeStaleBranches,
		cleanFn:     cleanGitBranch,
	}
}

func analyzeStaleBranches(ctx context.Context, repo string) []jackal.Finding {
	// Fetch prune to update remote tracking state
	_ = exec.Command("git", "-C", repo, "fetch", "--prune").Run()

	out, err := gitCmd(repo, "branch", "-vv")
	if err != nil {
		return nil
	}

	var findings []jackal.Finding
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "* ") {
			continue // skip current branch
		}
		if strings.Contains(line, ": gone]") {
			parts := strings.Fields(line)
			if len(parts) < 1 {
				continue
			}
			branchName := parts[0]
			findings = append(findings, jackal.Finding{
				RuleName:    "git_stale_branches",
				Category:    jackal.CategoryDev,
				Description: fmt.Sprintf("Stale branch: %s", branchName),
				Path:        filepath.Join(repo, ".git", "refs", "heads", branchName),
				SizeBytes:   0, // Branches are refs, negligible size
				Severity:    jackal.SeveritySafe,
			})
		}
	}
	return findings
}

// ── Large Pack Files Rule ────────────────────────────────────────────

// NewGitLargeObjectsRule finds git repos with oversized pack files.
func NewGitLargeObjectsRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "git_large_objects",
		displayName: "Git Large Pack Files",
		description: "Git repos with oversized .git directories (pack files, old objects)",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: analyzeLargeGitDir,
		cleanFn:     cleanGitGC,
	}
}

func analyzeLargeGitDir(ctx context.Context, repo string) []jackal.Finding {
	gitDir := filepath.Join(repo, ".git")
	size, _ := dirSizeAndCount(gitDir)

	// Only flag if .git dir is > 200MB
	if size < 200*1024*1024 {
		return nil
	}

	repoName := filepath.Base(repo)
	return []jackal.Finding{{
		RuleName:    "git_large_objects",
		Category:    jackal.CategoryDev,
		Description: fmt.Sprintf("Large .git: %s", repoName),
		Path:        gitDir,
		SizeBytes:   size,
		Severity:    jackal.SeverityCaution,
		IsDir:       true,
	}}
}

// ── Orphaned Worktrees Rule ──────────────────────────────────────────

// NewGitOrphanedWorktreesRule finds worktrees whose working directory no longer exists.
func NewGitOrphanedWorktreesRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "git_orphaned_worktrees",
		displayName: "Git Orphaned Worktrees",
		description: "Git worktrees whose working directories no longer exist",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: analyzeOrphanedWorktrees,
		cleanFn:     cleanGitWorktreePrune,
	}
}

func analyzeOrphanedWorktrees(ctx context.Context, repo string) []jackal.Finding {
	out, err := gitCmd(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}

	var findings []jackal.Finding
	var currentPath string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		}
		if line == "prunable" && currentPath != "" {
			findings = append(findings, jackal.Finding{
				RuleName:    "git_orphaned_worktrees",
				Category:    jackal.CategoryDev,
				Description: fmt.Sprintf("Orphaned worktree: %s", filepath.Base(currentPath)),
				Path:        currentPath,
				Severity:    jackal.SeveritySafe,
			})
		}
	}
	return findings
}

// ── Untracked Build Artifacts Rule ───────────────────────────────────

// NewGitUntrackedArtifactsRule finds large untracked files in git repos.
func NewGitUntrackedArtifactsRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "git_untracked_artifacts",
		displayName: "Git Untracked Artifacts",
		description: "Large untracked files in git repos (build output, binaries, archives)",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: analyzeUntrackedArtifacts,
	}
}

func analyzeUntrackedArtifacts(ctx context.Context, repo string) []jackal.Finding {
	out, err := gitCmd(repo, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil
	}

	// Common build artifact extensions
	artifactExts := map[string]bool{
		".o": true, ".a": true, ".so": true, ".dylib": true, ".dll": true,
		".exe": true, ".bin": true, ".out": true, ".class": true, ".jar": true,
		".war": true, ".tar": true, ".gz": true, ".zip": true, ".tgz": true,
		".dmg": true, ".iso": true, ".img": true, ".wasm": true,
		".pyc": true, ".pyo": true, ".egg": true, ".whl": true,
	}

	var totalSize int64
	var count int

	for _, relPath := range strings.Split(out, "\n") {
		relPath = strings.TrimSpace(relPath)
		if relPath == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(relPath))
		if !artifactExts[ext] {
			continue
		}
		absPath := filepath.Join(repo, relPath)
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		// Only flag files > 1MB
		if info.Size() < 1024*1024 {
			continue
		}
		totalSize += info.Size()
		count++
	}

	if count == 0 {
		return nil
	}

	return []jackal.Finding{{
		RuleName:    "git_untracked_artifacts",
		Category:    jackal.CategoryDev,
		Description: fmt.Sprintf("Untracked artifacts in %s (%d files)", filepath.Base(repo), count),
		Path:        repo,
		SizeBytes:   totalSize,
		FileCount:   count,
		Severity:    jackal.SeverityCaution,
		IsDir:       true,
	}}
}

// ── Merged Branches Rule ─────────────────────────────────────────────

// NewGitMergedBranchesRule finds local branches that have been fully merged.
func NewGitMergedBranchesRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "git_merged_branches",
		displayName: "Git Merged Branches",
		description: "Local branches fully merged into main/master (safe to delete)",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: analyzeMergedBranches,
		cleanFn:     cleanGitBranch,
	}
}

func analyzeMergedBranches(ctx context.Context, repo string) []jackal.Finding {
	// Determine default branch
	defaultBranch := "main"
	if out, err := gitCmd(repo, "rev-parse", "--verify", "main"); err != nil || out == "" {
		if out2, err2 := gitCmd(repo, "rev-parse", "--verify", "master"); err2 == nil && out2 != "" {
			defaultBranch = "master"
		} else {
			return nil
		}
	}

	out, err := gitCmd(repo, "branch", "--merged", defaultBranch)
	if err != nil {
		return nil
	}

	var findings []jackal.Finding
	for _, line := range strings.Split(out, "\n") {
		branch := strings.TrimSpace(line)
		if branch == "" || strings.HasPrefix(branch, "* ") {
			continue
		}
		if branch == "main" || branch == "master" || branch == "develop" || branch == "dev" {
			continue
		}
		findings = append(findings, jackal.Finding{
			RuleName:    "git_merged_branches",
			Category:    jackal.CategoryDev,
			Description: fmt.Sprintf("Merged branch: %s", branch),
			Path:        filepath.Join(repo, ".git", "refs", "heads", branch),
			SizeBytes:   0,
			Severity:    jackal.SeveritySafe,
		})
	}
	return findings
}

// ── Git Rerere Cache Rule ────────────────────────────────────────────

// NewGitRerereCacheRule finds old git rerere (reuse recorded resolution) caches.
func NewGitRerereCacheRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "git_rerere_cache",
		displayName: "Git Rerere Cache",
		description: "Old conflict resolution recordings",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: func(ctx context.Context, repo string) []jackal.Finding {
			rerereDir := filepath.Join(repo, ".git", "rr-cache")
			size, count := dirSizeAndCount(rerereDir)
			if size < 1024 { // skip if trivially small
				return nil
			}
			return []jackal.Finding{{
				RuleName:    "git_rerere_cache",
				Category:    jackal.CategoryDev,
				Description: fmt.Sprintf("Rerere cache: %s", filepath.Base(repo)),
				Path:        rerereDir,
				SizeBytes:   size,
				FileCount:   count,
				Severity:    jackal.SeveritySafe,
				IsDir:       true,
			}}
		},
	}
}

// ── Git Reflog Bloat Rule ────────────────────────────────────────────

// NewGitReflogBloatRule finds repos with large reflogs.
func NewGitReflogBloatRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "git_reflog_bloat",
		displayName: "Git Reflog Bloat",
		description: "Repos with oversized reflogs consuming disk space",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		cleanFn:     cleanGitReflogExpire,
		analyzeRepo: func(ctx context.Context, repo string) []jackal.Finding {
			logsDir := filepath.Join(repo, ".git", "logs")
			size, count := dirSizeAndCount(logsDir)
			if size < 5*1024*1024 { // only flag if > 5MB
				return nil
			}
			return []jackal.Finding{{
				RuleName:    "git_reflog_bloat",
				Category:    jackal.CategoryDev,
				Description: fmt.Sprintf("Reflog bloat: %s", filepath.Base(repo)),
				Path:        logsDir,
				SizeBytes:   size,
				FileCount:   count,
				Severity:    jackal.SeverityCaution,
				IsDir:       true,
			}}
		},
	}
}

// ── Clean Functions (actual git remediations) ────────────────────────

// cleanGitBranch deletes a local git branch by parsing the repo and branch from the finding path.
func cleanGitBranch(ctx context.Context, f jackal.Finding, dryRun bool) (int64, error) {
	// Path is like /repo/.git/refs/heads/branch-name
	// Extract repo root and branch name
	path := f.Path
	refsIdx := strings.Index(path, "/.git/refs/heads/")
	if refsIdx < 0 {
		return 0, fmt.Errorf("cannot parse branch path: %s", path)
	}
	repo := path[:refsIdx]
	branch := path[refsIdx+len("/.git/refs/heads/"):]

	if dryRun {
		return 0, nil
	}
	_, err := gitCmd(repo, "branch", "-D", branch)
	return 0, err
}

// cleanGitGC runs git gc on a repo to compact the .git directory.
func cleanGitGC(ctx context.Context, f jackal.Finding, dryRun bool) (int64, error) {
	// Path is the .git directory — repo root is parent
	repo := filepath.Dir(f.Path)
	if dryRun {
		return f.SizeBytes / 2, nil // estimate 50% reduction
	}
	sizeBefore, _ := dirSizeAndCount(f.Path)
	err := exec.Command("git", "-C", repo, "gc", "--aggressive", "--prune=now").Run()
	if err != nil {
		return 0, fmt.Errorf("git gc: %w", err)
	}
	sizeAfter, _ := dirSizeAndCount(f.Path)
	freed := sizeBefore - sizeAfter
	if freed < 0 {
		freed = 0
	}
	return freed, nil
}

// cleanGitWorktreePrune prunes orphaned worktrees.
func cleanGitWorktreePrune(ctx context.Context, f jackal.Finding, dryRun bool) (int64, error) {
	// Find the repo that owns this worktree — walk up to find .git
	repo := f.Path
	for repo != "/" {
		if _, err := os.Stat(filepath.Join(repo, ".git")); err == nil {
			break
		}
		repo = filepath.Dir(repo)
	}
	if dryRun {
		return 0, nil
	}
	_, err := gitCmd(repo, "worktree", "prune")
	return 0, err
}

// cleanGitReflogExpire expires old reflog entries.
func cleanGitReflogExpire(ctx context.Context, f jackal.Finding, dryRun bool) (int64, error) {
	repo := filepath.Dir(filepath.Dir(f.Path)) // .git/logs -> .git -> repo
	if dryRun {
		return f.SizeBytes / 2, nil
	}
	sizeBefore, _ := dirSizeAndCount(f.Path)
	err := exec.Command("git", "-C", repo, "reflog", "expire", "--expire=30.days", "--all").Run()
	if err != nil {
		return 0, fmt.Errorf("git reflog expire: %w", err)
	}
	sizeAfter, _ := dirSizeAndCount(f.Path)
	freed := sizeBefore - sizeAfter
	if freed < 0 {
		freed = 0
	}
	return freed, nil
}

func defaultDevPaths() []string {
	return []string{
		"~/Development",
		"~/code",
		"~/projects",
		"~/src",
		"~/repos",
		"~/workspace",
		"~/work",
	}
}
