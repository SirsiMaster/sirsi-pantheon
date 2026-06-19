package guard

import "testing"

// node builds a NodeCapacity with the given total/free/vram/agentRSS for testing
// the derivations (OSBaseline mirrors SampleNodeCapacity: total/32).
func node(totalGB, freeGB, vramGB, agentGB int64) NodeCapacity {
	return NodeCapacity{
		TotalRAM:   totalGB * gb,
		FreeRAM:    freeGB * gb,
		VRAMBytes:  vramGB * gb,
		AgentRSS:   agentGB * gb,
		OSBaseline: (totalGB * gb) / nodeOSBaselineDivisor,
	}
}

// TestMaxConcurrencyScalesWithNode is the heart of ADR-031-B: concurrency is a
// FUNCTION of the measured node, not the hardcoded 1. Same model, different boxes,
// different answers — and never below 1, never OOM-ing a small one.
func TestMaxConcurrencyScalesWithNode(t *testing.T) {
	model := int64(12) * gb
	cases := []struct {
		name                     string
		total, free, vram, agent int64
		wantAtLeast, wantAtMost  int
	}{
		// 48 GB box, 12 GB model, agents holding ~6 GB → tight → ~1.
		{"48GB M5 Max (the OOM box)", 48, 24, 0, 6, 1, 2},
		// 256 GB box → the warm broker's real value: many concurrent slots.
		{"256GB server", 256, 230, 0, 8, 12, 20},
		// Tiny box that can't even hold model+reserve → floored to 1, never 0.
		{"16GB laptop, model barely fits", 16, 14, 0, 1, 1, 1},
		// Dedicated-VRAM GPU caps by VRAM even when RAM is huge.
		{"big RAM but 24GB VRAM caps it", 256, 230, 24, 8, 1, 2},
	}
	for _, c := range cases {
		nc := node(c.total, c.free, c.vram, c.agent)
		got := nc.MaxConcurrency(model)
		if got < c.wantAtLeast || got > c.wantAtMost {
			t.Errorf("%s: MaxConcurrency=%d, want [%d,%d]", c.name, got, c.wantAtLeast, c.wantAtMost)
		}
		if got < 1 {
			t.Errorf("%s: must never return <1", c.name)
		}
		// Maximality invariant: C concurrent slots cost (1+C)×model + reserve (1
		// resident + C working — the #63/06-18 fact). On a node that genuinely fits,
		// `got` must be the LARGEST C that still fits, and (got+1) must NOT fit. This
		// is what the broker re-point first dropped (C×model, no resident) — the old
		// code returned one slot too many here on the 256 GB box and would over-commit.
		if nc.Fits(model) && nc.VRAMBytes == 0 {
			if used := int64(1+got)*model + nc.DynamicReserve(); used > nc.FreeRAM {
				t.Errorf("%s: C=%d over-commits: (1+%d)×model+reserve=%dGB > %dGB free", c.name, got, got, used/gb, nc.FreeRAM/gb)
			}
			if int64(2+got)*model+nc.DynamicReserve() <= nc.FreeRAM {
				t.Errorf("%s: C=%d not maximal — one more slot still fits", c.name, got)
			}
		}
	}
	// Unknown model size → conservative 1.
	if got := node(256, 230, 0, 8).MaxConcurrency(0); got != 1 {
		t.Errorf("unknown model size must yield 1, got %d", got)
	}
}

// TestDynamicReserveIsProportional proves the reserve scales with the node (not a
// flat 8 GB) while honoring the 8 GB floor.
func TestDynamicReserveIsProportional(t *testing.T) {
	// 48 GB: total/8 = 6 GB < 8 GB floor → margin 8 GB; + agentRSS + OSBaseline.
	small := node(48, 24, 0, 4)
	wantSmall := small.OSBaseline + 4*gb + nodeMarginFloor // floor applies
	if got := small.DynamicReserve(); got != wantSmall {
		t.Errorf("48GB reserve=%d, want %d (8GB floor)", got, wantSmall)
	}
	// 256 GB: total/8 = 32 GB > floor → proportional margin 32 GB.
	big := node(256, 230, 0, 8)
	wantBig := big.OSBaseline + 8*gb + 256*gb/nodeMarginDivisor
	if got := big.DynamicReserve(); got != wantBig {
		t.Errorf("256GB reserve=%d, want %d (proportional)", got, wantBig)
	}
	// The proportional margin must actually be larger on the bigger node.
	if big.DynamicReserve() <= small.DynamicReserve() {
		t.Error("reserve must grow with node size, not stay constant")
	}
}

// TestDynamicCapFloorsAtTwoModels: the cap never drops below 2×model (resident +
// one model of working memory, matching the Fits gate), and is free−reserve
// otherwise. The floor only clamps the refuse case Fits already rejected.
func TestDynamicCapFloorsAtTwoModels(t *testing.T) {
	model := int64(12) * gb
	tight := node(48, 20, 0, 10) // free−reserve ≈ 0.5GB → floor at 2×model
	if got := tight.DynamicCap(model); got != 2*model {
		t.Errorf("tight node cap=%d, want floor %d (2×model)", got, 2*model)
	}
	roomy := node(256, 230, 0, 8)
	if got := roomy.DynamicCap(model); got != roomy.FreeRAM-roomy.DynamicReserve() {
		t.Errorf("roomy node cap=%d, want free−reserve %d", got, roomy.FreeRAM-roomy.DynamicReserve())
	}
}

// TestFitsRefuseGate is claude-home's binding condition: refuse when one model
// won't fit within the dynamic reserve, even though MaxConcurrency floors at 1.
func TestFitsRefuseGate(t *testing.T) {
	model := int64(12) * gb
	// 48GB box, only 20GB free, agents holding 10GB: reserve ≈ 1.5+10+8 = 19.5GB;
	// 2×model + reserve = 43.5GB > 20GB free → must NOT fit (refuse).
	tight := node(48, 20, 0, 10)
	if tight.Fits(model) {
		t.Errorf("tight node must refuse: 2×model %dGB + reserve %dGB > %dGB free",
			2*model/gb, tight.DynamicReserve()/gb, tight.FreeRAM/gb)
	}
	// Same node with room: 48GB, 40GB free, 2GB agents → reserve ≈ 1.5+2+8 = 11.5;
	// 2×12 + 11.5 = 35.5 <= 40 → fits.
	roomy := node(48, 40, 0, 2)
	if !roomy.Fits(model) {
		t.Errorf("roomy node should fit: model %dGB + reserve %dGB <= %dGB free",
			model/gb, roomy.DynamicReserve()/gb, roomy.FreeRAM/gb)
	}
	// The guard's invariant: if Fits is false, the caller must refuse — but if a
	// caller ignored it, MaxConcurrency would still floor at 1 (the trap Fits guards).
	if tight.MaxConcurrency(model) != 1 {
		t.Error("MaxConcurrency floors at 1 even when it doesn't fit — that is exactly why Fits exists")
	}
}

func TestBootstrapPressureAndString(t *testing.T) {
	if bootstrapPressure(40) != PressureNormal || bootstrapPressure(6) != PressureWarn || bootstrapPressure(2) != PressureCritical {
		t.Error("bootstrap pressure tiers wrong")
	}
	if PressureNormal.String() != "normal" || PressureCritical.String() != "critical" || PressureUnknown.String() != "unknown" {
		t.Error("PressureLevel.String() wrong")
	}
}
