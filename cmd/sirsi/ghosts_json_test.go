package main

// Tests for the structured `sirsi ghosts --json` contract (TUI design proof
// gap V3). NOTE: no t.Parallel() — sibling tests in this package swap
// package globals (repo lessons #129/#131).

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ka"
)

// TestBuildGhostReport_Shape pins the structured contract: real residual
// paths/kinds/sizes (not display strings), deterministic size-descending
// order, and the exact wire keys the TUI/menubar/dashboard decoders use.
func TestBuildGhostReport_Shape(t *testing.T) {
	ghosts := []ka.Ghost{
		{
			AppName:  "Smallapp",
			BundleID: "com.example.smallapp",
			Residuals: []ka.Residual{
				{Path: "/Users/x/Library/Preferences/com.example.smallapp.plist", Type: ka.ResidualPreferences, SizeBytes: 1024, FileCount: 1},
			},
			TotalSize:  1024,
			TotalFiles: 1,
		},
		{
			AppName:  "Bigapp",
			BundleID: "com.example.bigapp",
			Residuals: []ka.Residual{
				{Path: "/Users/x/Library/Caches/com.example.bigapp", Type: ka.ResidualCaches, SizeBytes: 2 << 20, FileCount: 40},
				{Path: "/Users/x/Library/Application Support/Bigapp", Type: ka.ResidualAppSupport, SizeBytes: 1 << 20, FileCount: 12},
			},
			TotalSize:        3 << 20,
			TotalFiles:       52,
			InLaunchServices: true,
		},
	}

	r := buildGhostReport(ghosts, nil)

	if r.Command != "sirsi ghosts" {
		t.Errorf("command = %q", r.Command)
	}
	if r.GhostCount != 2 || len(r.Ghosts) != 2 {
		t.Fatalf("ghost_count = %d / %d ghosts", r.GhostCount, len(r.Ghosts))
	}
	if want := int64(3<<20 + 1024); r.TotalWasteBytes != want {
		t.Errorf("total_waste_bytes = %d, want %d", r.TotalWasteBytes, want)
	}
	if r.TotalWaste == "" || r.Summary == "" {
		t.Errorf("human fields must be populated: waste=%q summary=%q", r.TotalWaste, r.Summary)
	}
	// Deterministic order: largest waste first (the scanner merges from a map).
	if r.Ghosts[0].AppName != "Bigapp" {
		t.Errorf("ghosts[0] = %q, want Bigapp (size-descending order)", r.Ghosts[0].AppName)
	}
	g := r.Ghosts[0]
	if g.BundleID != "com.example.bigapp" || !g.InLaunchServices || g.TotalFiles != 52 {
		t.Errorf("ghost fields lost in translation: %+v", g)
	}
	if len(g.Residuals) != 2 {
		t.Fatalf("residuals = %d, want 2", len(g.Residuals))
	}
	res := g.Residuals[0]
	if res.Path != "/Users/x/Library/Caches/com.example.bigapp" || res.Type != string(ka.ResidualCaches) || res.SizeBytes != 2<<20 || res.FileCount != 40 {
		t.Errorf("residual = %+v — the contract needs real paths/kinds/sizes", res)
	}

	// Pin the wire keys.
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"command", "summary", "ghost_count", "total_waste_bytes", "total_waste", "ghosts"} {
		if _, ok := wire[key]; !ok {
			t.Errorf("JSON contract missing key %q", key)
		}
	}
	ghostsArr := wire["ghosts"].([]interface{})
	entry := ghostsArr[0].(map[string]interface{})
	for _, key := range []string{"app_name", "bundle_id", "total_size_bytes", "total_files", "in_launch_services", "residuals"} {
		if _, ok := entry[key]; !ok {
			t.Errorf("ghost entry missing key %q", key)
		}
	}
	residual := entry["residuals"].([]interface{})[0].(map[string]interface{})
	for _, key := range []string{"path", "type", "size_bytes", "file_count"} {
		if _, ok := residual[key]; !ok {
			t.Errorf("residual entry missing key %q", key)
		}
	}
}

// TestBuildGhostReport_EmptyAndErrors: a clean machine yields ghosts:[] (never
// null) and a scan error surfaces as a warning, not a silent omission.
func TestBuildGhostReport_EmptyAndErrors(t *testing.T) {
	r := buildGhostReport(nil, nil)
	if r.GhostCount != 0 || r.TotalWasteBytes != 0 {
		t.Errorf("empty scan: count=%d waste=%d", r.GhostCount, r.TotalWasteBytes)
	}
	if r.Summary != "No ghost app remnants detected" {
		t.Errorf("summary = %q", r.Summary)
	}
	b, _ := json.Marshal(r)
	var wire map[string]interface{}
	_ = json.Unmarshal(b, &wire)
	if _, isArray := wire["ghosts"].([]interface{}); !isArray {
		t.Errorf("ghosts must serialize as [] (got %T)", wire["ghosts"])
	}

	// A ghost with no residuals (launch-services-only) still emits residuals:[].
	r2 := buildGhostReport([]ka.Ghost{{AppName: "Lsonly", BundleID: "com.example.lsonly", InLaunchServices: true}}, errors.New("lsregister timed out"))
	if len(r2.Warnings) != 1 {
		t.Fatalf("scan error must surface as a warning, got %v", r2.Warnings)
	}
	b2, _ := json.Marshal(r2)
	var wire2 struct {
		Ghosts []map[string]interface{} `json:"ghosts"`
	}
	_ = json.Unmarshal(b2, &wire2)
	if _, isArray := wire2.Ghosts[0]["residuals"].([]interface{}); !isArray {
		t.Errorf("residuals must serialize as [] (got %T)", wire2.Ghosts[0]["residuals"])
	}
}
