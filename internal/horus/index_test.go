package horus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ── Build & Query ──────────────────────────────────────────────────────────

func TestIndex_BuildAndQuery(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "manifest.gob")

	// Create a synthetic directory tree so the test is cross-platform.
	// ~/Library/Caches only exists on macOS — Linux CI has no such path.
	dataDir := filepath.Join(tmpDir, "data")
	subDir := filepath.Join(dataDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write some files so size > 0
	os.WriteFile(filepath.Join(dataDir, "a.txt"), []byte("hello world"), 0o644)
	os.WriteFile(filepath.Join(dataDir, "b.bin"), make([]byte, 4096), 0o644)
	os.WriteFile(filepath.Join(subDir, "c.log"), []byte("log entry"), 0o644)

	// Build index of the synthetic tree.
	start := time.Now()
	m, err := Index(IndexOptions{
		Roots:        []string{dataDir},
		MaxDepth:     5,
		CachePath:    cachePath,
		ForceRefresh: true,
	})
	buildTime := time.Since(start)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	t.Logf("BUILD: %d dirs, %d files in %s",
		m.Stats.DirsWalked, m.Stats.FilesIndexed, buildTime.Round(time.Millisecond))

	if m.Stats.FilesIndexed < 3 {
		t.Errorf("Expected at least 3 files indexed, got %d", m.Stats.FilesIndexed)
	}

	// Check cache file was written.
	if info, statErr := os.Stat(cachePath); statErr == nil {
		t.Logf("CACHE SIZE: %.1f KB (gob)", float64(info.Size())/1024)
	} else {
		t.Errorf("Cache file not written: %v", statErr)
	}

	// Load from cache — should be very fast.
	start = time.Now()
	m2, err := Index(IndexOptions{
		Roots:     []string{dataDir},
		MaxDepth:  5,
		CachePath: cachePath,
		TTL:       1 * time.Hour,
	})
	queryTime := time.Since(start)
	if err != nil {
		t.Fatalf("Cached load failed: %v", err)
	}

	t.Logf("CACHE LOAD: %d dir summaries in %s", len(m2.Dirs), queryTime.Round(time.Millisecond))

	// Query: DirSizeAndCount — O(1) lookup.
	start = time.Now()
	size, count := m.DirSizeAndCount(dataDir)
	queryDur := time.Since(start)
	t.Logf("QUERY DirSizeAndCount(data): %d bytes, %d files in %s", size, count, queryDur)

	if size == 0 {
		t.Error("Expected non-zero size for data dir")
	}
	if count < 3 {
		t.Errorf("Expected at least 3 files, got %d", count)
	}
}

func TestManifest_Exists(t *testing.T) {
	m := &Manifest{
		Dirs: map[string]DirSummary{
			"/foo":     {TotalSize: 100, FileCount: 1},
			"/foo/bar": {TotalSize: 100},
		},
	}
	if !m.Exists("/foo/bar") {
		t.Error("Expected /foo/bar to exist")
	}
	if !m.Exists("/foo") {
		t.Error("Expected /foo to exist via Dirs")
	}
	if m.Exists("/foo/baz") {
		t.Error("Expected /foo/baz to not exist")
	}
}

func TestManifest_DirSizeAndCount(t *testing.T) {
	m := &Manifest{
		Dirs: map[string]DirSummary{
			"/data":     {TotalSize: 350, FileCount: 3, DirCount: 1},
			"/data/sub": {TotalSize: 50, FileCount: 1},
			"/other":    {TotalSize: 999, FileCount: 1},
		},
	}

	size, count := m.DirSizeAndCount("/data")
	if size != 350 {
		t.Errorf("Expected size 350, got %d", size)
	}
	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}

	// Test O(1): subdir lookup.
	size2, count2 := m.DirSizeAndCount("/data/sub")
	if size2 != 50 || count2 != 1 {
		t.Errorf("Expected size 50/count 1, got %d/%d", size2, count2)
	}

	// Non-existent directory.
	size3, count3 := m.DirSizeAndCount("/nonexistent")
	if size3 != 0 || count3 != 0 {
		t.Errorf("Expected 0/0, got %d/%d", size3, count3)
	}
}

// ── DirSize & DirCount (0% → 100%) ──────────────────────────────────────

func TestManifest_DirSize(t *testing.T) {
	m := &Manifest{
		Dirs: map[string]DirSummary{
			"/data": {TotalSize: 12345, FileCount: 5},
		},
	}

	if got := m.DirSize("/data"); got != 12345 {
		t.Errorf("DirSize: expected 12345, got %d", got)
	}
	if got := m.DirSize("/nonexistent"); got != 0 {
		t.Errorf("DirSize nonexistent: expected 0, got %d", got)
	}
}

func TestManifest_DirCount(t *testing.T) {
	m := &Manifest{
		Dirs: map[string]DirSummary{
			"/data": {TotalSize: 100, FileCount: 7},
		},
	}

	if got := m.DirCount("/data"); got != 7 {
		t.Errorf("DirCount: expected 7, got %d", got)
	}
	if got := m.DirCount("/nonexistent"); got != 0 {
		t.Errorf("DirCount nonexistent: expected 0, got %d", got)
	}
}

// ── Glob (0% → 100%) ────────────────────────────────────────────────────

func TestManifest_Glob(t *testing.T) {
	m := &Manifest{
		Dirs: map[string]DirSummary{
			"/data":         {},
			"/other":        {},
			"/data/sub":     {},
			"/data/reports": {},
		},
	}

	// Match all directories in /data
	matches := m.Glob("/data/*")
	if len(matches) != 2 {
		t.Errorf("Expected 2 directory matches for /data/*, got %d: %v", len(matches), matches)
	}

	// No more file-level globbing in Phase 2 for performance.
	logMatches := m.Glob("/data/*.log")
	if len(logMatches) != 0 {
		t.Errorf("Expected 0 matches for files in Phase 2, got %d", len(logMatches))
	}
}

// ── EntriesUnder (0% → 100%) ────────────────────────────────────────────

func TestManifest_EntriesUnder(t *testing.T) {
	m := &Manifest{
		Dirs: map[string]DirSummary{
			"/data":       {TotalSize: 1000, FileCount: 10},
			"/data/sub1":  {TotalSize: 200, FileCount: 3},
			"/data/sub2":  {TotalSize: 300, FileCount: 4},
			"/other":      {TotalSize: 500, FileCount: 5},
			"/data-extra": {TotalSize: 100, FileCount: 1}, // prefix trick — should NOT match
		},
	}

	result := m.EntriesUnder("/data")
	// Should match /data/sub1 and /data/sub2 but NOT /data itself, /other, or /data-extra
	if len(result) != 2 {
		t.Errorf("Expected 2 entries under /data, got %d: %v", len(result), result)
	}
	if _, ok := result["/data/sub1"]; !ok {
		t.Error("Missing /data/sub1")
	}
	if _, ok := result["/data/sub2"]; !ok {
		t.Error("Missing /data/sub2")
	}
	if _, ok := result["/data-extra"]; ok {
		t.Error("Should not match /data-extra (prefix mismatch)")
	}

	// Empty result
	empty := m.EntriesUnder("/nonexistent")
	if len(empty) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(empty))
	}
}

// ── FindDirsNamed (0% → 100%) ──────────────────────────────────────────

func TestManifest_FindDirsNamed(t *testing.T) {
	sep := string(filepath.Separator)
	m := &Manifest{
		Dirs: map[string]DirSummary{
			"/project":                         {},
			"/project/app":                     {},
			"/project/app/node_modules":        {},
			"/project/lib":                     {},
			"/project/lib/node_modules":        {},
			"/project/deep/a/b/c/node_modules": {},
			"/other/node_modules":              {},
		},
	}
	_ = sep

	// Find all node_modules under /project (no depth limit)
	results := m.FindDirsNamed("/project", "node_modules", 0)
	if len(results) != 3 {
		t.Errorf("Expected 3 node_modules under /project, got %d: %v", len(results), results)
	}

	// With depth limit of 2 — should exclude deep/a/b/c/node_modules (depth 4)
	shallow := m.FindDirsNamed("/project", "node_modules", 2)
	for _, p := range shallow {
		rel, _ := filepath.Rel("/project", p)
		depth := strings.Count(rel, sep)
		if depth > 2 {
			t.Errorf("FindDirsNamed returned %s at depth %d, limit was 2", p, depth)
		}
	}

	// Should NOT match /other/node_modules
	for _, p := range results {
		if strings.HasPrefix(p, "/other") {
			t.Errorf("Should not match outside root: %s", p)
		}
	}

	// Non-existent name
	none := m.FindDirsNamed("/project", "nonexistent", 0)
	if len(none) != 0 {
		t.Errorf("Expected 0 results, got %d", len(none))
	}
}

// ── Summary (0% → 100%) ────────────────────────────────────────────────

func TestManifest_Summary(t *testing.T) {
	m := &Manifest{
		Version: "2.0.0",
		Stats: WalkStats{
			DirsWalked:   100,
			FilesIndexed: 5000,
			WalkDuration: 250 * time.Millisecond,
			Parallelism:  8,
		},
	}

	s := m.Summary()
	if !strings.Contains(s, "2.0.0") {
		t.Errorf("Summary should contain version, got: %s", s)
	}
	if !strings.Contains(s, "100 dirs") {
		t.Errorf("Summary should contain dir count, got: %s", s)
	}
	if !strings.Contains(s, "5000 files") {
		t.Errorf("Summary should contain file count, got: %s", s)
	}
	if !strings.Contains(s, "8 goroutines") {
		t.Errorf("Summary should contain parallelism, got: %s", s)
	}
}

// ── DefaultCachePath (0% → 100%) ────────────────────────────────────────

func TestDefaultCachePath(t *testing.T) {
	p := DefaultCachePath()
	if p == "" {
		t.Fatal("DefaultCachePath returned empty string")
	}
	if !strings.HasSuffix(p, "manifest.gob") {
		t.Errorf("Expected path ending in manifest.gob, got: %s", p)
	}
	if !strings.Contains(p, "pantheon") {
		t.Errorf("Expected path containing 'pantheon', got: %s", p)
	}
	if !strings.Contains(p, "horus") {
		t.Errorf("Expected path containing 'horus', got: %s", p)
	}
}

// ── DefaultRoots (0% → 100%) ───────────────────────────────────────────

func TestDefaultRoots(t *testing.T) {
	roots := DefaultRoots()
	if len(roots) == 0 {
		t.Fatal("DefaultRoots returned empty slice")
	}

	// Phase 3: Scoped roots — should contain hygiene targets, not ~/Development
	foundCache := false
	foundCaches := false
	for _, r := range roots {
		if strings.HasSuffix(r, ".cache") {
			foundCache = true
		}
		if strings.HasSuffix(r, "Caches") {
			foundCaches = true
		}
	}
	if !foundCache {
		t.Error("DefaultRoots should include ~/.cache")
	}
	if !foundCaches {
		t.Error("DefaultRoots should include ~/Library/Caches")
	}

	// Platform-specific entries
	switch runtime.GOOS {
	case "darwin":
		foundBrew := false
		for _, r := range roots {
			if strings.Contains(r, "homebrew") {
				foundBrew = true
			}
		}
		if !foundBrew {
			t.Error("On darwin, DefaultRoots should include /opt/homebrew")
		}
	case "linux":
		foundVar := false
		for _, r := range roots {
			if strings.Contains(r, "/var/cache") {
				foundVar = true
			}
		}
		if !foundVar {
			t.Error("On linux, DefaultRoots should include /var/cache")
		}
	}

	t.Logf("DefaultRoots (%s): %d roots", runtime.GOOS, len(roots))
}

// ── loadJSONManifest (0% → 100%) ────────────────────────────────────────

func TestLoadJSONManifest(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "manifest.json")

	// Write a valid JSON manifest
	m := &Manifest{
		Version:   "1.0.0",
		Platform:  "test/amd64",
		Timestamp: time.Now().Add(-1 * time.Minute),
		Roots:     []string{"/tmp"},
		Dirs: map[string]DirSummary{
			"/tmp/data": {TotalSize: 100, FileCount: 2, DirCount: 1},
		},
		Stats: WalkStats{
			DirsWalked:   5,
			FilesIndexed: 10,
			Parallelism:  4,
		},
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(jsonPath, data, 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Load it
	loaded, err := loadJSONManifest(jsonPath)
	if err != nil {
		t.Fatalf("loadJSONManifest failed: %v", err)
	}
	if loaded.Version != "1.0.0" {
		t.Errorf("Version: expected 1.0.0, got %s", loaded.Version)
	}
	if loaded.Stats.DirsWalked != 5 {
		t.Errorf("DirsWalked: expected 5, got %d", loaded.Stats.DirsWalked)
	}
	if len(loaded.Dirs) != 1 {
		t.Errorf("Dirs: expected 1, got %d", len(loaded.Dirs))
	}

	// Invalid JSON
	badPath := filepath.Join(tmpDir, "bad.json")
	os.WriteFile(badPath, []byte("{invalid"), 0o644)
	_, err = loadJSONManifest(badPath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}

	// Missing file
	_, err = loadJSONManifest(filepath.Join(tmpDir, "nonexistent.json"))
	if err == nil {
		t.Error("Expected error for missing file")
	}
}

// ── Index with JSON fallback ────────────────────────────────────────────

func TestIndex_JSONFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "manifest.gob") // gob path
	jsonPath := filepath.Join(tmpDir, "manifest.json") // JSON fallback

	// Create a valid JSON manifest that's fresh
	m := &Manifest{
		Version:   "1.0.0",
		Platform:  "test/amd64",
		Timestamp: time.Now(), // fresh!
		Roots:     []string{tmpDir},
		Dirs:      map[string]DirSummary{tmpDir: {TotalSize: 42}},
		Stats:     WalkStats{DirsWalked: 1, FilesIndexed: 1},
	}
	data, _ := json.Marshal(m)
	os.WriteFile(jsonPath, data, 0o644)

	// No gob file exists — should fall back to JSON
	loaded, err := Index(IndexOptions{
		Roots:     []string{tmpDir},
		CachePath: cachePath,
		TTL:       1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Index JSON fallback failed: %v", err)
	}
	if loaded.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0 from JSON fallback, got %s", loaded.Version)
	}
}

// ── Index with expired cache → fresh build ──────────────────────────────

func TestIndex_ExpiredCacheRebuild(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "manifest.gob")
	dataDir := filepath.Join(tmpDir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "fresh.txt"), []byte("fresh"), 0o644)

	// Write an expired gob cache
	old := &Manifest{
		Version:   "1.0.0",
		Timestamp: time.Now().Add(-1 * time.Hour), // expired
		Roots:     []string{dataDir},
		Dirs:      map[string]DirSummary{},
		Stats:     WalkStats{DirsWalked: 0, FilesIndexed: 0},
	}
	if err := SaveManifest(cachePath, old); err != nil {
		t.Fatal(err)
	}

	// Index with short TTL — cache is expired, should rebuild
	m, err := Index(IndexOptions{
		Roots:     []string{dataDir},
		CachePath: cachePath,
		TTL:       1 * time.Second,
	})
	if err != nil {
		t.Fatalf("Index rebuild failed: %v", err)
	}
	if m.Version != "2.0.0" {
		t.Errorf("Expected fresh build (v2.0.0), got %s", m.Version)
	}
	if m.Stats.FilesIndexed < 1 {
		t.Error("Expected at least 1 file after rebuild")
	}
}

// ── SaveManifest error path ─────────────────────────────────────────────

func TestSaveManifest_ErrorPaths(t *testing.T) {
	m := &Manifest{
		Version: "2.0.0",
		Dirs:    map[string]DirSummary{"/test": {}},
	}

	// Successful save + load round-trip
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "deep", "nested", "manifest.gob")
	if err := SaveManifest(savePath, m); err != nil {
		t.Fatalf("SaveManifest failed: %v", err)
	}

	loaded, err := LoadManifest(savePath)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if loaded.Version != "2.0.0" {
		t.Errorf("Round-trip: expected 2.0.0, got %s", loaded.Version)
	}
}

// ── LoadManifest error paths ────────────────────────────────────────────

func TestLoadManifest_Errors(t *testing.T) {
	// Missing file
	_, err := LoadManifest("/nonexistent/path/manifest.gob")
	if err == nil {
		t.Error("Expected error for missing file")
	}

	// Corrupt gob
	tmpDir := t.TempDir()
	corruptPath := filepath.Join(tmpDir, "corrupt.gob")
	os.WriteFile(corruptPath, []byte("not valid gob data"), 0o644)
	_, err = LoadManifest(corruptPath)
	if err == nil {
		t.Error("Expected error for corrupt gob")
	}
}

// ── buildIndex with nonexistent roots ───────────────────────────────────

func TestBuildIndex_NonexistentRoots(t *testing.T) {
	m, err := buildIndex(IndexOptions{
		Roots:    []string{"/nonexistent/root/abc123"},
		MaxDepth: 3,
	})
	if err != nil {
		t.Fatalf("buildIndex with nonexistent root should succeed, got: %v", err)
	}
	if m.Stats.FilesIndexed != 0 {
		t.Errorf("Expected 0 files indexed for nonexistent root, got %d", m.Stats.FilesIndexed)
	}
}

// ── buildIndex with explicit parallelism ────────────────────────────────

func TestBuildIndex_Parallelism(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "p")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "test.txt"), []byte("test"), 0o644)

	m, err := buildIndex(IndexOptions{
		Roots:       []string{dataDir},
		MaxDepth:    3,
		Parallelism: 2,
	})
	if err != nil {
		t.Fatalf("buildIndex failed: %v", err)
	}
	if m.Stats.Parallelism != 2 {
		t.Errorf("Parallelism: expected 2, got %d", m.Stats.Parallelism)
	}
}

// ── Index defaults ──────────────────────────────────────────────────────

func TestIndex_DefaultOptions(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "defaults")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, "f.txt"), []byte("hi"), 0o644)

	m, err := buildIndex(IndexOptions{
		Roots: []string{dataDir},
		// MaxDepth 0 → DefaultMaxDepth
		// Parallelism 0 → GOMAXPROCS
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Stats.Parallelism != runtime.GOMAXPROCS(0) {
		t.Errorf("Expected default parallelism %d, got %d", runtime.GOMAXPROCS(0), m.Stats.Parallelism)
	}
}
