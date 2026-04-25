package rules

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// ═══════════════════════════════════════════
// CI/CD — GitHub Actions, build caches, runners, Docker images
// ═══════════════════════════════════════════

// NewGitHubActionsCacheRule scans local GitHub Actions runner caches.
func NewGitHubActionsCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "github_actions_cache",
		displayName: "GitHub Actions Cache",
		category:    jackal.CategoryDev,
		description: "Local GitHub Actions runner tool caches and artifacts",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/act",
			"~/actions-runner/_work/_tool",
			"~/actions-runner/_work/_temp",
			"/opt/hostedtoolcache",
		},
		minAgeDays: 7,
	}
}

// NewActRunnerCacheRule scans the local `act` runner cache (nektos/act).
func NewActRunnerCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "act_runner_cache",
		displayName: "Act Runner Cache",
		category:    jackal.CategoryDev,
		description: "nektos/act local runner Docker layer cache",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/actcache",
			"~/.act",
		},
	}
}

// NewBuildOutputRule finds stale build output directories.
func NewBuildOutputRule() jackal.ScanRule {
	return &findRule{
		name:        "build_output",
		displayName: "Build Output Dirs",
		category:    jackal.CategoryDev,
		description: "Stale build/dist/out directories from compilation",
		platforms:   []string{"darwin", "linux"},
		targetName:  "dist",
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		minAgeDays:  14,
		severity:    jackal.SeverityCaution,
	}
}

// NewNextJSCacheRule finds .next build caches.
func NewNextJSCacheRule() jackal.ScanRule {
	return &findRule{
		name:        "nextjs_cache",
		displayName: "Next.js Build Cache",
		category:    jackal.CategoryDev,
		description: ".next build directories (often 200MB+ each)",
		platforms:   []string{"darwin", "linux"},
		targetName:  ".next",
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		minAgeDays:  7,
		severity:    jackal.SeverityCaution,
	}
}

// NewTurborepoCache finds .turbo build caches.
func NewTurborepoCache() jackal.ScanRule {
	return &findRule{
		name:        "turborepo_cache",
		displayName: "Turborepo Cache",
		category:    jackal.CategoryDev,
		description: ".turbo build caches from monorepo builds",
		platforms:   []string{"darwin", "linux"},
		targetName:  ".turbo",
		searchPaths: defaultDevPaths(),
		maxDepth:    4,
		minAgeDays:  7,
		severity:    jackal.SeverityCaution,
	}
}

// NewDanglingDockerImagesRule finds dangling Docker images consuming disk.
func NewDanglingDockerImagesRule() jackal.ScanRule {
	return &dockerImageRule{
		name:        "docker_dangling_images",
		displayName: "Docker Dangling Images",
		description: "Untagged Docker images wasting disk space",
		platforms:   []string{"darwin", "linux"},
	}
}

type dockerImageRule struct {
	name        string
	displayName string
	description string
	platforms   []string
}

func (r *dockerImageRule) Name() string              { return r.name }
func (r *dockerImageRule) DisplayName() string       { return r.displayName }
func (r *dockerImageRule) Category() jackal.Category { return jackal.CategoryDev }
func (r *dockerImageRule) Description() string       { return r.description }
func (r *dockerImageRule) Platforms() []string       { return r.platforms }

func (r *dockerImageRule) Scan(ctx context.Context, opts jackal.ScanOptions) ([]jackal.Finding, error) {
	// Check if docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, nil
	}

	// List dangling images with their sizes
	out, err := exec.Command("docker", "images", "--filter", "dangling=true", "--format", "{{.Size}}").Output()
	if err != nil {
		return nil, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	count := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			count++
		}
	}

	if count == 0 {
		return nil, nil
	}

	// Get total size from docker system df
	var totalSize int64
	dfOut, err := exec.Command("docker", "system", "df", "--format", "{{.Type}}\t{{.Size}}\t{{.Reclaimable}}").Output()
	if err == nil {
		for _, line := range strings.Split(string(dfOut), "\n") {
			if strings.HasPrefix(line, "Images") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					totalSize = parseDockerSize(parts[2])
				}
			}
		}
	}
	if totalSize == 0 {
		totalSize = int64(count) * 100 * 1024 * 1024 // rough estimate: 100MB per image
	}

	return []jackal.Finding{{
		RuleName:    r.name,
		Category:    jackal.CategoryDev,
		Description: fmt.Sprintf("Docker: %d dangling images", count),
		Path:        "/var/run/docker.sock",
		SizeBytes:   totalSize,
		FileCount:   count,
		Severity:    jackal.SeveritySafe,
	}}, nil
}

func (r *dockerImageRule) Clean(ctx context.Context, findings []jackal.Finding, opts jackal.CleanOptions) (*jackal.CleanResult, error) {
	result := &jackal.CleanResult{}
	if opts.DryRun {
		for _, f := range findings {
			result.Cleaned += f.FileCount
			result.BytesFreed += f.SizeBytes
		}
		return result, nil
	}

	out, err := exec.Command("docker", "image", "prune", "-a", "-f").Output()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("docker prune: %w", err))
		return result, nil
	}

	// Parse "Total reclaimed space: X.XXX GB" from output
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "reclaimed") {
			result.Cleaned = findings[0].FileCount
			result.BytesFreed = findings[0].SizeBytes
		}
	}
	return result, nil
}

func parseDockerSize(s string) int64 {
	s = strings.TrimSpace(s)
	multiplier := int64(1)
	if strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "kB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "kB")
	}
	s = strings.TrimSpace(s)
	f := 0.0
	_, _ = fmt.Sscanf(s, "%f", &f)
	return int64(f * float64(multiplier))
}

// NewDockerBuildCacheRule finds Docker BuildKit cache.
func NewDockerBuildCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "docker_buildkit_cache",
		displayName: "Docker BuildKit Cache",
		category:    jackal.CategoryDev,
		description: "BuildKit build cache layers consuming disk space",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Containers/com.docker.docker/Data/docker/buildkit",
		},
	}
}

// ═══════════════════════════════════════════
// REPO HYGIENE — env files, lock files, dead configs, symlinks
// ═══════════════════════════════════════════

// NewEnvFileRule finds .env files that may contain secrets.
func NewEnvFileRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "env_files",
		displayName: "Env Files With Secrets",
		description: ".env files in repos that may contain API keys or credentials",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: analyzeEnvFiles,
		cleanFn:     cleanEnvFile,
	}
}

func analyzeEnvFiles(ctx context.Context, repo string) []jackal.Finding {
	// Find .env files that aren't gitignored
	out, err := gitCmd(repo, "ls-files", "--others", "--exclude-standard", "--", "*.env", ".env.*", ".env")
	if err != nil {
		return nil
	}

	var findings []jackal.Finding
	for _, relPath := range strings.Split(out, "\n") {
		relPath = strings.TrimSpace(relPath)
		if relPath == "" || relPath == ".env.example" || relPath == ".env.template" || relPath == ".env.sample" {
			continue
		}
		absPath := filepath.Join(repo, relPath)
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		// Read first few bytes to check for actual secrets
		data, _ := os.ReadFile(absPath)
		content := string(data)
		hasSecrets := false
		secretPatterns := []string{"API_KEY", "SECRET", "PASSWORD", "TOKEN", "PRIVATE_KEY",
			"AWS_ACCESS", "DATABASE_URL", "STRIPE_", "SENDGRID", "TWILIO"}
		for _, pat := range secretPatterns {
			if strings.Contains(strings.ToUpper(content), pat) {
				hasSecrets = true
				break
			}
		}
		if !hasSecrets {
			continue
		}

		findings = append(findings, jackal.Finding{
			RuleName:     "env_files",
			Category:     jackal.CategoryDev,
			Description:  fmt.Sprintf("Env file with secrets: %s", relPath),
			Path:         absPath,
			SizeBytes:    info.Size(),
			Severity:     jackal.SeverityWarning,
			LastModified: info.ModTime(),
		})
	}
	return findings
}

// cleanEnvFile adds the .env file to .gitignore so it won't be accidentally committed.
func cleanEnvFile(ctx context.Context, f jackal.Finding, dryRun bool) (int64, error) {
	// Find the repo root (walk up from the .env file)
	dir := filepath.Dir(f.Path)
	repoRoot := dir
	for repoRoot != "/" {
		if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err == nil {
			break
		}
		repoRoot = filepath.Dir(repoRoot)
	}

	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	envRelPath, _ := filepath.Rel(repoRoot, f.Path)

	if dryRun {
		return 0, nil // advisory action, no space freed
	}

	// Check if already in .gitignore
	existing, _ := os.ReadFile(gitignorePath)
	if strings.Contains(string(existing), envRelPath) {
		return 0, nil // already ignored
	}

	// Append to .gitignore
	file, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("open .gitignore: %w", err)
	}
	defer file.Close()

	entry := "\n# Added by Sirsi — secret file protection\n" + envRelPath + "\n"
	if _, err := file.WriteString(entry); err != nil {
		return 0, fmt.Errorf("write .gitignore: %w", err)
	}

	return 0, nil
}

// NewStaleLockFilesRule finds orphaned lock files.
func NewStaleLockFilesRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "stale_lock_files",
		displayName: "Stale Lock Files",
		description: "Orphaned .lock files that may block operations",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: analyzeStaleLockFiles,
	}
}

func analyzeStaleLockFiles(ctx context.Context, repo string) []jackal.Finding {
	lockFiles := []string{
		filepath.Join(repo, ".git", "index.lock"),
		filepath.Join(repo, ".git", "config.lock"),
		filepath.Join(repo, ".git", "HEAD.lock"),
		filepath.Join(repo, ".git", "refs", "heads", "*.lock"),
	}

	var findings []jackal.Finding
	for _, pattern := range lockFiles {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				continue
			}
			// Only flag if older than 1 hour (active git ops create temporary locks)
			if time.Since(info.ModTime()) < time.Hour {
				continue
			}
			findings = append(findings, jackal.Finding{
				RuleName:     "stale_lock_files",
				Category:     jackal.CategoryDev,
				Description:  fmt.Sprintf("Stale lock: %s", filepath.Base(match)),
				Path:         match,
				SizeBytes:    info.Size(),
				Severity:     jackal.SeverityCaution,
				LastModified: info.ModTime(),
			})
		}
	}
	return findings
}

// NewDeadSymlinksRule finds broken symlinks in dev directories.
func NewDeadSymlinksRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "dead_symlinks",
		displayName: "Dead Symlinks",
		description: "Broken symbolic links pointing to nonexistent targets",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		analyzeRepo: analyzeDeadSymlinks,
	}
}

func analyzeDeadSymlinks(ctx context.Context, repo string) []jackal.Finding {
	var findings []jackal.Finding
	_ = filepath.WalkDir(repo, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		// Skip .git, node_modules, vendor
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			if _, err := os.Stat(target); os.IsNotExist(err) {
				findings = append(findings, jackal.Finding{
					RuleName:    "dead_symlinks",
					Category:    jackal.CategoryDev,
					Description: fmt.Sprintf("Dead symlink → %s", filepath.Base(target)),
					Path:        path,
					Severity:    jackal.SeveritySafe,
				})
			}
		}
		return nil
	})
	return findings
}

// NewOversizedReposRule flags repos where the working tree is disproportionately large.
// Sirsi can compact via git gc and identify large untracked directories.
func NewOversizedReposRule() jackal.ScanRule {
	return &gitRepoRule{
		name:        "oversized_repos",
		displayName: "Oversized Repos",
		description: "Git repos with unusually large working trees (>2GB)",
		platforms:   []string{"darwin", "linux"},
		searchPaths: defaultDevPaths(),
		maxDepth:    2,
		cleanFn:     cleanOversizedRepo,
		analyzeRepo: func(ctx context.Context, repo string) []jackal.Finding {
			size, count := dirSizeAndCount(repo)
			if size < 2*1024*1024*1024 { // 2GB threshold
				return nil
			}
			return []jackal.Finding{{
				RuleName:    "oversized_repos",
				Category:    jackal.CategoryDev,
				Description: fmt.Sprintf("Oversized repo: %s", filepath.Base(repo)),
				Path:        repo,
				SizeBytes:   size,
				FileCount:   count,
				Severity:    jackal.SeverityCaution,
				IsDir:       true,
			}}
		},
	}
}

// cleanOversizedRepo compacts a repo: git gc, prune loose objects, repack.
func cleanOversizedRepo(ctx context.Context, f jackal.Finding, dryRun bool) (int64, error) {
	repo := f.Path
	gitDir := filepath.Join(repo, ".git")

	sizeBefore, _ := dirSizeAndCount(gitDir)

	if dryRun {
		// Estimate: git gc typically recovers 20-50% of .git size
		return sizeBefore / 3, nil
	}

	// Phase 1: git gc --aggressive
	if err := exec.Command("git", "-C", repo, "gc", "--aggressive", "--prune=now").Run(); err != nil {
		return 0, fmt.Errorf("git gc: %w", err)
	}

	// Phase 2: repack for maximum compression
	_ = exec.Command("git", "-C", repo, "repack", "-a", "-d", "--depth=250", "--window=250").Run()

	// Phase 3: prune unreachable objects
	_ = exec.Command("git", "-C", repo, "prune", "--expire=now").Run()

	sizeAfter, _ := dirSizeAndCount(gitDir)
	freed := sizeBefore - sizeAfter
	if freed < 0 {
		freed = 0
	}
	return freed, nil
}

// NewCoverageReportsRule finds old test coverage reports.
func NewCoverageReportsRule() jackal.ScanRule {
	return &findRule{
		name:        "coverage_reports",
		displayName: "Coverage Reports",
		category:    jackal.CategoryDev,
		description: "Old test coverage output directories",
		platforms:   []string{"darwin", "linux"},
		targetName:  "coverage",
		searchPaths: defaultDevPaths(),
		maxDepth:    4,
		minAgeDays:  7,
	}
}

// NewLogFilesRule finds oversized log files in dev projects.
func NewLogFilesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "dev_log_files",
		displayName: "Dev Log Files",
		category:    jackal.CategoryDev,
		description: "Large log files from development servers and build tools",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/Development/*/logs/*",
			"~/Development/*/*.log",
			"~/code/*/logs/*",
			"~/projects/*/logs/*",
		},
		minAgeDays: 3,
	}
}

// NewVenvRule finds Python virtual environments.
func NewVenvRule() jackal.ScanRule {
	return &findRule{
		name:        "python_venvs",
		displayName: "Python Virtual Envs",
		category:    jackal.CategoryDev,
		description: "Python venv/virtualenv directories (often 200MB+ each)",
		platforms:   []string{"darwin", "linux"},
		targetName:  "venv",
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		minAgeDays:  30,
		severity:    jackal.SeverityCaution,
	}
}

// NewDotEnvVenvRule finds .venv directories (alternate name).
func NewDotEnvVenvRule() jackal.ScanRule {
	return &findRule{
		name:        "python_dot_venvs",
		displayName: "Python .venv Dirs",
		category:    jackal.CategoryDev,
		description: ".venv directories from Python projects",
		platforms:   []string{"darwin", "linux"},
		targetName:  ".venv",
		searchPaths: defaultDevPaths(),
		maxDepth:    3,
		minAgeDays:  30,
		severity:    jackal.SeverityCaution,
	}
}
