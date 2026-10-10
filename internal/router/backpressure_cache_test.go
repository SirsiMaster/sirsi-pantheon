package router

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Each nine-loop cohort shares one real cache record. Expiry is represented on
// disk, without a global mutable clock or probe seam. Unknown is cached as
// unknown, never as a pressure reading; after TTL it must retry and recover.
func TestHostLoadProbeIsSharedAcrossLoopsForTheTTL(t *testing.T) {
	for _, tc := range []struct {
		name            string
		first, next     float64
		firstOK, nextOK bool
	}{
		{"low to high", 1, 9.5, true, true},
		{"high to low", 9.5, 1, true, true},
		{"known to unknown", 9.5, 0, true, false},
		{"unknown to known", 0, 1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "host-pressure-v2.cache")
			calls := 0
			value, known := tc.first, tc.firstOK
			probe := func() (float64, bool) { calls++; return value, known }
			cohort := func(want float64, ok bool, count int) {
				t.Helper()
				for i := 0; i < 9; i++ {
					v, k := coordinatedHostLoad(p, probe, time.Second)
					if v != want || k != ok {
						t.Fatalf("loop%d: got %v %v want %v %v", i, v, k, want, ok)
					}
				}
				if calls != count {
					t.Fatalf("probes=%d want%d", calls, count)
				}
			}
			cohort(tc.first, tc.firstOK, 1)
			v, k, fresh := readPressureRecord(p)
			if !fresh || v != tc.first || k != tc.firstOK {
				t.Fatalf("cache: %v %v %v", v, k, fresh)
			}
			expired := time.Now().Add(-hostLoadCacheTTL - time.Second).UnixMilli()
			if err := os.WriteFile(p, []byte(fmt.Sprintf("v2 %d %t %g\n", expired, known, value)), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, fresh := readPressureRecord(p); fresh {
				t.Fatal("expired record reused")
			}
			value, known = tc.next, tc.nextOK
			cohort(tc.next, tc.nextOK, 2)
		})
	}
}

func TestPressureCacheRejectsInvalidRecords(t *testing.T) {
	for _, tc := range []struct{ name, record string }{
		{"legacy", fmt.Sprintf("%d 4.5", time.Now().UnixMilli())},
		{"future", fmt.Sprintf("v2 %d true 4.5", time.Now().Add(time.Minute).UnixMilli())},
		{"stale unknown", fmt.Sprintf("v2 %d false 0", time.Now().Add(-time.Minute).UnixMilli())},
		{"corrupt", "v2 nonsense"},
		{"nan", fmt.Sprintf("v2 %d true NaN", time.Now().UnixMilli())},
		{"inf", fmt.Sprintf("v2 %d true +Inf", time.Now().UnixMilli())},
		{"negative", fmt.Sprintf("v2 %d true -1", time.Now().UnixMilli())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "cache")
			if err := os.WriteFile(p, []byte(tc.record), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, fresh := readPressureRecord(p); fresh {
				t.Fatal("invalid record accepted")
			}
			calls := 0
			v, ok := coordinatedHostLoad(p, func() (float64, bool) { calls++; return 2, true }, time.Second)
			if !ok || v != 2 || calls != 1 {
				t.Fatal(v, ok, calls)
			}
		})
	}
}

func TestPressureCacheNormalizesInvalidProbe(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), -1} {
		p := filepath.Join(t.TempDir(), "cache")
		calls := 0
		for i := 0; i < 2; i++ {
			got, ok := coordinatedHostLoad(p, func() (float64, bool) { calls++; return v, true }, time.Second)
			if ok || got != 0 {
				t.Fatal(got, ok)
			}
		}
		if calls != 1 {
			t.Fatal("unknown not amortized", calls)
		}
	}
}
