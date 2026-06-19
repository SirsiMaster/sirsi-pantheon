package main

import "testing"

// TestGemmaSafeConcurrency locks the RAM gate that prevents the broker from
// OOM-ing the machine (the 2026-06-18 incident: concurrency 4 ballooned a 12 GB
// model to ~64 GB on a 48 GB box → Jetsam). Each concurrent slot is budgeted at
// ~a full model; serial (1) is allowed whenever the model + headroom fits.
func TestGemmaSafeConcurrency(t *testing.T) {
	const gb = int64(1) << 30
	cases := []struct {
		name        string
		requested   int
		model, free int64
		wantSafe    int
		wantRefuse  bool
	}{
		{"12GB model, 40GB free, want 4 → capped to 1", 4, 12 * gb, 40 * gb, 1, false},
		{"12GB model, 18GB free → REFUSE (model+headroom won't fit)", 4, 12 * gb, 18 * gb, 0, true},
		{"12GB model, requested 1 → 1", 1, 12 * gb, 40 * gb, 1, false},
		{"4GB model, 48GB free, want 4 → 4 fits", 4, 4 * gb, 48 * gb, 4, false},
		{"4GB model, 48GB free, want 2 → 2", 2, 4 * gb, 48 * gb, 2, false},
		{"unknown model size (0) defaults conservative, 48GB free → 1", 4, 0, 48 * gb, 1, false},
	}
	for _, c := range cases {
		safe, note := gemmaSafeConcurrency(c.requested, c.model, c.free)
		if c.wantRefuse {
			if safe != 0 || note == "" {
				t.Errorf("%s: want refuse (0 + reason), got safe=%d note=%q", c.name, safe, note)
			}
			continue
		}
		if safe != c.wantSafe {
			t.Errorf("%s: want safe=%d, got %d (note=%q)", c.name, c.wantSafe, safe, note)
		}
		if safe < 1 {
			t.Errorf("%s: must never return <1 when the model fits", c.name)
		}
	}
}
