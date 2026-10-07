package router

import (
	"path/filepath"
	"testing"
	"time"
)

// One probe serves every loop on the host for the TTL; an expired or missing cache
// probes again (both directions).
func TestHostLoadProbeIsSharedAcrossLoopsForTheTTL(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "host-load.cache")
	now := time.Unix(1_800_000_000, 0)
	probes := 0
	oldP, oldN, oldF := hostLoadCachePath, hostLoadNow, loadProbeFn
	t.Cleanup(func() { hostLoadCachePath, hostLoadNow, loadProbeFn = oldP, oldN, oldF })
	hostLoadCachePath = func() string { return cache }
	hostLoadNow = func() time.Time { return now }
	loadProbeFn = func() (float64, bool) { probes++; return 4.5, true }

	for i := 0; i < 9; i++ { // nine wake loops in one cycle
		if v, ok := defaultLoadAvg1m(); !ok || v != 4.5 {
			t.Fatalf("loop %d: got %v %v", i, v, ok)
		}
	}
	if probes != 1 {
		t.Fatalf("nine loops inside the TTL must share one probe, got %d", probes)
	}
	now = now.Add(hostLoadCacheTTL + time.Second)
	loadProbeFn = func() (float64, bool) { probes++; return 7.0, true }
	if v, _ := defaultLoadAvg1m(); v != 7.0 || probes != 2 {
		t.Fatalf("an expired cache must probe again: v=%v probes=%d", v, probes)
	}
	// An unreadable host is never cached as a reading.
	now = now.Add(hostLoadCacheTTL + time.Second)
	loadProbeFn = func() (float64, bool) { probes++; return 0, false }
	if _, ok := defaultLoadAvg1m(); ok {
		t.Fatal("a failed probe must not report ok")
	}
	if _, ok := readHostLoadCache(); ok {
		t.Fatal("a failed probe must not be cached")
	}
}
