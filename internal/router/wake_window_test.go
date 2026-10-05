package router

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMeasurementWindowHoldsDispatch: while rails.lock exists the wake loop must
// not dispatch a consumer (it would invalidate the measurement); without it,
// dispatch proceeds. Both directions (A35).
func TestMeasurementWindowHoldsDispatch(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "rails.lock")
	t.Setenv("MAAT_RAILS_LOCK", lock)

	if measurementWindowOpen("claude-inference", 3) {
		t.Fatal("no rails.lock: window reported open, dispatch would be wrongly held")
	}
	if err := os.WriteFile(lock, []byte("mercury\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !measurementWindowOpen("claude-inference", 3) {
		t.Fatal("rails.lock present: window reported closed, a consumer would contaminate the run")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if measurementWindowOpen("claude-inference", 3) {
		t.Fatal("rails.lock removed: dispatch still held after the window closed")
	}
}
