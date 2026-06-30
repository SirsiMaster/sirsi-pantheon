package cleaner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ═══════════════════════════════════════════
// DecisionLog — NewDecisionLog
// ═══════════════════════════════════════════

func TestNewDecisionLog_CreatesDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatalf("NewDecisionLog() error: %v", err)
	}
	if dl == nil {
		t.Fatal("NewDecisionLog() returned nil")
	}
	if dl.SessionID == "" {
		t.Error("SessionID should be populated")
	}
	if dl.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}

	// Verify the decisions directory exists
	logDir := filepath.Join(tmp, ".config", "anubis", "mirror", "decisions")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Errorf("decision log directory was not created: %s", logDir)
	}
}

// ═══════════════════════════════════════════
// DecisionLog — Record
// ═══════════════════════════════════════════

func TestDecisionLog_Record_TrashIncrementsFreed(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatalf("NewDecisionLog() error: %v", err)
	}

	err = dl.Record(Decision{
		Path:   "/some/path/file.txt",
		Size:   1024,
		Action: "trash",
		Reason: "duplicate",
	})
	if err != nil {
		t.Fatalf("Record() error: %v", err)
	}

	if dl.TotalFreed != 1024 {
		t.Errorf("TotalFreed = %d, want 1024", dl.TotalFreed)
	}
	if len(dl.Decisions) != 1 {
		t.Errorf("Decisions count = %d, want 1", len(dl.Decisions))
	}
}

func TestDecisionLog_Record_DeleteIncrementsFreed(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatal(err)
	}

	_ = dl.Record(Decision{Path: "a", Size: 500, Action: "delete"})
	_ = dl.Record(Decision{Path: "b", Size: 300, Action: "delete"})

	if dl.TotalFreed != 800 {
		t.Errorf("TotalFreed = %d, want 800", dl.TotalFreed)
	}
}

func TestDecisionLog_Record_KeepDoesNotIncrementFreed(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatal(err)
	}

	_ = dl.Record(Decision{Path: "a", Size: 999, Action: "keep"})
	_ = dl.Record(Decision{Path: "b", Size: 999, Action: "skip"})

	if dl.TotalFreed != 0 {
		t.Errorf("TotalFreed = %d, want 0 (keep/skip should not increment)", dl.TotalFreed)
	}
}

func TestDecisionLog_Record_SetsTimestamp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	_ = dl.Record(Decision{Path: "test", Action: "trash"})
	after := time.Now()

	ts := dl.Decisions[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", ts, before, after)
	}
}

// ═══════════════════════════════════════════
// DecisionLog — Persistence (save + load)
// ═══════════════════════════════════════════

func TestDecisionLog_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatal(err)
	}

	_ = dl.Record(Decision{
		Path:       "/foo/bar.bin",
		Size:       2048,
		Action:     "trash",
		Reason:     "duplicate found",
		DupGroupID: "group-1",
		SHA256:     "abc123def456",
		Reversible: true,
	})
	_ = dl.Record(Decision{
		Path:   "/baz/qux.tmp",
		Size:   512,
		Action: "delete",
		Reason: "cache cleanup",
	})

	// Load it back
	loaded, err := LoadDecisionLog(dl.path)
	if err != nil {
		t.Fatalf("LoadDecisionLog() error: %v", err)
	}

	if loaded.SessionID != dl.SessionID {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, dl.SessionID)
	}
	if loaded.TotalFreed != dl.TotalFreed {
		t.Errorf("TotalFreed = %d, want %d", loaded.TotalFreed, dl.TotalFreed)
	}
	if len(loaded.Decisions) != 2 {
		t.Fatalf("Decisions count = %d, want 2", len(loaded.Decisions))
	}
	if loaded.Decisions[0].SHA256 != "abc123def456" {
		t.Errorf("SHA256 = %q, want %q", loaded.Decisions[0].SHA256, "abc123def456")
	}
	if loaded.Decisions[0].Reversible != true {
		t.Error("Reversible should be true for first decision")
	}
	if loaded.Decisions[1].Action != "delete" {
		t.Errorf("Action = %q, want %q", loaded.Decisions[1].Action, "delete")
	}
}

func TestDecisionLog_SaveCreatesFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatal(err)
	}

	_ = dl.Record(Decision{Path: "x", Action: "keep"})

	if _, err := os.Stat(dl.path); os.IsNotExist(err) {
		t.Error("decision log file should be created after Record()")
	}

	// Verify it's valid JSON
	data, _ := os.ReadFile(dl.path)
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("decision log is not valid JSON: %v", err)
	}
}

func TestLoadDecisionLog_NonExistent(t *testing.T) {
	_, err := LoadDecisionLog("/nonexistent/path.json")
	if err == nil {
		t.Error("LoadDecisionLog() should error on non-existent file")
	}
}

func TestLoadDecisionLog_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	badFile := filepath.Join(tmp, "bad.json")
	os.WriteFile(badFile, []byte("not json {{{"), 0644)

	_, err := LoadDecisionLog(badFile)
	if err == nil {
		t.Error("LoadDecisionLog() should error on invalid JSON")
	}
}

// ═══════════════════════════════════════════
// ListDecisionLogs
// ═══════════════════════════════════════════

func TestListDecisionLogs_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// No decision dir yet
	logs, err := ListDecisionLogs()
	if err != nil {
		t.Fatalf("ListDecisionLogs() error: %v", err)
	}
	if logs != nil {
		t.Errorf("expected nil for non-existent dir, got %v", logs)
	}
}

func TestListDecisionLogs_WithLogs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create the decision log dir with some files
	logDir := filepath.Join(tmp, ".config", "anubis", "mirror", "decisions")
	os.MkdirAll(logDir, 0700)
	os.WriteFile(filepath.Join(logDir, "session-001.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(logDir, "session-002.json"), []byte("{}"), 0644)

	logs, err := ListDecisionLogs()
	if err != nil {
		t.Fatalf("ListDecisionLogs() error: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}
}

func TestListDecisionLogs_SkipsDirectories(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	logDir := filepath.Join(tmp, ".config", "anubis", "mirror", "decisions")
	os.MkdirAll(logDir, 0700)
	os.WriteFile(filepath.Join(logDir, "session-001.json"), []byte("{}"), 0644)
	os.MkdirAll(filepath.Join(logDir, "subdir"), 0700)

	logs, err := ListDecisionLogs()
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log (dirs should be skipped), got %d", len(logs))
	}
}

// ═══════════════════════════════════════════
// DeleteFile
// ═══════════════════════════════════════════

func TestDeleteFile_DryRun(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.bin")
	data := make([]byte, 4096)
	os.WriteFile(path, data, 0644)

	freed, err := DeleteFile(path, true, false)
	if err != nil {
		t.Fatalf("DeleteFile(dry-run) error: %v", err)
	}
	if freed != 4096 {
		t.Errorf("freed = %d, want 4096", freed)
	}

	// File should still exist after dry-run
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file should still exist after dry-run")
	}
}

func TestDeleteFile_NonExistent(t *testing.T) {
	freed, err := DeleteFile("/tmp/nonexistent-path-abc123", false, false)
	if err != nil {
		t.Errorf("DeleteFile(nonexistent) should not error, got: %v", err)
	}
	if freed != 0 {
		t.Errorf("freed = %d, want 0 for non-existent file", freed)
	}
}

func TestDeleteFile_ProtectedPath(t *testing.T) {
	// Use mock platform to test protected path logic on any OS
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	_, err := DeleteFile("/System/Library/test", false, false)
	if err == nil {
		t.Error("DeleteFile on protected path should return error")
	}
}

func TestDeleteFile_ActualDelete(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "deleteme.txt")
	os.WriteFile(path, []byte("content to delete"), 0644)

	freed, err := DeleteFile(path, false, false)
	if err != nil {
		t.Fatalf("DeleteFile() error: %v", err)
	}
	if freed == 0 {
		t.Error("freed should be > 0 for actual file")
	}

	// File should be gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted after DeleteFile()")
	}
}

func TestDeleteFile_Directory(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "subdir")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644)

	freed, err := DeleteFile(dir, false, false)
	if err != nil {
		t.Fatalf("DeleteFile(dir) error: %v", err)
	}
	if freed < 6 { // at least 6 bytes (3+3)
		t.Errorf("freed = %d, want >= 6", freed)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory should be removed after DeleteFile()")
	}
}

func TestDeleteFile_DryRunDirectory(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "subdir")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "file.bin"), make([]byte, 1024), 0644)

	freed, err := DeleteFile(dir, true, false)
	if err != nil {
		t.Fatalf("DeleteFile(dir, dry-run) error: %v", err)
	}
	if freed < 1024 {
		t.Errorf("freed = %d, want >= 1024", freed)
	}

	// Directory should still exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("directory should still exist after dry-run")
	}
}

// ═══════════════════════════════════════════
// DirSize (expanded)
// ═══════════════════════════════════════════

func TestDirSize_WithFiles(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.bin"), make([]byte, 1000), 0644)
	os.WriteFile(filepath.Join(tmp, "b.bin"), make([]byte, 2000), 0644)

	size := DirSize(tmp)
	if size != 3000 {
		t.Errorf("DirSize = %d, want 3000", size)
	}
}

func TestDirSize_Nested(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(tmp, "root.bin"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(sub, "child.bin"), make([]byte, 200), 0644)

	size := DirSize(tmp)
	if size != 300 {
		t.Errorf("DirSize(nested) = %d, want 300", size)
	}
}

func TestDirSize_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	size := DirSize(tmp)
	if size != 0 {
		t.Errorf("DirSize(empty) = %d, want 0", size)
	}
}

// ═══════════════════════════════════════════
// CleanFile — safety integration
// ═══════════════════════════════════════════

func TestCleanFile_BlockedPathLogsSkip(t *testing.T) {
	// Use mock platform to test protected path logic on any OS
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatal(err)
	}

	// Try to clean a protected path (/System/ is in Mock's protected prefixes)
	_, err = CleanFile("/System/Library/something", false, "test", "grp", "hash", dl)
	if err == nil {
		t.Error("CleanFile on protected path should return error")
	}

	// Should have logged a skip decision
	if len(dl.Decisions) != 1 {
		t.Fatalf("expected 1 skip decision, got %d", len(dl.Decisions))
	}
	if dl.Decisions[0].Action != "skip" {
		t.Errorf("expected action 'skip', got %q", dl.Decisions[0].Action)
	}
}

func TestCleanFile_NonExistentFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dl, err := NewDecisionLog()
	if err != nil {
		t.Fatal(err)
	}

	freed, err := CleanFile(filepath.Join(tmp, "nonexistent.txt"), false, "test", "", "", dl)
	if err != nil {
		t.Errorf("CleanFile on non-existent should not error, got: %v", err)
	}
	if freed != 0 {
		t.Errorf("freed = %d, want 0", freed)
	}
}

// ═══════════════════════════════════════════
// Decision struct
// ═══════════════════════════════════════════

func TestDecision_JSONRoundTrip(t *testing.T) {
	d := Decision{
		Path:       "/test/path.bin",
		Size:       4096,
		Action:     "trash",
		Reason:     "duplicate",
		DupGroupID: "grp-1",
		SHA256:     "abcdef1234567890",
		Timestamp:  time.Now(),
		Reversible: true,
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var parsed Decision
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if parsed.Path != d.Path {
		t.Errorf("Path = %q, want %q", parsed.Path, d.Path)
	}
	if parsed.Size != d.Size {
		t.Errorf("Size = %d, want %d", parsed.Size, d.Size)
	}
	if parsed.Action != d.Action {
		t.Errorf("Action = %q, want %q", parsed.Action, d.Action)
	}
	if parsed.SHA256 != d.SHA256 {
		t.Errorf("SHA256 = %q, want %q", parsed.SHA256, d.SHA256)
	}
	if parsed.Reversible != d.Reversible {
		t.Errorf("Reversible = %v, want %v", parsed.Reversible, d.Reversible)
	}
}

// ═══════════════════════════════════════════
// Protected path constants — completeness
// ═══════════════════════════════════════════

func TestProtectedPrefixesContainCriticalPaths(t *testing.T) {
	// Test via the platform interface instead of the removed map
	darwin := &platform.Darwin{}
	linux := &platform.Linux{}

	mustContain := func(list []string, item string) {
		for _, p := range list {
			if p == item {
				return
			}
		}
		t.Errorf("protected prefixes missing critical path %q", item)
	}

	mustContain(darwin.ProtectedPrefixes(), "/System/")
	mustContain(darwin.ProtectedPrefixes(), "/usr/")
	mustContain(darwin.ProtectedPrefixes(), "/bin/")
	mustContain(darwin.ProtectedPrefixes(), "/sbin/")

	mustContain(linux.ProtectedPrefixes(), "/boot/")
	mustContain(linux.ProtectedPrefixes(), "/etc/")
	mustContain(linux.ProtectedPrefixes(), "/proc/")
	mustContain(linux.ProtectedPrefixes(), "/sys/")
	mustContain(linux.ProtectedPrefixes(), "/dev/")
}

func TestProtectedNamesContainSensitiveFiles(t *testing.T) {
	for _, essential := range []string{".git", ".env", ".ssh", ".gnupg", "id_rsa", "id_ed25519"} {
		found := false
		for _, n := range protectedNames {
			if n == essential {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("protectedNames missing %q", essential)
		}
	}
}

func TestProtectedHomeDirsContainUserDirs(t *testing.T) {
	for _, dir := range []string{"Desktop", "Documents", "Downloads", "Pictures", "Music", "Movies", "Library"} {
		found := false
		for _, d := range protectedHomeDirs {
			if d == dir {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("protectedHomeDirs missing %q", dir)
		}
	}
}
