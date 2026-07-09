package osiris

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// clearGitEnv strips every GIT_* variable for this test. The Ma'at pre-push
// hook runs `go test` WITH GIT_DIR/GIT_WORK_TREE set by git itself — leaked
// into subprocesses, they make every git command target the REAL repo
// regardless of cmd.Dir (repo issue #99's class). Un-scrubbed, this exact
// suite committed checkpoint commits onto its own feature branch from inside
// the push gate (2026-07-09) — the test hijacked the repo it ships in.
func clearGitEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			key, _, _ := strings.Cut(kv, "=")
			t.Setenv(key, "") // registers restore
			os.Unsetenv(key)
		}
	}
}

// tempRepo builds a real throwaway git repo (never the developer's).
func tempRepo(t *testing.T) string {
	t.Helper()
	clearGitEnv(t)
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "osiris@test"},
		{"config", "user.name", "osiris-test"},
		{"commit", "--allow-empty", "-q", "-m", "root"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestCommitCheckpointSecuresDirtyTree(t *testing.T) {
	dir := tempRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "risky.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := CommitCheckpoint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Committed || res.FilesCommitted != 1 || res.Hash == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
	// The tree must actually be clean afterwards — the lever RESOLVES the finding.
	after, err := CommitCheckpoint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if after.Committed {
		t.Fatalf("second checkpoint must be a clean no-op, got %+v", after)
	}
	if !strings.Contains(after.Message, "clean") {
		t.Fatalf("no-op must say the tree is clean: %q", after.Message)
	}
}

func TestCommitCheckpointRefusesNonRepo(t *testing.T) {
	if _, err := CommitCheckpoint(t.TempDir()); err == nil {
		t.Fatal("a non-repo must be an explicit error, not a silent no-op")
	}
}
