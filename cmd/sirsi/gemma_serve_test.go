package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

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
	stackIdx := strings.Index(gemmaCapWrapper, "threading.stack_size(512 * 1024 * 1024)")
	runIdx := strings.Index(gemmaCapWrapper, "runpy.run_module")
	if stackIdx < 0 {
		t.Error("the cap wrapper must raise Python worker thread stack before MLX server threads start")
	}
	if runIdx < 0 {
		t.Fatal("the cap wrapper must still launch mlx_lm.server through runpy")
	}
	if stackIdx > runIdx {
		t.Error("threading.stack_size must run before mlx_lm.server creates worker threads")
	}

	// 3) The cold path now refuses through the SAME NodeCapacity.Fits gate as the
	//    warm broker (no separate gemmaSafeConcurrency helper) — and the 2×model
	//    working-memory budget survived the re-point: a 12 GB model on a 48 GB node
	//    with only 30 GB free + ~4 GB of agents must still REFUSE (2×12 + ~13.5
	//    reserve = 37.5 > 30). Under the old 1×model gate this would have been
	//    admitted (12 + 13.5 = 25.5 ≤ 30) and run too tight to serve.
	coldTight := guard.NodeCapacity{TotalRAM: 48 * gb, FreeRAM: 30 * gb, OSBaseline: 48 * gb / 32, AgentRSS: 4 * gb}
	if coldTight.Fits(12 * gb) {
		t.Error("cold-path NodeCapacity gate: 12GB model on 30GB-free/48GB node must refuse (2×model budget)")
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
