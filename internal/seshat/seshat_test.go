package seshat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPaths(t *testing.T) {
	paths := DefaultPaths()
	if paths.KnowledgeDir == "" {
		t.Fatal("KnowledgeDir should not be empty")
	}
	if paths.BrainDir == "" {
		t.Fatal("BrainDir should not be empty")
	}
}

func TestWriteAndReadKnowledgeItem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}

	tmpDir := t.TempDir()
	paths := Paths{
		KnowledgeDir: filepath.Join(tmpDir, "knowledge"),
	}

	ki := KnowledgeItem{
		Title:   "Test Knowledge Item",
		Summary: "A test KI for Seshat.",
		References: []KIReference{
			{Type: "file", Value: "/test/path"},
		},
	}

	artifacts := map[string]string{
		"overview.md": "# Test Overview\n\nThis is a test.",
		"notes.md":    "# Notes\n\nSome notes.",
	}

	// Write
	err := WriteKnowledgeItem(paths, "test_ki", ki, artifacts)
	if err != nil {
		t.Fatalf("WriteKnowledgeItem: %v", err)
	}

	// Verify directory structure
	metaPath := filepath.Join(paths.KnowledgeDir, "test_ki", "metadata.json")
	if _, statErr := os.Stat(metaPath); os.IsNotExist(statErr) {
		t.Fatal("metadata.json not created")
	}

	tsPath := filepath.Join(paths.KnowledgeDir, "test_ki", "timestamps.json")
	if _, statErr := os.Stat(tsPath); os.IsNotExist(statErr) {
		t.Fatal("timestamps.json not created")
	}

	overviewPath := filepath.Join(paths.KnowledgeDir, "test_ki", "artifacts", "overview.md")
	if _, statErr := os.Stat(overviewPath); os.IsNotExist(statErr) {
		t.Fatal("artifacts/overview.md not created")
	}

	// Read back
	readKI, err := ReadKnowledgeItem(paths, "test_ki")
	if err != nil {
		t.Fatalf("ReadKnowledgeItem: %v", err)
	}

	if readKI.Title != "Test Knowledge Item" {
		t.Errorf("Title = %q, want %q", readKI.Title, "Test Knowledge Item")
	}
	if readKI.Summary != "A test KI for Seshat." {
		t.Errorf("Summary mismatch")
	}
	if len(readKI.References) != 1 {
		t.Errorf("References count = %d, want 1", len(readKI.References))
	}
}

func TestListKnowledgeItems(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}

	tmpDir := t.TempDir()
	paths := Paths{
		KnowledgeDir: filepath.Join(tmpDir, "knowledge"),
	}

	// Create two KIs
	for _, name := range []string{"ki_alpha", "ki_beta"} {
		err := WriteKnowledgeItem(paths, name, KnowledgeItem{
			Title:   name,
			Summary: "test",
		}, map[string]string{"readme.md": "# " + name})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	items, err := ListKnowledgeItems(paths)
	if err != nil {
		t.Fatalf("ListKnowledgeItems: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("items = %d, want 2", len(items))
	}
}

func TestSyncKIToGeminiMD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}

	tmpDir := t.TempDir()
	paths := Paths{
		KnowledgeDir: filepath.Join(tmpDir, "knowledge"),
	}

	// Create a KI
	err := WriteKnowledgeItem(paths, "test_sync", KnowledgeItem{
		Title:   "Sync Test",
		Summary: "Testing GEMINI.md injection.",
	}, map[string]string{"overview.md": "# Overview"})
	if err != nil {
		t.Fatalf("create KI: %v", err)
	}

	// Sync to a new file
	targetFile := filepath.Join(tmpDir, "GEMINI.md")
	err = SyncKIToGeminiMD(paths, "test_sync", targetFile)
	if err != nil {
		t.Fatalf("SyncKIToGeminiMD: %v", err)
	}

	content, _ := os.ReadFile(targetFile)
	contentStr := string(content)

	if !contains(contentStr, "<!-- KI:test_sync:START -->") {
		t.Error("missing start marker")
	}
	if !contains(contentStr, "<!-- KI:test_sync:END -->") {
		t.Error("missing end marker")
	}
	if !contains(contentStr, "Sync Test") {
		t.Error("missing KI title")
	}

	// Sync again — should replace, not duplicate
	err = SyncKIToGeminiMD(paths, "test_sync", targetFile)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}

	content, _ = os.ReadFile(targetFile)
	hits := countOccurrences(string(content), "<!-- KI:test_sync:START -->")
	if hits != 1 {
		t.Errorf("marker count = %d, want 1 (idempotent)", hits)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}
