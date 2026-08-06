package guard

import "testing"

// The 16 GB target machine must be exercisable from a 48 GB dev box, so the
// thresholds are fractions of RAM rather than absolute bytes. This pins that a
// footprint which is unremarkable on a workstation is CRITICAL on the machine
// Sirsi actually targets.
func TestFootprintThresholdsScaleWithMachine(t *testing.T) {
	const gib = int64(1) << 30
	cases := []struct {
		name     string
		ram      int64
		worst    int64
		wantCrit bool
		wantWarn bool
	}{
		{"6 GB on a 48 GB workstation is fine", 48 * gib, 6 * gib, false, false},
		// The scaling point: identical footprint, opposite verdicts.
		{"6 GB on a 16 GB laptop WARNS (38%)", 16 * gib, 6 * gib, false, true},
		{"9 GB on a 16 GB laptop is CRITICAL (56%)", 16 * gib, 9 * gib, true, false},
		{"the real 2026-07-27 broker: 40.5 GB of 48 GB", 48 * gib, 40 * gib, true, false},
		{"a third of the machine warns", 48 * gib, 17 * gib, false, true},
	}
	for _, c := range cases {
		frac := float64(c.worst) / float64(c.ram)
		gotCrit := frac >= footprintCriticalFraction
		gotWarn := !gotCrit && frac >= footprintWarnFraction
		if gotCrit != c.wantCrit || gotWarn != c.wantWarn {
			t.Errorf("%s: frac=%.2f crit=%v warn=%v, want crit=%v warn=%v",
				c.name, frac, gotCrit, gotWarn, c.wantCrit, c.wantWarn)
		}
	}
}

// memSize is the single chokepoint that keeps RSS from being read directly.
// Reading .RSS is how the gemma broker stayed invisible through three OOMs, so
// the fallback must only engage when footprint is genuinely unavailable.
func TestMemSizePrefersFootprint(t *testing.T) {
	const gib = int64(1) << 30
	// The real divergence measured on the broker: 4.71 GB resident, 29.4 GB footprint.
	if got := memSize(ProcessInfo{RSS: 4 * gib, Footprint: 29 * gib}); got != 29*gib {
		t.Errorf("memSize used RSS over footprint: got %d GB", got/gib)
	}
	// Non-darwin / dead pid: footprint unavailable, RSS is all there is.
	if got := memSize(ProcessInfo{RSS: 4 * gib, Footprint: 0}); got != 4*gib {
		t.Errorf("memSize did not fall back to RSS: got %d", got)
	}
}

// The finding alarms on max(live, peak). The verb has to say which one, or the
// sentence claims the present tense for a lifetime peak — "holds 41.5 GB" for a
// process currently holding 15.5 GB, which is what the owner caught in the
// standalone `sirsi ask` output before the V2 demo.
func TestFootprintVerbNamesWhichMeasurement(t *testing.T) {
	const gb = int64(1) << 30

	// Live IS the worst: present tense is correct.
	if got := footprintVerb(41*gb, 41*gb); got != "holds" {
		t.Fatalf("live==worst must read as present tense, got %q", got)
	}
	// Peak exceeds live: the alarming number is historical.
	if got := footprintVerb(15*gb, 41*gb); got != "peaked at" {
		t.Fatalf("a peak above live must not be reported in the present tense, got %q", got)
	}
	// A process that has since exited entirely still peaked.
	if got := footprintVerb(0, 41*gb); got != "peaked at" {
		t.Fatalf("zero live with a real peak must read as a peak, got %q", got)
	}
	// Degenerate: nothing measured. Must not claim a peak.
	if got := footprintVerb(0, 0); got != "holds" {
		t.Fatalf("no measurement must not invent a peak claim, got %q", got)
	}
}
