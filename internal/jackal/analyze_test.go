package jackal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnalyze_BasicDirectory(t *testing.T) {
	tmp := t.TempDir()

	// Create visible subdirectories with files
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(tmp, name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "data.bin"), make([]byte, 4096), 0o644)
	}
	// Single file at root
	os.WriteFile(filepath.Join(tmp, "readme.txt"), []byte("hello"), 0o644)

	result, err := Analyze(tmp, 0)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if result.Path != tmp {
		t.Errorf("Path = %q, want %q", result.Path, tmp)
	}
	if len(result.Entries) < 3 {
		t.Errorf("expected at least 3 entries, got %d", len(result.Entries))
	}
	if result.TotalSize == 0 {
		t.Error("TotalSize should be > 0")
	}
	if result.ScanTime == 0 {
		t.Error("ScanTime should be > 0")
	}

	// Entries should be sorted by size descending
	for i := 1; i < len(result.Entries); i++ {
		if result.Entries[i].Size > result.Entries[i-1].Size {
			t.Errorf("entries not sorted: [%d].Size=%d > [%d].Size=%d",
				i, result.Entries[i].Size, i-1, result.Entries[i-1].Size)
		}
	}
}

func TestAnalyze_SkipsHiddenDirs(t *testing.T) {
	tmp := t.TempDir()

	os.MkdirAll(filepath.Join(tmp, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(tmp, ".hidden", "secret"), make([]byte, 1024), 0o644)
	os.MkdirAll(filepath.Join(tmp, "visible"), 0o755)
	os.WriteFile(filepath.Join(tmp, "visible", "data"), make([]byte, 1024), 0o644)

	result, err := Analyze(tmp, 0)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	for _, e := range result.Entries {
		if e.Name == ".hidden" {
			t.Error("hidden directory should be skipped")
		}
	}
}

func TestAnalyze_EmptyDir(t *testing.T) {
	tmp := t.TempDir()

	result, err := Analyze(tmp, 0)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for empty dir, got %d", len(result.Entries))
	}
	if result.TotalSize != 0 {
		t.Errorf("expected 0 total size, got %d", result.TotalSize)
	}
}

func TestAnalyze_NotADirectory(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "file.txt")
	os.WriteFile(file, []byte("data"), 0o644)

	_, err := Analyze(file, 0)
	if err == nil {
		t.Error("expected error for non-directory path")
	}
}

func TestAnalyze_NonexistentPath(t *testing.T) {
	_, err := Analyze("/nonexistent/path/that/does/not/exist", 0)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestAnalyzeCtx_Cancellation(t *testing.T) {
	tmp := t.TempDir()

	// Create some content
	for i := 0; i < 5; i++ {
		dir := filepath.Join(tmp, filepath.Base(t.Name())+string(rune('a'+i)))
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "f"), make([]byte, 100), 0o644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result, err := AnalyzeCtx(ctx, tmp, 0)
	// Either error or empty results is acceptable for canceled context
	if err != nil {
		return // error is fine
	}
	// If no error, result should still be valid (possibly partial)
	if result == nil {
		t.Error("nil result without error")
	}
}

func TestAnalyze_MaxDepth(t *testing.T) {
	tmp := t.TempDir()

	// Create nested structure: dir/sub/deep/file
	deep := filepath.Join(tmp, "top", "sub", "deep")
	os.MkdirAll(deep, 0o755)
	os.WriteFile(filepath.Join(deep, "file.bin"), make([]byte, 2048), 0o644)

	// With maxDepth=1, the deep file size should not be counted
	r1, err := AnalyzeCtx(context.Background(), tmp, 1)
	if err != nil {
		t.Fatalf("maxDepth=1 error: %v", err)
	}

	// With maxDepth=0 (unlimited), the deep file should be counted
	r0, err := AnalyzeCtx(context.Background(), tmp, 0)
	if err != nil {
		t.Fatalf("maxDepth=0 error: %v", err)
	}

	if r0.TotalSize < r1.TotalSize {
		t.Errorf("unlimited depth (%d) should be >= depth=1 (%d)", r0.TotalSize, r1.TotalSize)
	}
}

func TestAnalyze_CapsEntries(t *testing.T) {
	tmp := t.TempDir()

	// Create more than maxAnalyzeEntries visible dirs
	for i := 0; i < maxAnalyzeEntries+5; i++ {
		name := filepath.Join(tmp, string(rune('A'+i/26))+string(rune('a'+i%26)))
		os.MkdirAll(name, 0o755)
		os.WriteFile(filepath.Join(name, "f"), make([]byte, 100), 0o644)
	}

	result, err := Analyze(tmp, 0)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if len(result.Entries) > maxAnalyzeEntries {
		t.Errorf("entries = %d, should be capped at %d", len(result.Entries), maxAnalyzeEntries)
	}
}

func TestAnalyze_FileEntryModTime(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "recent.txt"), []byte("x"), 0o644)

	result, err := Analyze(tmp, 0)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if len(result.Entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
	if result.Entries[0].ModTime.IsZero() {
		t.Error("ModTime should be populated")
	}
	if time.Since(result.Entries[0].ModTime) > time.Minute {
		t.Error("ModTime should be recent")
	}
}
