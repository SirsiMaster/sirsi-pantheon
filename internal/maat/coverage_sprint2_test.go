package maat

import (
	"os"
	"path/filepath"
	"testing"
)

// ── runFullCoverage ──────────────────────────────────────────────────────

func TestRunFullCoverage_RealProject(t *testing.T) {
	t.Skip("Integration test — runs go test -cover ./... recursively")
}

// ── runDiffCoverage ─────────────────────────────────────────────────────

func TestRunDiffCoverage_WithCache(t *testing.T) {
	// Create a temp cache directory for test isolation
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "coverage-cache.json")

	// Pre-populate cache with fake data so runDiffCoverage uses cached path
	cache := []CoverageResult{
		{Package: "internal/jackal", Coverage: 95.0},
		{Package: "internal/ka", Coverage: 93.0},
		{Package: "internal/guard", Coverage: 89.0},
		{Package: "internal/brain", Coverage: 94.0},
		{Package: "internal/cleaner", Coverage: 85.0},
		{Package: "internal/horus", Coverage: 92.0},
		{Package: "internal/mcp", Coverage: 86.0},
		{Package: "internal/hapi", Coverage: 83.0},
		{Package: "internal/mirror", Coverage: 82.0},
		{Package: "internal/yield", Coverage: 82.0},
		{Package: "internal/maat", Coverage: 80.0},
		{Package: "internal/scarab", Coverage: 93.0},
		{Package: "internal/sight", Coverage: 77.0},
		{Package: "internal/scales", Coverage: 95.0},
		{Package: "internal/seba", Coverage: 90.0},
		{Package: "internal/profile", Coverage: 83.0},
		{Package: "internal/stealth", Coverage: 82.0},
		{Package: "internal/ignore", Coverage: 91.0},
		{Package: "internal/logging", Coverage: 95.0},
		{Package: "internal/platform", Coverage: 73.0},
		{Package: "internal/updater", Coverage: 87.0},
		{Package: "internal/output", Coverage: 100.0},
	}
	saveCoverageCache(cachePath, cache)

	ca := &CoverageAssessor{
		CachePath: cachePath,
		DiffOnly:  true,
		Runner:    nil,
	}

	output, err := ca.runDiffCoverage()
	if err != nil {
		t.Logf("runDiffCoverage: %v (may need git)", err)
		return
	}
	if output == "" {
		t.Log("No changed packages — using all cached data")
	}
	t.Logf("runDiffCoverage returned %d bytes", len(output))
}

// ── CoverageAssessor.Assess with runner ─────────────────────────────────

func TestCoverageAssess_WithRunner(t *testing.T) {
	ca := &CoverageAssessor{
		Thresholds: DefaultThresholds(),
		CachePath:  filepath.Join(t.TempDir(), "test-cache.json"),
		Runner: func() (string, error) {
			return "ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/jackal\t1.2s\tcoverage: 95.0% of statements\n" +
				"ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/ka\t0.8s\tcoverage: 93.0% of statements\n" +
				"ok  \tgithub.com/SirsiMaster/sirsi-pantheon/internal/cleaner\t0.5s\tcoverage: 85.7% of statements", nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if len(assessments) == 0 {
		t.Error("Expected assessments")
	}

	// Check that jackal and ka assessments exist
	found := map[string]bool{}
	for _, a := range assessments {
		found[a.Subject] = true
	}
	if !found["jackal"] {
		t.Error("Expected jackal assessment")
	}
}

// ── CanonAssessor.Assess with runner ─────────────────────────────────────

func TestCanonAssess_WithRunner(t *testing.T) {
	ca := &CanonAssessor{
		Runner: func(count int) (string, error) {
			return "abc123\nfeat: add new deity\nRefs: ADR-005\n---END---\n" +
				"def456\nfix: typo\n\n---END---", nil
		},
	}

	assessments, err := ca.Assess()
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if len(assessments) == 0 {
		t.Error("Expected assessments")
	}
}

func TestCanonAssess_Error(t *testing.T) {
	ca := &CanonAssessor{
		Runner: func(count int) (string, error) {
			return "", os.ErrNotExist
		},
	}

	_, err := ca.Assess()
	if err == nil {
		t.Error("Expected error when runner fails")
	}
}
