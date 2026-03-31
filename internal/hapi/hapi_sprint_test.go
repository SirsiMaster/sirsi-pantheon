package hapi

import (
	"os"
	"path/filepath"
	"testing"
)

// ── Hapi Coverage Sprint Tests ──────────────────────────────────────
// Additional tests targeting uncovered paths in dedup.go and detect.go.
// Avoids redeclaring tests already in hapi_test.go.

func TestFindDuplicates_MultipleGroups(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Group 1: two identical 500-byte files
	content1 := make([]byte, 500)
	for i := range content1 {
		content1[i] = 'A'
	}
	os.WriteFile(filepath.Join(tmp, "g1a.dat"), content1, 0o644)
	os.WriteFile(filepath.Join(tmp, "g1b.dat"), content1, 0o644)

	// Group 2: two identical 500-byte files with different content
	content2 := make([]byte, 500)
	for i := range content2 {
		content2[i] = 'B'
	}
	os.WriteFile(filepath.Join(tmp, "g2a.dat"), content2, 0o644)
	os.WriteFile(filepath.Join(tmp, "g2b.dat"), content2, 0o644)

	result, err := FindDuplicates([]string{tmp}, 100)
	if err != nil {
		t.Fatalf("FindDuplicates: %v", err)
	}

	if len(result.Groups) != 2 {
		t.Errorf("expected 2 duplicate groups, got %d", len(result.Groups))
	}
	if result.TotalWasted != 1000 {
		t.Errorf("total wasted = %d, want 1000", result.TotalWasted)
	}
}

func TestFindDuplicates_MultipleDirs(t *testing.T) {
	t.Parallel()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	content := make([]byte, 200)
	for i := range content {
		content[i] = 'X'
	}
	os.WriteFile(filepath.Join(dir1, "cross1.dat"), content, 0o644)
	os.WriteFile(filepath.Join(dir2, "cross2.dat"), content, 0o644)

	result, err := FindDuplicates([]string{dir1, dir2}, 100)
	if err != nil {
		t.Fatalf("FindDuplicates: %v", err)
	}
	if len(result.Groups) != 1 {
		t.Errorf("expected 1 cross-directory dup group, got %d", len(result.Groups))
	}
}

func TestFindDuplicates_SortByWasted(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Small duplicates (200 bytes)
	small := make([]byte, 200)
	for i := range small {
		small[i] = 's'
	}
	os.WriteFile(filepath.Join(tmp, "s1.dat"), small, 0o644)
	os.WriteFile(filepath.Join(tmp, "s2.dat"), small, 0o644)

	// Large duplicates (800 bytes) — should be first in results
	large := make([]byte, 800)
	for i := range large {
		large[i] = 'L'
	}
	os.WriteFile(filepath.Join(tmp, "l1.dat"), large, 0o644)
	os.WriteFile(filepath.Join(tmp, "l2.dat"), large, 0o644)

	result, err := FindDuplicates([]string{tmp}, 100)
	if err != nil {
		t.Fatalf("FindDuplicates: %v", err)
	}

	if len(result.Groups) < 2 {
		t.Fatalf("expected at least 2 groups, got %d", len(result.Groups))
	}
	// First group should have the most wasted space
	if result.Groups[0].Wasted < result.Groups[1].Wasted {
		t.Error("groups should be sorted by wasted space descending")
	}
}

func TestHashFile_NotFound(t *testing.T) {
	t.Parallel()
	_, err := hashFile("/nonexistent/file.dat")
	if err == nil {
		t.Error("hashFile(missing) should error")
	}
}

func TestHashFile_SHA256Length(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	path := filepath.Join(tmp, "test.dat")
	os.WriteFile(path, []byte("hello world"), 0o644)

	hash, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	if len(hash) != 64 { // SHA-256 hex = 64 chars
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

func TestDedupResult_ZeroValue(t *testing.T) {
	t.Parallel()
	result := &DedupResult{}
	if result.TotalWasted != 0 || result.TotalFiles != 0 || result.Scanned != 0 {
		t.Error("zero-value DedupResult should have all zero fields")
	}
}

func TestDuplicateGroup_Fields(t *testing.T) {
	t.Parallel()
	group := DuplicateGroup{
		Hash:   "abc123",
		Wasted: 1024,
	}
	if group.Hash != "abc123" {
		t.Errorf("Hash = %q", group.Hash)
	}
	if group.Wasted != 1024 {
		t.Errorf("Wasted = %d", group.Wasted)
	}
}
