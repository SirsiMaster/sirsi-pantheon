package cleaner

// Coverage-lift tests for the safety-critical deletion paths (Tier A).
// All filesystem activity is confined to t.TempDir(); the platform layer is
// swapped for platform.Mock so no real trash/delete syscalls escape the test.
// NOTE: none of these tests use t.Parallel() — they swap the package-level
// platform singleton and HOME, which must not race (PRs #129/#131 pattern).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ── DeleteFileReversible ─────────────────────────────────────────────────

func TestDeleteFileReversible_ProtectedPath(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	size, err := DeleteFileReversible("/usr/bin/ls", false)
	if err == nil {
		t.Fatal("expected error for protected path")
	}
	if size != 0 {
		t.Errorf("size = %d, want 0 for blocked path", size)
	}
}

func TestDeleteFileReversible_NonExistent(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	size, err := DeleteFileReversible(filepath.Join(t.TempDir(), "gone.txt"), false)
	if err != nil {
		t.Fatalf("nonexistent file should be a no-op, got: %v", err)
	}
	if size != 0 {
		t.Errorf("size = %d, want 0 for nonexistent file", size)
	}
}

func TestDeleteFileReversible_StatError(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	// A path whose parent is a regular file yields ENOTDIR from Lstat,
	// which is not IsNotExist — the "cannot stat" branch.
	dir := t.TempDir()
	parentFile := filepath.Join(dir, "regular-file.txt")
	if err := os.WriteFile(parentFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := DeleteFileReversible(filepath.Join(parentFile, "child"), false)
	if err == nil {
		t.Fatal("expected stat error for path under a regular file")
	}
	if !strings.Contains(err.Error(), "cannot stat") {
		t.Errorf("error = %v, want 'cannot stat'", err)
	}
}

func TestDeleteFileReversible_DryRunFile(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	dir := t.TempDir()
	path := filepath.Join(dir, "dry.txt")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	size, err := DeleteFileReversible(path, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Error("dry run must not touch the file")
	}
}

func TestDeleteFileReversible_DryRunDirUsesDirSize(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	dir := t.TempDir()
	sub := filepath.Join(dir, "victim")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(sub, "a.txt"), []byte("12345"), 0o644)
	os.WriteFile(filepath.Join(sub, "b.txt"), []byte("6789"), 0o644)

	size, err := DeleteFileReversible(sub, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if size != 9 {
		t.Errorf("size = %d, want 9 (recursive dir size)", size)
	}
	if _, statErr := os.Stat(sub); statErr != nil {
		t.Error("dry run must not touch the directory")
	}
}

func TestDeleteFileReversible_NoTrashRefuses(t *testing.T) {
	m := &platform.Mock{NoTrash: true}
	platform.Set(m)
	defer platform.Reset()

	dir := t.TempDir()
	path := filepath.Join(dir, "keep-me.txt")
	if err := os.WriteFile(path, []byte("precious"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	size, err := DeleteFileReversible(path, false)
	if err == nil {
		t.Fatal("expected refusal when platform has no trash support")
	}
	if size != 0 {
		t.Errorf("size = %d, want 0 on refusal", size)
	}
	if len(m.TrashCalls) != 0 {
		t.Errorf("MoveToTrash must not be called, got %v", m.TrashCalls)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Error("file must remain untouched when deletion is refused")
	}
}

func TestDeleteFileReversible_TrashSuccess(t *testing.T) {
	m := &platform.Mock{}
	platform.Set(m)
	defer platform.Reset()

	dir := t.TempDir()
	path := filepath.Join(dir, "trash-me.txt")
	content := []byte("bytes to free")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	size, err := DeleteFileReversible(path, false)
	if err != nil {
		t.Fatalf("DeleteFileReversible: %v", err)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}
	if len(m.TrashCalls) != 1 || m.TrashCalls[0] != path {
		t.Errorf("TrashCalls = %v, want [%s]", m.TrashCalls, path)
	}
}

// ── CleanFile remaining branches ─────────────────────────────────────────

func TestCleanFile_StatError(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	dir := t.TempDir()
	parentFile := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	log := &DecisionLog{SessionID: "test", path: filepath.Join(dir, "log.json")}

	_, err := CleanFile(filepath.Join(parentFile, "child"), false, "r", "g", "h", log)
	if err == nil {
		t.Fatal("expected stat error for path under a regular file")
	}
	if !strings.Contains(err.Error(), "cannot stat") {
		t.Errorf("error = %v, want 'cannot stat'", err)
	}
}

func TestCleanFile_NoTrashDirectDelete(t *testing.T) {
	platform.Set(&platform.Mock{NoTrash: true})
	defer platform.Reset()

	dir := t.TempDir()
	path := filepath.Join(dir, "linux-style.txt")
	content := []byte("delete me directly")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	log := &DecisionLog{SessionID: "test", path: filepath.Join(dir, "log.json")}

	size, err := CleanFile(path, false, "cleanup", "grp", "hash", log)
	if err != nil {
		t.Fatalf("CleanFile: %v", err)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("file should have been removed on non-trash platform")
	}
	if len(log.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(log.Decisions))
	}
	d := log.Decisions[0]
	if d.Action != "delete" {
		t.Errorf("Action = %q, want delete", d.Action)
	}
	if d.Reversible {
		t.Error("direct delete must be recorded as not reversible")
	}
	if log.TotalFreed != int64(len(content)) {
		t.Errorf("TotalFreed = %d, want %d", log.TotalFreed, len(content))
	}
}

func TestCleanFile_NoTrashRemoveFails(t *testing.T) {
	platform.Set(&platform.Mock{NoTrash: true})
	defer platform.Reset()

	// os.Remove fails on a non-empty directory — the direct-delete error branch.
	dir := t.TempDir()
	victim := filepath.Join(dir, "nonempty")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(victim, "child.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	log := &DecisionLog{SessionID: "test", path: filepath.Join(dir, "log.json")}

	size, err := CleanFile(victim, false, "r", "g", "h", log)
	if err == nil {
		t.Fatal("expected os.Remove error for non-empty directory")
	}
	if size != 0 {
		t.Errorf("size = %d, want 0 on failure", size)
	}
	if len(log.Decisions) != 0 {
		t.Errorf("no decision should be recorded on failed removal, got %d", len(log.Decisions))
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Error("directory must survive the failed removal")
	}
}

// ── ValidatePath / checkProtected remaining branches ─────────────────────

func TestValidatePath_InsideProtectedExactDir(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	// Files INSIDE the ~/.config/anubis config dir are protected via the
	// exact-prefix branch (relPath under "<exact>/").
	home := t.TempDir()
	t.Setenv("HOME", home)

	err := ValidatePath(filepath.Join(home, ".config", "anubis", "mirror", "state.json"))
	if err == nil {
		t.Fatal("expected BLOCKED for file inside protected config dir")
	}
	if !strings.Contains(err.Error(), "BLOCKED") {
		t.Errorf("error = %v, want BLOCKED", err)
	}
}

func TestValidatePath_ProtectedHomeDirAndSuffix(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		path string
	}{
		{"home Desktop dir", filepath.Join(home, "Desktop")},
		{"keychain suffix", filepath.Join(home, "some", "login.keychain-db")},
		{"protected name .ssh", filepath.Join(home, "sub", ".ssh")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePath(tt.path); err == nil {
				t.Errorf("ValidatePath(%q) should be blocked", tt.path)
			}
		})
	}

	// A normal file in home is allowed.
	if err := ValidatePath(filepath.Join(home, "Desktop", "junk.tmp")); err != nil {
		t.Errorf("file inside Desktop should be allowed, got: %v", err)
	}
}

// ── DecisionLog error branches ───────────────────────────────────────────

func TestNewDecisionLog_NoHome(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := NewDecisionLog(); err == nil {
		t.Fatal("expected error when HOME is unset")
	}
}

func TestNewDecisionLog_MkdirFails(t *testing.T) {
	// HOME pointing at a regular file makes MkdirAll fail with ENOTDIR.
	dir := t.TempDir()
	fakeHome := filepath.Join(dir, "home-is-a-file")
	if err := os.WriteFile(fakeHome, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("HOME", fakeHome)

	if _, err := NewDecisionLog(); err == nil {
		t.Fatal("expected error when decision log dir cannot be created")
	}
}

func TestListDecisionLogs_NoHome(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := ListDecisionLogs(); err == nil {
		t.Fatal("expected error when HOME is unset")
	}
}

func TestListDecisionLogs_ReadDirError(t *testing.T) {
	// The decisions path exists but is a regular file: ReadDir fails with
	// ENOTDIR, which is not IsNotExist — the hard-error branch.
	home := t.TempDir()
	t.Setenv("HOME", home)

	mirror := filepath.Join(home, ".config", "anubis", "mirror")
	if err := os.MkdirAll(mirror, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mirror, "decisions"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := ListDecisionLogs(); err == nil {
		t.Fatal("expected error when decisions path is not a directory")
	}
}
