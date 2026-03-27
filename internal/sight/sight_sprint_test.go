package sight

import (
	"runtime"
	"testing"
)

// ── Fix ──────────────────────────────────────────────────────────────────

func TestFix_DryRun(t *testing.T) {
	// Dry run should never modify the system
	err := Fix(true)
	if runtime.GOOS != "darwin" {
		if err == nil {
			t.Error("Fix should fail on non-darwin")
		}
		return
	}
	if err != nil {
		t.Errorf("Fix(dryRun=true) should not error on macOS: %v", err)
	}
}

func TestFix_NonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		// On darwin we'd actually try to rebuild — skip
		// (we test the dry run path above)
		t.Skip("Not testing live Fix on macOS")
	}
	err := Fix(false)
	if err == nil {
		t.Error("Fix should error on non-darwin")
	}
}

// ── ReindexSpotlight ─────────────────────────────────────────────────────

func TestReindexSpotlight_DryRun(t *testing.T) {
	err := ReindexSpotlight(true)
	if runtime.GOOS != "darwin" {
		if err == nil {
			t.Error("ReindexSpotlight should fail on non-darwin")
		}
		return
	}
	if err != nil {
		t.Errorf("ReindexSpotlight(dryRun=true) should not error on macOS: %v", err)
	}
}

func TestReindexSpotlight_NonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Not testing live Spotlight reindex on macOS")
	}
	err := ReindexSpotlight(false)
	if err == nil {
		t.Error("ReindexSpotlight should error on non-darwin")
	}
}

// ── SightResult Struct ───────────────────────────────────────────────────

func TestSightResult_Fields(t *testing.T) {
	r := SightResult{
		GhostRegistrations: []GhostRegistration{
			{BundleID: "com.test.app", Path: "/Applications/Test.app", Name: "Test"},
		},
		TotalGhosts:        1,
		LaunchServicesSize: 1024 * 1024,
		CanFix:             true,
	}

	if r.TotalGhosts != 1 {
		t.Errorf("TotalGhosts = %d, want 1", r.TotalGhosts)
	}
	if r.LaunchServicesSize != 1024*1024 {
		t.Errorf("LaunchServicesSize = %d, want %d", r.LaunchServicesSize, 1024*1024)
	}
	if r.GhostRegistrations[0].BundleID != "com.test.app" {
		t.Error("BundleID mismatch")
	}
	if !r.CanFix {
		t.Error("CanFix should be true")
	}
}

// ── GhostRegistration ────────────────────────────────────────────────────

func TestGhostRegistration_Fields(t *testing.T) {
	g := GhostRegistration{
		BundleID: "com.example.ghostapp",
		Path:     "/Applications/GhostApp.app",
		Name:     "GhostApp",
	}

	if g.BundleID != "com.example.ghostapp" {
		t.Errorf("BundleID = %q", g.BundleID)
	}
	if g.Path != "/Applications/GhostApp.app" {
		t.Errorf("Path = %q", g.Path)
	}
	if g.Name != "GhostApp" {
		t.Errorf("Name = %q", g.Name)
	}
}
