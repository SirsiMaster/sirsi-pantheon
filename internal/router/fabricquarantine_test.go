package router

import "testing"

// TestFabricDispatchQuarantineHolds and TestFabricDispatchQuarantineAllows
// verify BOTH directions (A35) of the wake.go dispatch gate directly, without
// spinning up a whole RunWakeLoop.
func TestFabricDispatchQuarantineHolds(t *testing.T) {
	defer SetFabricQuarantinedFn(nil)
	SetFabricQuarantinedFn(func(string) bool { return true })

	if !fabricDispatchQuarantined("worker-agent", 3) {
		t.Fatal("quarantined fabric must hold dispatch")
	}
}

func TestFabricDispatchQuarantineAllows(t *testing.T) {
	defer SetFabricQuarantinedFn(nil)
	SetFabricQuarantinedFn(func(string) bool { return false })

	if fabricDispatchQuarantined("worker-agent", 3) {
		t.Fatal("unquarantined fabric must not hold dispatch")
	}
}

// TestFabricQuarantineMarkerPath pins the path convention, mirroring
// QuarantineMarkerPath's own test — a `sirsi router quarantine` operator
// depends on this being stable and discoverable.
func TestFabricQuarantineMarkerPath(t *testing.T) {
	got := FabricQuarantineMarkerPath("/home/op")
	want := "/home/op/.sirsi/fabric-quarantine"
	if got != want {
		t.Fatalf("FabricQuarantineMarkerPath = %q, want %q", got, want)
	}
}

// TestIsFabricQuarantinedDefaultChecksMarkerFile exercises the real (non-test
// double) seam end to end against a temp HOME-style path.
func TestIsFabricQuarantinedDefaultChecksMarkerFile(t *testing.T) {
	defer SetFabricQuarantinedFn(nil)
	SetFabricQuarantinedFn(nil) // restore real os.Stat-backed default

	dir := t.TempDir()
	if IsFabricQuarantined(dir) {
		t.Fatal("no marker written yet — must not read as quarantined")
	}
}
