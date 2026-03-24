package maat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ── DefaultThresholds ────────────────────────────────────────────────────

func TestDefaultThresholds(t *testing.T) {
	thresholds := DefaultThresholds()
	if len(thresholds) == 0 {
		t.Fatal("DefaultThresholds should not be empty")
	}

	// Verify safety-critical modules are marked
	criticalModules := map[string]bool{"cleaner": false, "guard": false}
	for _, th := range thresholds {
		if th.Module == "" {
			t.Error("Threshold module should not be empty")
		}
		if th.MinCoverage <= 0 {
			t.Errorf("Threshold for %s has invalid min coverage: %.1f", th.Module, th.MinCoverage)
		}
		if _, ok := criticalModules[th.Module]; ok {
			criticalModules[th.Module] = th.SafetyCritical
		}
	}

	for mod, isCritical := range criticalModules {
		if !isCritical {
			t.Errorf("Module %s should be marked safety-critical", mod)
		}
	}
}

// ── DefaultCanonDocuments ────────────────────────────────────────────────

func TestDefaultCanonDocuments(t *testing.T) {
	docs := DefaultCanonDocuments()
	if len(docs) == 0 {
		t.Fatal("DefaultCanonDocuments should not be empty")
	}
	for _, doc := range docs {
		if doc.Name == "" {
			t.Error("Canon document Name should not be empty")
		}
		if doc.Path == "" {
			t.Error("Canon document Path should not be empty")
		}
	}
}

// ── cacheCoversThresholds ────────────────────────────────────────────────

func TestCacheCoversThresholds_Empty(t *testing.T) {
	c := &CoverageAssessor{
		Thresholds: DefaultThresholds(),
	}
	// Empty cache should return false
	if c.cacheCoversThresholds(nil) {
		t.Error("Empty cache should not cover thresholds")
	}
	if c.cacheCoversThresholds([]CoverageResult{}) {
		t.Error("Empty results should not cover thresholds")
	}
}

func TestCacheCoversThresholds_Partial(t *testing.T) {
	c := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80},
			{Module: "guard", MinCoverage: 60},
		},
	}
	// Only one of two modules cached
	partial := []CoverageResult{{Package: "cleaner", Coverage: 80.0}}
	if c.cacheCoversThresholds(partial) {
		t.Error("Partial cache should not cover all thresholds")
	}
}

func TestCacheCoversThresholds_Complete(t *testing.T) {
	c := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80},
			{Module: "guard", MinCoverage: 60},
		},
	}
	complete := []CoverageResult{
		{Package: "cleaner", Coverage: 80.0},
		{Package: "guard", Coverage: 65.0},
	}
	if !c.cacheCoversThresholds(complete) {
		t.Error("Complete cache should cover all thresholds")
	}
}

// ── loadCoverageCache / saveCoverageCache ────────────────────────────────

func TestCoverageCache_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "cache.json")

	results := []CoverageResult{
		{Package: "cleaner", Coverage: 80.4},
		{Package: "guard", Coverage: 89.0, NoTests: false},
		{Package: "output", NoTests: true},
	}

	// Save
	err := saveCoverageCache(path, results)
	if err != nil {
		t.Fatalf("saveCoverageCache: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Cache file not created: %v", err)
	}

	// Load
	loaded, err := loadCoverageCache(path)
	if err != nil {
		t.Fatalf("loadCoverageCache: %v", err)
	}

	if len(loaded) != len(results) {
		t.Fatalf("Loaded %d results, want %d", len(loaded), len(results))
	}
	if loaded[0].Package != "cleaner" || loaded[0].Coverage != 80.4 {
		t.Errorf("First result: %+v", loaded[0])
	}
	if !loaded[2].NoTests {
		t.Error("Third result should have NoTests=true")
	}
}

func TestLoadCoverageCache_NotFound(t *testing.T) {
	_, err := loadCoverageCache("/nonexistent/cache.json")
	if err == nil {
		t.Error("Expected error for nonexistent cache")
	}
}

func TestLoadCoverageCache_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json"), 0o644)

	_, err := loadCoverageCache(path)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// ── changedPackages ──────────────────────────────────────────────────────

func TestChangedPackages_DefaultBase(t *testing.T) {
	c := &CoverageAssessor{
		ProjectRoot: "/tmp/nonexistent-repo",
	}
	// Git diff will fail for nonexistent repo — should return nil
	pkgs := c.changedPackages()
	if pkgs != nil {
		t.Errorf("Expected nil for nonexistent repo, got %v", pkgs)
	}
}

func TestChangedPackages_WithBase(t *testing.T) {
	c := &CoverageAssessor{
		DiffBase: "HEAD~5",
	}
	// Should not panic regardless of result
	_ = c.changedPackages()
}

// ── coverageCachePath ────────────────────────────────────────────────────

func TestCoverageCachePath_Custom(t *testing.T) {
	c := &CoverageAssessor{CachePath: "/custom/path/cache.json"}
	if c.coverageCachePath() != "/custom/path/cache.json" {
		t.Errorf("Custom cache path = %q", c.coverageCachePath())
	}
}

func TestCoverageCachePath_Default(t *testing.T) {
	c := &CoverageAssessor{}
	path := c.coverageCachePath()
	if path == "" {
		t.Error("Default cache path should not be empty")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("Cache path should be absolute, got %q", path)
	}
}

// ── coverageCacheEntry JSON ──────────────────────────────────────────────

func TestCoverageCacheEntry_JSON(t *testing.T) {
	entry := coverageCacheEntry{
		Results: []CoverageResult{
			{Package: "brain", Coverage: 94.6},
		},
		Timestamp: "2026-03-24T17:00:00Z",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var parsed coverageCacheEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(parsed.Results) != 1 || parsed.Results[0].Package != "brain" {
		t.Errorf("Round-trip failed: %+v", parsed)
	}
}
