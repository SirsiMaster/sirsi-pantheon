package main

import (
	"strings"
	"testing"
)

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

// TestGemmaNeverAgainInvariants encodes ADR-031-A so no future edit can silently
// re-introduce the 2026-06-18 footgun (concurrency-4 default + no hard cap).
func TestGemmaNeverAgainInvariants(t *testing.T) {
	const gb = int64(1) << 30

	// 1) The default concurrency must be 1 (it shipped as 4 and OOM'd the host).
	if dv := gemmaServeCmd.Flags().Lookup("concurrency").DefValue; dv != "1" {
		t.Errorf("default --concurrency must be 1 (was the footgun at 4); got %q", dv)
	}

	// 2) The broker MUST launch through the hard-cap wrapper that bounds MLX memory
	//    BEFORE the model loads — the layer that makes "never OOM" true at runtime.
	if !strings.Contains(gemmaCapWrapper, "set_memory_limit") || !strings.Contains(gemmaCapWrapper, "set_cache_limit") {
		t.Error("the cap wrapper must set mx.set_memory_limit + set_cache_limit before launching the server")
	}

	// 3) Serial budget is 2×model: a 12 GB model with only 30 GB free must be REFUSED
	//    (2×12 + 8 headroom = 32 > 30), not allowed to run serial at 1×.
	if safe, _ := gemmaSafeConcurrency(1, 12*gb, 30*gb); safe != 0 {
		t.Errorf("2× serial budget: 12GB model + 30GB free must refuse (0), got %d", safe)
	}
}
