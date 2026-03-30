package rules

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// ── baseScanRule.Scan Tests ─────────────────────────────────────────────

func TestBaseScanRule_Scan_MatchesFiles(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a scannable file
	cacheDir := filepath.Join(tmp, "caches")
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "data.bin"), make([]byte, 1024), 0o644)

	rule := &baseScanRule{
		name:        "test_cache",
		displayName: "Test Caches",
		category:    jackal.CategoryGeneral,
		description: "Test cache rule",
		platforms:   []string{"darwin", "linux"},
		paths:       []string{filepath.Join(tmp, "caches")},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleName != "test_cache" {
		t.Errorf("RuleName = %q", findings[0].RuleName)
	}
	if findings[0].SizeBytes == 0 {
		t.Error("SizeBytes should be > 0")
	}
	if !findings[0].IsDir {
		t.Error("expected IsDir=true for directory finding")
	}
}

func TestBaseScanRule_Scan_NoMatches(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	rule := &baseScanRule{
		name:      "nonexistent_cache",
		paths:     []string{filepath.Join(tmp, "does_not_exist*")},
		platforms: []string{"darwin", "linux"},
		category:  jackal.CategoryGeneral,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for nonexistent path, got %d", len(findings))
	}
}

func TestBaseScanRule_Scan_GlobPattern(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create multiple matching dirs
	for _, name := range []string{"cache-a", "cache-b"} {
		dir := filepath.Join(tmp, name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "file.dat"), make([]byte, 512), 0o644)
	}

	rule := &baseScanRule{
		name:      "glob_test",
		paths:     []string{filepath.Join(tmp, "cache-*")},
		platforms: []string{"darwin", "linux"},
		category:  jackal.CategoryDev,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 2 {
		t.Errorf("expected 2 findings from glob, got %d", len(findings))
	}
}

func TestBaseScanRule_Scan_SkipsEmptyDirs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	emptyDir := filepath.Join(tmp, "empty")
	os.MkdirAll(emptyDir, 0o755)

	rule := &baseScanRule{
		name:      "empty_test",
		paths:     []string{filepath.Join(tmp, "empty")},
		platforms: []string{"darwin", "linux"},
		category:  jackal.CategoryGeneral,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty dir, got %d", len(findings))
	}
}

func TestBaseScanRule_Scan_MinAgeDays(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a file that was just modified (age = 0)
	cacheDir := filepath.Join(tmp, "recent")
	os.MkdirAll(cacheDir, 0o755)
	recentFile := filepath.Join(cacheDir, "new.dat")
	os.WriteFile(recentFile, make([]byte, 256), 0o644)

	rule := &baseScanRule{
		name:       "age_test",
		paths:      []string{filepath.Join(tmp, "recent")},
		platforms:  []string{"darwin", "linux"},
		category:   jackal.CategoryGeneral,
		minAgeDays: 30, // Must be 30+ days old
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	// Recent files should be filtered out
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (too recent), got %d", len(findings))
	}
}

func TestBaseScanRule_Scan_MinAgeDaysFromOpts(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create old file
	oldDir := filepath.Join(tmp, "old")
	os.MkdirAll(oldDir, 0o755)
	oldFile := filepath.Join(oldDir, "ancient.dat")
	os.WriteFile(oldFile, make([]byte, 256), 0o644)
	// Set mod time to 60 days ago
	oldTime := time.Now().AddDate(0, 0, -60)
	os.Chtimes(oldFile, oldTime, oldTime)

	rule := &baseScanRule{
		name:      "old_test",
		paths:     []string{filepath.Join(tmp, "old", "*")},
		platforms: []string{"darwin", "linux"},
		category:  jackal.CategoryGeneral,
	}

	// With MinAgeDays override in opts
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{
		HomeDir:    tmp,
		MinAgeDays: 30,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding (old file), got %d", len(findings))
	}
}

func TestBaseScanRule_Scan_Excludes(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create dirs
	for _, name := range []string{"keep", "skip"} {
		dir := filepath.Join(tmp, name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "file.dat"), make([]byte, 512), 0o644)
	}

	rule := &baseScanRule{
		name:      "exclude_test",
		paths:     []string{filepath.Join(tmp, "*")},
		platforms: []string{"darwin", "linux"},
		category:  jackal.CategoryGeneral,
		excludes:  []string{filepath.Join(tmp, "skip")},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	for _, f := range findings {
		if filepath.Base(f.Path) == "skip" {
			t.Error("excluded path 'skip' should not appear in findings")
		}
	}
}

// ── isExcluded Tests ────────────────────────────────────────────────────

func TestIsExcluded_DirectAndChild(t *testing.T) {
	t.Parallel()
	rule := &baseScanRule{
		excludes: []string{"/opt/excluded"},
	}

	if !rule.isExcluded("/opt/excluded", "/Users/test") {
		t.Error("exact exclude path should be excluded")
	}
	if !rule.isExcluded("/opt/excluded/subdir/file.txt", "/Users/test") {
		t.Error("child of exclude path should be excluded")
	}
}

func TestIsExcluded_GlobWithTrailingStar(t *testing.T) {
	t.Parallel()
	// The prefix-match check in isExcluded only short-circuits when the
	// exclude pattern ends in '*'. Globs like *.log end in 'g' and fall
	// through to the prefix check — which correctly matches subdirectories.
	rule := &baseScanRule{
		excludes: []string{"/opt/caches/*"},
	}

	// Trailing * means prefix-match is skipped — only glob match applies
	if !rule.isExcluded("/opt/caches/test.log", "/Users/test") {
		t.Error("/* glob should match any child")
	}
	// Parent dir itself should NOT match the /* glob
	if rule.isExcluded("/opt/caches", "/Users/test") {
		t.Error("/* glob should NOT match parent dir itself")
	}
}

func TestIsExcluded_NoExcludes(t *testing.T) {
	t.Parallel()
	rule := &baseScanRule{}
	if rule.isExcluded("/any/path", "/home") {
		t.Error("empty excludes should never exclude")
	}
}

// ── dirSizeAndCount Tests ───────────────────────────────────────────────

func TestDirSizeAndCount_Basic(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create 3 files
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		os.WriteFile(filepath.Join(tmp, name), make([]byte, 100), 0o644)
	}

	size, count := dirSizeAndCount(tmp)
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
	if size != 300 {
		t.Errorf("size = %d, want 300", size)
	}
}

func TestDirSizeAndCount_Empty(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	size, count := dirSizeAndCount(tmp)
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if size != 0 {
		t.Errorf("size = %d, want 0", size)
	}
}

func TestDirSizeAndCount_Nested(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	subdir := filepath.Join(tmp, "sub")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(tmp, "root.txt"), make([]byte, 50), 0o644)
	os.WriteFile(filepath.Join(subdir, "nested.txt"), make([]byte, 200), 0o644)

	size, count := dirSizeAndCount(tmp)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if size != 250 {
		t.Errorf("size = %d, want 250", size)
	}
}

func TestDirSizeAndCount_NonExistent(t *testing.T) {
	t.Parallel()
	size, count := dirSizeAndCount("/nonexistent/path")
	if count != 0 || size != 0 {
		t.Errorf("nonexistent: size=%d count=%d, want 0/0", size, count)
	}
}

// ── baseScanRule field accessors ─────────────────────────────────────────

func TestBaseScanRule_Accessors(t *testing.T) {
	t.Parallel()
	rule := &baseScanRule{
		name:        "test_rule",
		displayName: "Test Rule",
		category:    jackal.CategoryAI,
		description: "A test rule for coverage",
		platforms:   []string{"darwin", "linux", "windows"},
	}

	if rule.Name() != "test_rule" {
		t.Errorf("Name() = %q", rule.Name())
	}
	if rule.DisplayName() != "Test Rule" {
		t.Errorf("DisplayName() = %q", rule.DisplayName())
	}
	if rule.Category() != jackal.CategoryAI {
		t.Errorf("Category() = %q", rule.Category())
	}
	if rule.Description() != "A test rule for coverage" {
		t.Errorf("Description() = %q", rule.Description())
	}
	if len(rule.Platforms()) != 3 {
		t.Errorf("Platforms() len = %d", len(rule.Platforms()))
	}
}

// ── baseScanRule.Clean Tests ────────────────────────────────────────────

func TestBaseScanRule_Clean_DryRun(t *testing.T) {
	t.Parallel()
	rule := &baseScanRule{name: "clean_test"}

	findings := []jackal.Finding{
		{Path: "/nonexistent/file.dat", SizeBytes: 1024},
	}

	result, err := rule.Clean(context.Background(), findings, jackal.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}
	if result == nil {
		t.Fatal("Clean returned nil result")
	}
	// DryRun should still attempt (cleaner.DeleteFile handles dry run)
}

func TestBaseScanRule_Clean_NonExistentFile(t *testing.T) {
	t.Parallel()
	rule := &baseScanRule{name: "clean_test"}

	findings := []jackal.Finding{
		{Path: "/nonexistent/path/file.dat", SizeBytes: 512},
	}

	result, err := rule.Clean(context.Background(), findings, jackal.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	// cleaner.DeleteFile may skip or error on nonexistent — either is acceptable
	t.Logf("Clean result: cleaned=%d skipped=%d errors=%d",
		result.Cleaned, result.Skipped, len(result.Errors))
}
