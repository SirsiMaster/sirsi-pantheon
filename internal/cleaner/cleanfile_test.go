package cleaner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ── CleanFile ────────────────────────────────────────────────────────────

func TestCleanFile_Success(t *testing.T) {
	// Set mock platform for controlled behavior
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	// Create a temp file to clean
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test-file.txt")
	os.WriteFile(filePath, []byte("hello world"), 0o644)

	// Create a decision log in the temp dir
	logPath := filepath.Join(dir, "decisions.json")
	log := &DecisionLog{
		SessionID: "test-session",
		path:      logPath,
	}

	// CleanFile should succeed — Mock platform returns nil for MoveToTrash
	size, err := CleanFile(filePath, "test reason", "group-1", "abc123", log)
	if err != nil {
		t.Fatalf("CleanFile: %v", err)
	}
	if size <= 0 {
		t.Errorf("Expected positive size, got %d", size)
	}

	// Decision should be recorded
	if len(log.Decisions) == 0 {
		t.Error("Expected at least one decision")
	}
	d := log.Decisions[0]
	if d.Action != "trash" && d.Action != "delete" {
		t.Errorf("Action = %q, want trash or delete", d.Action)
	}
	if d.Reason != "test reason" {
		t.Errorf("Reason = %q", d.Reason)
	}
	if d.DupGroupID != "group-1" {
		t.Errorf("DupGroupID = %q", d.DupGroupID)
	}
	if d.SHA256 != "abc123" {
		t.Errorf("SHA256 = %q", d.SHA256)
	}
}

func TestCleanFile_ProtectedPath(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "decisions.json")
	log := &DecisionLog{
		SessionID: "test-session",
		path:      logPath,
	}

	// Try to clean a protected path
	_, err := CleanFile("/usr/bin/ls", "test", "g1", "hash", log)
	if err == nil {
		t.Error("Expected error for protected path")
	}

	// Skip decision should be recorded
	if len(log.Decisions) == 0 {
		t.Error("Expected skip decision for protected path")
	}
	if log.Decisions[0].Action != "skip" {
		t.Errorf("Action = %q, want skip", log.Decisions[0].Action)
	}
}

func TestCleanFile_NonExistent(t *testing.T) {
	platform.Set(&platform.Mock{})
	defer platform.Reset()

	dir := t.TempDir()
	log := &DecisionLog{SessionID: "test", path: filepath.Join(dir, "log.json")}

	size, err := CleanFile(filepath.Join(dir, "nonexistent.txt"), "test", "g1", "h", log)
	if err != nil {
		t.Errorf("CleanFile for nonexistent file should not error: %v", err)
	}
	if size != 0 {
		t.Errorf("Size = %d, want 0 for nonexistent file", size)
	}
}

// ── DecisionLog ──────────────────────────────────────────────────────────

func TestDecisionLog_RecordAndSave(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test-decisions.json")

	log := &DecisionLog{
		SessionID: "test-session",
		path:      logPath,
	}

	err := log.Record(Decision{
		Path:   "/tmp/test.txt",
		Size:   1024,
		Action: "trash",
		Reason: "duplicate",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	if log.TotalFreed != 1024 {
		t.Errorf("TotalFreed = %d, want 1024", log.TotalFreed)
	}

	// Verify file was saved
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Errorf("Decision log file not created: %v", statErr)
	}

	// Load it back
	loaded, err := LoadDecisionLog(logPath)
	if err != nil {
		t.Fatalf("LoadDecisionLog: %v", err)
	}
	if len(loaded.Decisions) != 1 {
		t.Fatalf("Expected 1 decision, got %d", len(loaded.Decisions))
	}
	if loaded.Decisions[0].Action != "trash" {
		t.Errorf("Action = %q", loaded.Decisions[0].Action)
	}
}

func TestDecisionLog_RecordSkip(t *testing.T) {
	dir := t.TempDir()
	log := &DecisionLog{SessionID: "test", path: filepath.Join(dir, "log.json")}

	log.Record(Decision{
		Path:   "/tmp/skip.txt",
		Size:   500,
		Action: "skip",
		Reason: "blocked",
	})

	// skip should not add to TotalFreed
	if log.TotalFreed != 0 {
		t.Errorf("TotalFreed = %d, want 0 for skip action", log.TotalFreed)
	}
}
