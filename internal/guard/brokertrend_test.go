package guard

import (
	"testing"
	"time"
)

// TestEvaluateBrokerTrend_NoHistory proves the fail-safe: with nothing
// observed yet, stability is never assumed.
func TestEvaluateBrokerTrend_NoHistory(t *testing.T) {
	now := time.Now()
	stable, detail := evaluateBrokerTrend(nil, now, 12_300_000_000)
	if stable {
		t.Fatalf("stable = true with no history, want false (detail: %q)", detail)
	}
}

// TestEvaluateBrokerTrend_InsufficientWindow proves a broker only observed
// for a couple of minutes isn't yet called stable, even if flat so far.
func TestEvaluateBrokerTrend_InsufficientWindow(t *testing.T) {
	now := time.Now()
	history := []footprintSample{
		{Unix: now.Add(-2 * time.Minute).Unix(), Bytes: 12_300_000_000},
	}
	stable, detail := evaluateBrokerTrend(history, now, 12_300_000_000)
	if stable {
		t.Fatalf("stable = true with only 2m of history, want false (detail: %q)", detail)
	}
}

// TestEvaluateBrokerTrend_FlatIsStable is the "12.3 GB is normal" case: the
// same footprint sustained over the minimum window reads as stable.
func TestEvaluateBrokerTrend_FlatIsStable(t *testing.T) {
	now := time.Now()
	history := []footprintSample{
		{Unix: now.Add(-45 * time.Minute).Unix(), Bytes: 12_100_000_000},
		{Unix: now.Add(-30 * time.Minute).Unix(), Bytes: 12_300_000_000},
		{Unix: now.Add(-15 * time.Minute).Unix(), Bytes: 12_200_000_000},
	}
	stable, detail := evaluateBrokerTrend(history, now, 12_300_000_000)
	if !stable {
		t.Fatalf("stable = false for a flat footprint over 45m, want true (detail: %q)", detail)
	}
}

// TestEvaluateBrokerTrend_RunawayGrowthAlarms is the regression fixture for
// the actual 2026-07-27 incident shape: a footprint that grows sharply must
// still alarm, regardless of how "normal" its starting point looked, and
// regardless of whether it's still under some claimed cap. This is the check
// that a self-reported-cap exemption could never pass.
func TestEvaluateBrokerTrend_RunawayGrowthAlarms(t *testing.T) {
	now := time.Now()
	history := []footprintSample{
		{Unix: now.Add(-30 * time.Minute).Unix(), Bytes: 12_300_000_000}, // the "normal" baseline
	}
	// Grew to 31.4 GB — one of the actually-measured incident footprints.
	stable, detail := evaluateBrokerTrend(history, now, 31_400_000_000)
	if stable {
		t.Fatalf("stable = true for 12.3GB -> 31.4GB growth, want false (detail: %q)", detail)
	}
}

// TestEvaluateBrokerTrend_SlowCreepAlarms proves the ratio+absolute dual
// threshold catches growth even when neither check alone would look dramatic
// at every single sample — a broker that creeps from 12.3 to 17 GB (+38%,
// +4.7GB) over the window should still alarm.
func TestEvaluateBrokerTrend_SlowCreepAlarms(t *testing.T) {
	now := time.Now()
	history := []footprintSample{
		{Unix: now.Add(-20 * time.Minute).Unix(), Bytes: 12_300_000_000},
	}
	stable, detail := evaluateBrokerTrend(history, now, 17_000_000_000)
	if stable {
		t.Fatalf("stable = true for +38%%/+4.7GB creep, want false (detail: %q)", detail)
	}
}

// TestEvaluateBrokerTrend_StaleSamplesIgnored proves samples older than
// brokerTrendMaxAge don't count toward the window — a sample from 3 hours
// ago shouldn't let a currently-growing broker look "flat since forever".
func TestEvaluateBrokerTrend_StaleSamplesIgnored(t *testing.T) {
	now := time.Now()
	history := []footprintSample{
		{Unix: now.Add(-3 * time.Hour).Unix(), Bytes: 4_000_000_000}, // stale, out of window
	}
	stable, detail := evaluateBrokerTrend(history, now, 12_300_000_000)
	if stable {
		t.Fatalf("stable = true using only a stale out-of-window sample, want false (detail: %q)", detail)
	}
}

// TestPruneAndAppendFootprint_DropsOldKeepsRecent covers the pure history
// maintenance function directly.
func TestPruneAndAppendFootprint_DropsOldKeepsRecent(t *testing.T) {
	now := time.Now()
	history := []footprintSample{
		{Unix: now.Add(-3 * time.Hour).Unix(), Bytes: 1},    // should be dropped
		{Unix: now.Add(-30 * time.Minute).Unix(), Bytes: 2}, // should survive
	}
	got := pruneAndAppendFootprint(history, now, 3)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (one pruned, one kept, one appended)", len(got))
	}
	if got[0].Bytes != 2 {
		t.Errorf("got[0].Bytes = %d, want 2 (the surviving old sample)", got[0].Bytes)
	}
	if got[1].Bytes != 3 || got[1].Unix != now.Unix() {
		t.Errorf("got[1] = %+v, want the freshly appended current sample", got[1])
	}
}

// TestBrokerTrendStable_RoundTrip is an integration-style test through the
// exported entrypoint, using a temp HOME so it never touches the real
// ~/.sirsi history file.
func TestBrokerTrendStable_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// First call: no history yet -> not stable.
	stable, _ := BrokerTrendStable(12_300_000_000)
	if stable {
		t.Fatal("first-ever call reported stable, want false (no history)")
	}

	// Manually backdate the just-written sample so the next call sees enough
	// elapsed time, instead of sleeping in a test.
	path, err := brokerFootprintHistoryPath()
	if err != nil {
		t.Fatal(err)
	}
	backdated := []footprintSample{{Unix: time.Now().Add(-20 * time.Minute).Unix(), Bytes: 12_300_000_000}}
	if err := saveBrokerFootprintHistory(backdated); err != nil {
		t.Fatal(err)
	}
	_ = path

	stable, detail := BrokerTrendStable(12_300_000_000)
	if !stable {
		t.Fatalf("second call with 20m of flat backdated history reported not stable, want true (detail: %q)", detail)
	}
}
