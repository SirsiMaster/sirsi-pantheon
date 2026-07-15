package guard

import "testing"

func TestCheckLocalSnapshots(t *testing.T) {
	orig := localSnapshotsFn
	defer func() { localSnapshotsFn = orig }()

	// No reclaimable snapshots → no finding (never invent a row).
	localSnapshotsFn = func(string) []string { return nil }
	r := &DoctorReport{}
	checkLocalSnapshots(r)
	if len(r.Findings) != 0 {
		t.Fatalf("no snapshots should produce no finding, got %d", len(r.Findings))
	}

	// Snapshots present → one INFO finding (never an alarm) carrying the reclaim.
	localSnapshotsFn = func(string) []string {
		return []string{
			"com.apple.TimeMachine.2026-06-30-120000.local",
			"com.apple.TimeMachine.2026-06-29-120000.local",
		}
	}
	r = &DoctorReport{}
	checkLocalSnapshots(r)
	if len(r.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(r.Findings))
	}
	f := r.Findings[0]
	if f.Check != "Local Snapshots" || f.Severity != SeverityInfo {
		t.Fatalf("want Local Snapshots/Info, got %s/%v", f.Check, f.Severity)
	}
	if got := remediationCommand(f); got != "sirsi reclaim-snapshots" {
		t.Fatalf("Local Snapshots must offer the reclaim action, got %q", got)
	}
	if got := remediationKind(f); got != FixInstant {
		t.Fatalf("want FixInstant, got %v", got)
	}
}
