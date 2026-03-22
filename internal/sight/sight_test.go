package sight

import (
	"runtime"
	"testing"
)

// ═══════════════════════════════════════════
// parseLSRegisterDump — pure function tests
// ═══════════════════════════════════════════

func TestParseLSRegisterDump_EmptyInput(t *testing.T) {
	ghosts := parseLSRegisterDump("")
	if len(ghosts) != 0 {
		t.Errorf("expected 0 ghosts for empty input, got %d", len(ghosts))
	}
}

func TestParseLSRegisterDump_SkipsAppleApps(t *testing.T) {
	// Apple system apps should be excluded
	dump := `--------------------------------------------------------------------------------
	bundle id: com.apple.Safari
	path: /Applications/Safari.app
	name: Safari
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump)
	for _, g := range ghosts {
		if g.BundleID == "com.apple.Safari" {
			t.Error("com.apple.* apps should be excluded")
		}
	}
}

func TestParseLSRegisterDump_NoAppPath(t *testing.T) {
	// Entries without .app should be skipped
	dump := `--------------------------------------------------------------------------------
	bundle id: com.example.daemon
	path: /usr/local/bin/example
	name: ExampleDaemon
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump)
	if len(ghosts) != 0 {
		t.Errorf("expected 0 ghosts for non-.app entries, got %d", len(ghosts))
	}
}

func TestParseLSRegisterDump_MissingBundleID(t *testing.T) {
	dump := `--------------------------------------------------------------------------------
	path: /Applications/Missing.app
	name: Missing
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump)
	if len(ghosts) != 0 {
		t.Errorf("expected 0 ghosts when bundle id missing, got %d", len(ghosts))
	}
}

func TestParseLSRegisterDump_Deduplication(t *testing.T) {
	// Same bundle ID should only appear once
	dump := `--------------------------------------------------------------------------------
	bundle id: com.example.app
	path: /Applications/Example.app
	name: Example
--------------------------------------------------------------------------------
	bundle id: com.example.app
	path: /Applications/Example.app/Contents/MacOS/helper
	name: Example
--------------------------------------------------------------------------------`
	ghosts := parseLSRegisterDump(dump)
	count := 0
	for _, g := range ghosts {
		if g.BundleID == "com.example.app" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("duplicate bundleID should be deduplicated, got %d", count)
	}
}

// ═══════════════════════════════════════════
// GhostRegistration struct
// ═══════════════════════════════════════════

func TestGhostRegistration_Struct(t *testing.T) {
	g := GhostRegistration{
		BundleID: "com.parallels.desktop.console",
		Path:     "/Applications/Parallels Desktop.app",
		Name:     "Parallels Desktop",
	}
	if g.BundleID == "" {
		t.Error("BundleID should not be empty")
	}
	if g.Path == "" {
		t.Error("Path should not be empty")
	}
}

func TestSightResult_Defaults(t *testing.T) {
	r := SightResult{}
	if r.CanFix {
		t.Error("CanFix should default to false")
	}
	if r.TotalGhosts != 0 {
		t.Error("TotalGhosts should default to 0")
	}
}

// ═══════════════════════════════════════════
// Platform guard
// ═══════════════════════════════════════════

func TestFix_DryRunSafe(t *testing.T) {
	err := Fix(true)
	if runtime.GOOS != "darwin" {
		if err == nil {
			t.Error("Fix() should return error on non-darwin")
		}
	} else {
		if err != nil {
			t.Errorf("Fix(dryRun=true) should succeed on darwin, got: %v", err)
		}
	}
}

func TestReindexSpotlight_DryRunSafe(t *testing.T) {
	err := ReindexSpotlight(true)
	if runtime.GOOS != "darwin" {
		if err == nil {
			t.Error("ReindexSpotlight() should return error on non-darwin")
		}
	} else {
		if err != nil {
			t.Errorf("ReindexSpotlight(dryRun=true) should succeed on darwin, got: %v", err)
		}
	}
}
