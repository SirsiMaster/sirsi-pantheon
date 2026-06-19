package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
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

	// 1) The default concurrency must be 0 = AUTO-DERIVE from the node (ADR-031-B).
	//    It shipped as 4 (OOM'd the host), was hardened to a fixed 1 (#60), and is now
	//    node-derived: 0 means "use NodeCapacity.MaxConcurrency", which is RAM/VRAM-
	//    gated and floored at 1 — so auto is SAFE (bounded by the box), never the old
	//    aggressive fixed number. The footgun cannot return: a positive default would
	//    fail this, and the derivation itself refuses-or-floors (asserts 4/5 below).
	if dv := gemmaServeCmd.Flags().Lookup("concurrency").DefValue; dv != "0" {
		t.Errorf("default --concurrency must be 0 (auto-derive from the node, ADR-031-B); got %q", dv)
	}

	// 2) The broker MUST launch through the hard-cap wrapper that bounds MLX memory
	//    BEFORE the model loads — the layer that makes "never OOM" true at runtime.
	if !strings.Contains(gemmaCapWrapper, "set_memory_limit") || !strings.Contains(gemmaCapWrapper, "set_cache_limit") {
		t.Error("the cap wrapper must set mx.set_memory_limit + set_cache_limit before launching the server")
	}

	// 3) The cold path's gate still refuses-rather-than-OOM (2×model serial budget):
	//    a 12 GB model with only 30 GB free must be REFUSED, not run serial at 1×.
	if safe, _ := gemmaSafeConcurrency(1, 12*gb, 30*gb); safe != 0 {
		t.Errorf("cold-path 2× serial budget: 12GB model + 30GB free must refuse (0), got %d", safe)
	}

	// 4) The warm broker's node-derived gate (ADR-031-B, claude-home binding guard):
	//    a node where one model won't fit within the dynamic reserve must NOT fit —
	//    the broker refuses rather than run the floored-1 that would OOM.
	tight := guard.NodeCapacity{TotalRAM: 48 * gb, FreeRAM: 18 * gb, OSBaseline: 48 * gb / 32, AgentRSS: 4 * gb}
	if tight.Fits(12 * gb) {
		t.Error("node-derived gate: 12GB model must NOT fit on 18GB-free/48GB node — broker must refuse")
	}

	// 5) And MaxConcurrency NEVER returns <1 (the floor the Fits gate guards), and on
	//    a node that can't hold the model it still says 1 — which is exactly why (4)'s
	//    Fits refuse gate must run first.
	if tight.MaxConcurrency(12*gb) < 1 {
		t.Error("MaxConcurrency must floor at 1")
	}
}
