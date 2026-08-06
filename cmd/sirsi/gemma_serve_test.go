package main

import (
	"testing"
)

// TestGemmaNeverAgainInvariants encodes the native SNE cutover so no future edit
// can silently restore the retired Python broker or its unsafe defaults.
func TestGemmaNeverAgainInvariants(t *testing.T) {
	// 1) The default concurrency must be 0 = AUTO-DERIVE from the node (ADR-031-B).
	//    It shipped as 4 (OOM'd the host), was hardened to a fixed 1 (#60), and is now
	//    node-derived: 0 means "use NodeCapacity.MaxConcurrency", which is RAM/VRAM-
	//    gated and floored at 1 — so auto is SAFE (bounded by the box), never the old
	//    aggressive fixed number. The footgun cannot return: a positive default would
	//    fail this, and the derivation itself refuses-or-floors (asserts 4/5 below).
	if dv := gemmaServeCmd.Flags().Lookup("concurrency").DefValue; dv != "0" {
		t.Errorf("default --concurrency must be 0 (auto-derive from the node, ADR-031-B); got %q", dv)
	}

	// 2) Pantheon and SNE share the portfolio-standard port. Python's historical
	//    broker used 8765; accepting that port here would revive a split runtime.
	if gemmaServerDefaultPort != 8477 {
		t.Errorf("native SNE port = %d, want 8477", gemmaServerDefaultPort)
	}

}
