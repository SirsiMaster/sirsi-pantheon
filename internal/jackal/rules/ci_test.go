package rules

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// ── CI/CD Cache Rules ────────────────────────────────────────────────

func TestGitHubActionsCache_FindsCacheDir(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cacheDir := filepath.Join(home, ".cache", "act")
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "layer.tar"), make([]byte, 4096), 0o644)

	// Set mtime to old so it passes the 7-day age filter
	old := time.Now().Add(-14 * 24 * time.Hour)
	os.Chtimes(cacheDir, old, old)

	rule := NewGitHubActionsCacheRule()
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings for .cache/act, got 0")
	}
}

func TestActRunnerCache_FindsCacheDir(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cacheDir := filepath.Join(home, ".act")
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "config.json"), make([]byte, 256), 0o644)

	rule := NewActRunnerCacheRule()
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings for .act, got 0")
	}
}

// ── Build Output Rules ───────────────────────────────────────────────

func TestBuildOutputRule_FindsDistDir(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	projDir := filepath.Join(home, "Development", "myproject")
	distDir := filepath.Join(projDir, "dist")
	os.MkdirAll(distDir, 0o755)
	os.WriteFile(filepath.Join(distDir, "bundle.js"), make([]byte, 8192), 0o644)

	// Set mtime to old so it passes age filter
	old := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(distDir, old, old)

	rule := NewBuildOutputRule()
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings for dist/, got 0")
	}
}

func TestNextJSCache_FindsNextDir(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	projDir := filepath.Join(home, "Development", "webapp")
	nextDir := filepath.Join(projDir, ".next")
	os.MkdirAll(nextDir, 0o755)
	os.WriteFile(filepath.Join(nextDir, "BUILD_ID"), make([]byte, 64), 0o644)

	old := time.Now().Add(-14 * 24 * time.Hour)
	os.Chtimes(nextDir, old, old)

	rule := NewNextJSCacheRule()
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings for .next/, got 0")
	}
}

func TestTurborepoCache_FindsTurboDir(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	projDir := filepath.Join(home, "Development", "monorepo")
	turboDir := filepath.Join(projDir, ".turbo")
	os.MkdirAll(turboDir, 0o755)
	os.WriteFile(filepath.Join(turboDir, "cache.json"), make([]byte, 1024), 0o644)

	old := time.Now().Add(-14 * 24 * time.Hour)
	os.Chtimes(turboDir, old, old)

	rule := NewTurborepoCache()
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings for .turbo/, got 0")
	}
}

// ── Docker Rules ─────────────────────────────────────────────────────

func TestDockerDanglingImages_NoDockerSkips(t *testing.T) {
	t.Parallel()
	// If docker is not installed, the rule should return nil (not error)
	rule := NewDanglingDockerImagesRule()
	_, err := rule.Scan(context.Background(), jackal.ScanOptions{})
	if err != nil {
		t.Fatalf("expected nil error when docker unavailable, got: %v", err)
	}
}

func TestDockerBuildCacheRule_Constructor(t *testing.T) {
	t.Parallel()
	rule := NewDockerBuildCacheRule()
	if rule.Name() != "docker_buildkit_cache" {
		t.Errorf("Name() = %q", rule.Name())
	}
	if rule.Category() != jackal.CategoryDev {
		t.Errorf("Category() = %q", rule.Category())
	}
}

// ── Repo Hygiene Rules ───────────────────────────────────────────────

func TestEnvFiles_FindsSecretsInEnv(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	// Create a .env file with secrets
	envContent := "DATABASE_URL=postgres://user:pass@host/db\nAPI_KEY=sk-123456\n"
	os.WriteFile(filepath.Join(repo, ".env"), []byte(envContent), 0o644)

	findings := analyzeEnvFiles(context.Background(), repo)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for .env with API_KEY, got %d", len(findings))
	}
	if findings[0].Severity != jackal.SeverityWarning {
		t.Errorf("severity = %q, want warning", findings[0].Severity)
	}
}

func TestEnvFiles_SkipsExample(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	// .env.example should be skipped even if it has secret-like content
	os.WriteFile(filepath.Join(repo, ".env.example"), []byte("API_KEY=your_key_here\n"), 0o644)

	findings := analyzeEnvFiles(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for .env.example, got %d", len(findings))
	}
}

func TestEnvFiles_SkipsNoSecrets(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	// .env without secret patterns
	os.WriteFile(filepath.Join(repo, ".env"), []byte("NODE_ENV=production\nPORT=3000\n"), 0o644)

	findings := analyzeEnvFiles(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for .env without secrets, got %d", len(findings))
	}
}

func TestDeadSymlinks_FindsBroken(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	// Create a symlink pointing to nonexistent target
	os.Symlink("/nonexistent/target/file", filepath.Join(repo, "broken-link"))

	findings := analyzeDeadSymlinks(context.Background(), repo)
	if len(findings) != 1 {
		t.Fatalf("expected 1 dead symlink finding, got %d", len(findings))
	}
}

func TestDeadSymlinks_SkipsValid(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	// Create a valid symlink
	target := filepath.Join(repo, "real-file.txt")
	os.WriteFile(target, []byte("content"), 0o644)
	os.Symlink(target, filepath.Join(repo, "valid-link"))

	findings := analyzeDeadSymlinks(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for valid symlink, got %d", len(findings))
	}
}

func TestStaleLockFiles_FindsOldLocks(t *testing.T) {
	// NOT t.Parallel: this test does a real git init/commit (initGitRepo) and then
	// hand-sets .git/index.lock's mtime. Under the parallel batch's CPU contention a
	// lagging git op can re-touch index.lock AFTER our Chtimes, making the "stale"
	// lock look fresh (<1h) → 0 findings → heisenbug. Run serially for determinism.
	repo := initGitRepo(t)

	// Create a stale lock file (2 hours old)
	lockFile := filepath.Join(repo, ".git", "index.lock")
	os.WriteFile(lockFile, []byte("locked"), 0o644)
	old := time.Now().Add(-2 * time.Hour)
	os.Chtimes(lockFile, old, old)

	findings := analyzeStaleLockFiles(context.Background(), repo)
	if len(findings) != 1 {
		t.Fatalf("expected 1 stale lock finding, got %d", len(findings))
	}
}

func TestStaleLockFiles_SkipsRecent(t *testing.T) {
	// NOT t.Parallel: same git-lock-mtime race as TestStaleLockFiles_FindsOldLocks
	// (real git init/commit + manual .git/index.lock manipulation). Serial = deterministic.
	repo := initGitRepo(t)

	// Create a fresh lock file (just now — active git operation)
	lockFile := filepath.Join(repo, ".git", "index.lock")
	os.WriteFile(lockFile, []byte("locked"), 0o644)

	findings := analyzeStaleLockFiles(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for fresh lock, got %d", len(findings))
	}
}

func TestOversizedRepos_UnderThreshold(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	rule := NewOversizedReposRule().(*gitRepoRule)
	findings := rule.analyzeRepo(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for small repo, got %d", len(findings))
	}
}

func TestVenvRule_Constructor(t *testing.T) {
	t.Parallel()
	rule := NewVenvRule()
	if rule.Name() != "python_venvs" {
		t.Errorf("Name() = %q", rule.Name())
	}
	if rule.Category() != jackal.CategoryDev {
		t.Errorf("Category() = %q", rule.Category())
	}
}

func TestCoverageReportsRule_Constructor(t *testing.T) {
	t.Parallel()
	rule := NewCoverageReportsRule()
	if rule.Name() != "coverage_reports" {
		t.Errorf("Name() = %q", rule.Name())
	}
}

// ── Constructor Validation ───────────────────────────────────────────

func TestCIRules_Constructors(t *testing.T) {
	t.Parallel()
	rules := []struct {
		name string
		ctor func() jackal.ScanRule
	}{
		{"github_actions_cache", NewGitHubActionsCacheRule},
		{"act_runner_cache", NewActRunnerCacheRule},
		{"build_output", NewBuildOutputRule},
		{"nextjs_cache", NewNextJSCacheRule},
		{"turborepo_cache", NewTurborepoCache},
		{"docker_dangling_images", NewDanglingDockerImagesRule},
		{"docker_buildkit_cache", NewDockerBuildCacheRule},
		{"env_files", NewEnvFileRule},
		{"stale_lock_files", NewStaleLockFilesRule},
		{"dead_symlinks", NewDeadSymlinksRule},
		{"oversized_repos", NewOversizedReposRule},
		{"coverage_reports", NewCoverageReportsRule},
		{"dev_log_files", NewLogFilesRule},
		{"python_venvs", NewVenvRule},
		{"python_dot_venvs", NewDotEnvVenvRule},
	}

	for _, tt := range rules {
		rule := tt.ctor()
		if rule.Name() != tt.name {
			t.Errorf("%s: Name() = %q", tt.name, rule.Name())
		}
		if rule.DisplayName() == "" {
			t.Errorf("%s: empty DisplayName", tt.name)
		}
		if rule.Category() == "" {
			t.Errorf("%s: empty Category", tt.name)
		}
		if rule.Description() == "" {
			t.Errorf("%s: empty Description", tt.name)
		}
	}
}

// ── parseDockerSize ──────────────────────────────────────────────────

func TestParseDockerSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int64
	}{
		{"1.5GB", 1610612736},
		{"500MB", 524288000},
		{"100kB", 102400},
		{"0GB", 0},
	}

	for _, tt := range tests {
		got := parseDockerSize(tt.input)
		// Allow 1% tolerance for floating point
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if diff > tt.want/100+1 {
			t.Errorf("parseDockerSize(%q) = %d, want ~%d", tt.input, got, tt.want)
		}
	}
}
