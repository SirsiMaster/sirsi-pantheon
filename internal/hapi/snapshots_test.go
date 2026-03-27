package hapi

import (
	"runtime"
	"testing"
)

// ── ListSnapshots ────────────────────────────────────────────────────────

func TestListSnapshots_FieldValidation(t *testing.T) {
	result, err := ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Total != len(result.Snapshots) {
		t.Errorf("Total=%d but len(Snapshots)=%d", result.Total, len(result.Snapshots))
	}
	t.Logf("Found %d snapshots", result.Total)

	// Verify snapshot fields if any found
	for _, snap := range result.Snapshots {
		if snap.Name == "" {
			t.Error("Snapshot Name should not be empty")
		}
		if snap.Volume == "" {
			t.Error("Snapshot Volume should not be empty")
		}
	}
}

func TestListSnapshots_Fields(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}

	result, err := ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}

	// On macOS, should return a valid result
	if result == nil {
		t.Fatal("Expected result on macOS")
	}

	// Verify TimeMachine date extraction
	for _, snap := range result.Snapshots {
		if snap.Date == "" && snap.Name != "" {
			t.Logf("Snapshot %q has no date (may not be TimeMachine format)", snap.Name)
		}
	}
}

// ── PruneSnapshot ────────────────────────────────────────────────────────

func TestPruneSnapshot_DryRunSafe(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}

	// Dry run should succeed without deleting anything
	err := PruneSnapshot("test-snapshot-that-does-not-exist", true)
	if err != nil {
		t.Errorf("PruneSnapshot dry run should not error: %v", err)
	}
}

func TestPruneSnapshot_NonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Testing non-darwin codepath")
	}

	err := PruneSnapshot("test-snapshot", false)
	if err == nil {
		t.Error("PruneSnapshot should error on non-darwin")
	}
}

func TestPruneSnapshot_InvalidName(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}

	// This WILL fail because the snapshot doesn't exist — that's expected
	err := PruneSnapshot("nonexistent-timestamp", false)
	if err == nil {
		t.Error("PruneSnapshot should error for nonexistent snapshot")
	}
}

// ── Snapshot struct ──────────────────────────────────────────────────────

func TestSnapshotStruct(t *testing.T) {
	snap := Snapshot{
		Name:   "com.apple.TimeMachine.2026-03-21-120000.local",
		Date:   "2026-03-21-120000",
		Size:   "1.2 GB",
		Volume: "/",
	}
	if snap.Name == "" {
		t.Error("Name empty")
	}
	if snap.Date == "" {
		t.Error("Date empty")
	}
	if snap.Size != "1.2 GB" {
		t.Errorf("Size = %q, want 1.2 GB", snap.Size)
	}
	if snap.Volume != "/" {
		t.Errorf("Volume = %q, want /", snap.Volume)
	}
}

func TestSnapshotResult_Empty(t *testing.T) {
	r := SnapshotResult{}
	if r.Total != 0 {
		t.Error("Empty result should have total 0")
	}
	if len(r.Snapshots) != 0 {
		t.Error("Empty result should have no snapshots")
	}
}
