package rules

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// initGitRepo creates a minimal git repo in a temp directory.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = gitTestEnv() // #99 class: never inherit GIT_DIR/GIT_WORK_TREE
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644)
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// ── findGitRepos ─────────────────────────────────────────────────────

func TestFindGitRepos(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create 2 repos at different depths
	repo1 := filepath.Join(root, "project-a")
	repo2 := filepath.Join(root, "deep", "nested", "project-b")
	for _, p := range []string{repo1, repo2} {
		os.MkdirAll(filepath.Join(p, ".git"), 0o755)
	}

	repos := findGitRepos(root, 4)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
}

func TestFindGitRepos_RespectsMaxDepth(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "a", "b", "c", "d", ".git"), 0o755)

	repos := findGitRepos(root, 2)
	if len(repos) != 0 {
		t.Fatalf("expected 0 repos at depth 2, got %d", len(repos))
	}

	repos = findGitRepos(root, 5)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo at depth 5, got %d", len(repos))
	}
}

func TestFindGitRepos_SkipsNodeModules(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "node_modules", "pkg", ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "real-project", ".git"), 0o755)

	repos := findGitRepos(root, 4)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo (skipping node_modules), got %d", len(repos))
	}
}

// ── Merged Branches Rule ─────────────────────────────────────────────

func TestGitMergedBranches(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = gitTestEnv() // #99 class: never inherit GIT_DIR/GIT_WORK_TREE
		cmd.CombinedOutput()
	}

	// Create and merge a feature branch
	run("checkout", "-b", "feature-done")
	os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("done"), 0o644)
	run("add", ".")
	run("commit", "-m", "feature")
	run("checkout", "main")
	run("merge", "feature-done")

	findings := analyzeMergedBranches(context.Background(), repo)
	found := false
	for _, f := range findings {
		if f.Description == "Merged branch: feature-done" {
			found = true
			if f.Severity != jackal.SeveritySafe {
				t.Errorf("merged branch severity = %q, want safe", f.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected to find merged branch 'feature-done', got %d findings", len(findings))
	}
}

func TestGitMergedBranches_SkipsProtected(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	// main itself should not appear as a merged branch
	findings := analyzeMergedBranches(context.Background(), repo)
	for _, f := range findings {
		if f.Description == "Merged branch: main" {
			t.Error("main should not be listed as merged")
		}
	}
}

// ── Large .git Directory Rule ────────────────────────────────────────

func TestGitLargeObjects_SmallRepo(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	findings := analyzeLargeGitDir(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("small repo should have 0 findings, got %d", len(findings))
	}
}

func TestGitLargeObjects_Threshold(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git", "objects", "pack")
	os.MkdirAll(gitDir, 0o755)

	// Write a 201MB fake pack file to exceed the 200MB threshold
	f, _ := os.Create(filepath.Join(gitDir, "fake.pack"))
	f.Truncate(201 * 1024 * 1024)
	f.Close()

	findings := analyzeLargeGitDir(context.Background(), repo)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for 201MB .git, got %d", len(findings))
	}
	if findings[0].Severity != jackal.SeverityCaution {
		t.Errorf("severity = %q, want caution", findings[0].Severity)
	}
}

// ── Orphaned Worktrees Rule ──────────────────────────────────────────

func TestGitOrphanedWorktrees_NoWorktrees(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	findings := analyzeOrphanedWorktrees(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("repo with no worktrees should have 0 findings, got %d", len(findings))
	}
}

// ── Untracked Artifacts Rule ─────────────────────────────────────────

func TestGitUntrackedArtifacts_FindsLargeBinaries(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	// Create a large .o file (2MB)
	objFile := filepath.Join(repo, "build.o")
	f, _ := os.Create(objFile)
	f.Truncate(2 * 1024 * 1024)
	f.Close()

	findings := analyzeUntrackedArtifacts(context.Background(), repo)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for 2MB .o file, got %d", len(findings))
	}
	if findings[0].SizeBytes < 2*1024*1024 {
		t.Errorf("size = %d, want >= 2MB", findings[0].SizeBytes)
	}
}

func TestGitUntrackedArtifacts_IgnoresSmallFiles(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)

	// Create a small .o file (500 bytes — under 1MB threshold)
	os.WriteFile(filepath.Join(repo, "tiny.o"), make([]byte, 500), 0o644)

	findings := analyzeUntrackedArtifacts(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for tiny .o, got %d", len(findings))
	}
}

// ── Rerere Cache Rule ────────────────────────────────────────────────

func TestGitRerereCache(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	rrDir := filepath.Join(repo, ".git", "rr-cache", "abc123")
	os.MkdirAll(rrDir, 0o755)
	os.WriteFile(filepath.Join(rrDir, "postimage"), make([]byte, 4096), 0o644)

	rule := NewGitRerereCacheRule().(*gitRepoRule)
	findings := rule.analyzeRepo(context.Background(), repo)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for rerere cache, got %d", len(findings))
	}
}

func TestGitRerereCache_SkipsEmpty(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, ".git", "rr-cache"), 0o755)

	rule := NewGitRerereCacheRule().(*gitRepoRule)
	findings := rule.analyzeRepo(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for empty rerere, got %d", len(findings))
	}
}

// ── Reflog Bloat Rule ────────────────────────────────────────────────

func TestGitReflogBloat_UnderThreshold(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	logsDir := filepath.Join(repo, ".git", "logs")
	os.MkdirAll(logsDir, 0o755)
	os.WriteFile(filepath.Join(logsDir, "HEAD"), make([]byte, 1024), 0o644)

	rule := NewGitReflogBloatRule().(*gitRepoRule)
	findings := rule.analyzeRepo(context.Background(), repo)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for 1KB reflog, got %d", len(findings))
	}
}

func TestGitReflogBloat_OverThreshold(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	logsDir := filepath.Join(repo, ".git", "logs")
	os.MkdirAll(logsDir, 0o755)
	f, _ := os.Create(filepath.Join(logsDir, "HEAD"))
	f.Truncate(6 * 1024 * 1024) // 6MB
	f.Close()

	rule := NewGitReflogBloatRule().(*gitRepoRule)
	findings := rule.analyzeRepo(context.Background(), repo)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for 6MB reflog, got %d", len(findings))
	}
}

// ── Constructor Validation ───────────────────────────────────────────

func TestGitRules_Constructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]func() jackal.ScanRule{
		"git_stale_branches":      NewStaleBranchesRule,
		"git_merged_branches":     NewGitMergedBranchesRule,
		"git_large_objects":       NewGitLargeObjectsRule,
		"git_orphaned_worktrees":  NewGitOrphanedWorktreesRule,
		"git_untracked_artifacts": NewGitUntrackedArtifactsRule,
		"git_rerere_cache":        NewGitRerereCacheRule,
		"git_reflog_bloat":        NewGitReflogBloatRule,
	}

	for name, ctor := range constructors {
		rule := ctor()
		if rule.Name() != name {
			t.Errorf("%s: Name() = %q", name, rule.Name())
		}
		if rule.DisplayName() == "" {
			t.Errorf("%s: empty DisplayName", name)
		}
		if rule.Category() != jackal.CategoryDev {
			t.Errorf("%s: Category() = %q, want dev", name, rule.Category())
		}
		if rule.Description() == "" {
			t.Errorf("%s: empty Description", name)
		}
		if len(rule.Platforms()) == 0 {
			t.Errorf("%s: no platforms", name)
		}
	}
}
