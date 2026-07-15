package maat

// Fast mode (SkipTests — the DEFAULT for `sirsi audit`) must NEVER run the
// test suite. The old cold-cache fallback silently upgraded a ~2s verb into a
// 5+ minute full `go test -cover ./...` (2026-07-04: blew the UX contract's
// 120s budget, stalled surfaces that shell audit, blocked the pre-push gate).

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestFastModeNeverRunsTheSuite: with a COLD cache, SkipTests must return
// honest cold-cache warnings — and must not touch the Runner (the injected
// stand-in for any real scan).
func TestFastModeNeverRunsTheSuite(t *testing.T) {
	c := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
			{Module: "guard", MinCoverage: 80},
		},
		CachePath: filepath.Join(t.TempDir(), "coverage-cache.json"), // absent = cold
		SkipTests: true,
		Runner: func() (string, error) {
			t.Fatal("fast mode ran a coverage scan — the silent full-suite upgrade is back")
			return "", nil
		},
	}
	assessments, err := c.Assess()
	if err != nil {
		t.Fatal(err)
	}
	if len(assessments) != 2 {
		t.Fatalf("expected 2 assessments, got %d", len(assessments))
	}
	for _, a := range assessments {
		if a.Verdict != VerdictWarning {
			t.Fatalf("%s: cold cache must be an honest WARNING (unmeasured ≠ unhealthy), got %v", a.Subject, a.Verdict)
		}
		if !strings.Contains(a.Message, "cache cold") {
			t.Fatalf("%s: message must name the cold cache, got %q", a.Subject, a.Message)
		}
		if !strings.Contains(a.Remediation, "--full") {
			t.Fatalf("%s: remediation must point at --full, got %q", a.Subject, a.Remediation)
		}
	}
}

// TestFastModePartialCacheMixesHonestly: cached modules score normally;
// uncached ones warn as cold — still with zero scans.
func TestFastModePartialCacheMixesHonestly(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "coverage-cache.json")
	if err := saveCoverageCache(cachePath, []CoverageResult{{Package: "cleaner", Coverage: 92.5}}); err != nil {
		t.Fatal(err)
	}
	c := &CoverageAssessor{
		Thresholds: []CoverageThreshold{
			{Module: "cleaner", MinCoverage: 80, SafetyCritical: true},
			{Module: "guard", MinCoverage: 80},
		},
		CachePath: cachePath,
		SkipTests: true,
		Runner: func() (string, error) {
			t.Fatal("fast mode ran a coverage scan on a partial cache")
			return "", nil
		},
	}
	assessments, err := c.Assess()
	if err != nil {
		t.Fatal(err)
	}
	byModule := map[string]Assessment{}
	for _, a := range assessments {
		byModule[a.Subject] = a
	}
	if got := byModule["cleaner"].Verdict; got != VerdictPass {
		t.Fatalf("cached cleaner@92.5%% must PASS its 80%% threshold, got %v", got)
	}
	if got := byModule["guard"]; got.Verdict != VerdictWarning || !strings.Contains(got.Message, "cache cold") {
		t.Fatalf("uncached guard must warn cold, got %v %q", got.Verdict, got.Message)
	}
}

// TestFastModeDoesNotRewriteTheCache: a fast run must not save its own
// round-tripped parse back over the cache file.
func TestFastModeDoesNotRewriteTheCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "coverage-cache.json")
	seed := []CoverageResult{{Package: "cleaner", Coverage: 92.5}, {Package: "vault", Coverage: 71.0}}
	if err := saveCoverageCache(cachePath, seed); err != nil {
		t.Fatal(err)
	}
	c := &CoverageAssessor{
		Thresholds: []CoverageThreshold{{Module: "cleaner", MinCoverage: 80}},
		CachePath:  cachePath,
		SkipTests:  true,
	}
	if _, err := c.Assess(); err != nil {
		t.Fatal(err)
	}
	after, err := loadCoverageCache(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(seed) {
		t.Fatalf("fast mode rewrote the cache: %d entries, want %d", len(after), len(seed))
	}
}
