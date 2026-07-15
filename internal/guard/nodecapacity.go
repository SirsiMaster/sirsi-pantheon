// Package guard — nodecapacity.go
//
// NodeCapacity — the live self-model of THIS machine (ADR-031-B). The never-exhaust
// stack (ADR-031-A) is correct in its invariant but hardcoded 48 GB-tuned constants
// (concurrency=1, 8 GB headroom, free<8%/<4% thresholds, 2×model). That contradicts
// Pantheon's thesis: dynamically understand each node and balance within/across it.
//
// NodeCapacity is the foundation that replaces those literals: everything that
// enforces the invariant (broker concurrency, the memory cap, the governor's
// reserve) reads its NUMBERS from here, derived from the MEASURED node — never from
// constants. On 48 GB/12 GB-model it yields ~1 concurrency; on 256 GB, 13+. The
// invariant stays constant; the numbers became functions.
//
// It is serializable (JSON) so a future fleet layer (Ra/SirsiNexus) can balance
// load ACROSS nodes — each node's NodeCapacity + load is the unit it balances. The
// per-node governor is the prerequisite; this is that model.
//
// Pressure: the AUTHORITATIVE level comes from the kernel's
// DISPATCH_SOURCE_TYPE_MEMORYPRESSURE (ADR-031-B #4, codex SME) — node-proportional
// for free. That watcher now exists (pressure.go): the Hapi daemon runs it and
// records the kernel level; SampleNodeCapacity reads it via effectivePressure
// (this process → the daemon's cross-process cache → bootstrap). The free-% seed
// (bootstrapPressure) is the fallback the ADR sanctions when no watcher has
// reported — clearly marked PressureSource="bootstrap-snapshot", NOT the contract.
package guard

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

// PressureLevel mirrors the kernel's memory-pressure levels (node-relative).
type PressureLevel int

const (
	PressureUnknown PressureLevel = iota
	PressureNormal
	PressureWarn
	PressureCritical
)

func (p PressureLevel) String() string {
	switch p {
	case PressureNormal:
		return "normal"
	case PressureWarn:
		return "warn"
	case PressureCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// NodeCapacity is the live, serializable self-model of this machine.
type NodeCapacity struct {
	TotalRAM     int64         `json:"total_ram"`
	FreeRAM      int64         `json:"free_ram"`
	VRAMBytes    int64         `json:"vram_bytes"` // 0 on unified-memory Apple Silicon (shared with RAM)
	GPUType      string        `json:"gpu_type"`
	CPUCores     int           `json:"cpu_cores"`
	NeuralEngine bool          `json:"neural_engine"`
	OSBaseline   int64         `json:"os_baseline_rss"` // estimated idle OS footprint (refined later)
	AgentRSS     int64         `json:"agent_rss"`       // live claude/codex resident memory
	Pressure     PressureLevel `json:"pressure"`
	// PressureSource is "kernel-dispatch" (authoritative), "bootstrap-snapshot"
	// (a free-% seed, fallback only), or "unknown" (non-Darwin / no source).
	PressureSource string         `json:"pressure_source"`
	GovernedPIDs   map[int]string `json:"governed_pids"`
}

// nodeMarginDivisor sets the proportional reserve margin (TotalRAM / divisor),
// floored at nodeMarginFloor. NOT a hardcoded headroom: it scales with the node.
const (
	nodeMarginDivisor     = 8              // 12.5% of total RAM…
	nodeMarginFloor       = int64(8) << 30 // …but never below 8 GB
	nodeOSBaselineDivisor = 32             // rough idle-OS footprint ≈ TotalRAM/32 (refined later)
)

// SampleNodeCapacity builds the live model from the measured node: hardware
// (seba), free RAM + governed set (Hapi), and live agent RSS. Pressure is the
// bootstrap snapshot until the kernel dispatch watcher is wired (ADR-031-B #4).
func SampleNodeCapacity() NodeCapacity {
	n := NodeCapacity{GovernedPIDs: getGovernedFn()()}

	if hw, err := seba.DetectHardware(); err == nil {
		n.TotalRAM = hw.TotalRAM
		n.VRAMBytes = hw.GPU.VRAM
		n.GPUType = string(hw.GPU.Type)
		n.CPUCores = hw.CPUCores
		n.NeuralEngine = hw.NeuralEngine
	}
	if s, err := hapiSample(); err == nil {
		n.FreeRAM = s.FreeBytes
	} else {
		n.FreeRAM = hapiFreeRAMBytes()
	}
	if n.TotalRAM > 0 {
		n.OSBaseline = n.TotalRAM / nodeOSBaselineDivisor
	}
	// Authoritative kernel level when a watcher has reported (this process or the
	// Hapi daemon via the cache); the bootstrap free-% snapshot otherwise.
	n.Pressure, n.PressureSource = effectivePressure(n.FreeRAM, n.TotalRAM)
	n.AgentRSS = nodeAgentRSS()
	return n
}

// DynamicReserve is the RAM to keep free for the OS + live agents + a margin that
// SCALES with the node — replacing ADR-031-A's flat 8 GB headroom (mapping #2).
func (n NodeCapacity) DynamicReserve() int64 {
	margin := n.TotalRAM / nodeMarginDivisor
	if margin < nodeMarginFloor {
		margin = nodeMarginFloor
	}
	return n.OSBaseline + n.AgentRSS + margin
}

// MaxConcurrency is the largest number of concurrent decode slots this node can
// hold for a model of perModelBytes — derived, not the hardcoded 1 (mapping #1).
//
// Memory model (the #63 / 2026-06-18 empirical fact, preserved here so the
// re-point does NOT loosen it): a resident model costs 1×perModelBytes and EACH
// concurrent decode slot grows ~one more model of working memory. So C concurrent
// slots cost (1+C)×perModelBytes, NOT C× — concurrency 4 ballooned a 12 GB model
// to ~64 GB on a 48 GB box → Jetsam. The largest safe C therefore satisfies
//
//	(1+C)×perModelBytes + DynamicReserve <= FreeRAM,   floored at 1, VRAM-capped.
//
// On 48 GB/12 GB that's ~1; on 256 GB it's 13+ — the warm broker's real value
// (multi-concurrent on the GPU) appears automatically where the node can hold it.
func (n NodeCapacity) MaxConcurrency(perModelBytes int64) int {
	if perModelBytes <= 0 {
		return 1
	}
	// Subtract the resident model FIRST; the remainder holds working memory, one
	// model's worth per concurrent slot (the resident model is not a free slot).
	avail := n.FreeRAM - n.DynamicReserve() - perModelBytes
	slots := int(avail / perModelBytes)
	if n.VRAMBytes > 0 { // dedicated-VRAM GPUs cap by VRAM too (resident + working both land in VRAM); unified memory (0) skips
		if vramSlots := int(n.VRAMBytes/perModelBytes) - 1; vramSlots < slots {
			slots = vramSlots
		}
	}
	if slots < 1 {
		slots = 1
	}
	return slots
}

// Fits reports whether at least ONE model of perModelBytes can run within this
// node's dynamic reserve WITH room to actually serve. This is the binding refuse
// gate (claude-home, ADR-031-B): MaxConcurrency floors at 1 and so never refuses
// on its own — callers MUST check Fits first and refuse-rather-than-OOM.
//
// Budget = 2×perModelBytes + DynamicReserve: one resident model + one model of
// working memory (a serial decode) + the OS/agent/margin reserve. This restores
// ADR-031-A's #63 conservatism — the cold path's 2×model serial budget — that the
// first re-point dropped to 1×. Host-safety is unchanged either way (the hard MLX
// cap is the backstop), but a 1× gate admits a launch too tight to serve: it hits
// the cap and evicts immediately. Fits is the pre-launch gate, now node-proportional.
func (n NodeCapacity) Fits(perModelBytes int64) bool {
	if perModelBytes <= 0 {
		perModelBytes = 14 << 30 // unknown → conservative default
	}
	return 2*perModelBytes+n.DynamicReserve() <= n.FreeRAM
}

// DynamicCap is the per-node memory ceiling fed to the MLX cap wrapper:
// FreeRAM − DynamicReserve (mapping #3), floored at 2×model so an admitted load
// has room for resident weights + one model of working memory (matching the Fits
// gate). MLX evicts/errors at this ceiling instead of growing into Jetsam; Fits
// already guaranteed free ≥ 2×model + reserve, so the floor only clamps the
// never-admitted refuse case.
func (n NodeCapacity) DynamicCap(perModelBytes int64) int64 {
	if perModelBytes <= 0 {
		perModelBytes = 14 << 30 // unknown → conservative default
	}
	cap := n.FreeRAM - n.DynamicReserve()
	if floor := 2 * perModelBytes; cap < floor {
		cap = floor
	}
	return cap
}

// bootstrapPressure seeds a level from free-% — the FALLBACK only (ADR-031-B #4
// makes the kernel DISPATCH_SOURCE_MEMORYPRESSURE level authoritative). These
// percents are a seed, never the governance contract; they are replaced by the
// kernel level the moment the dispatch watcher reports.
func bootstrapPressure(freePct float64) PressureLevel {
	switch {
	case freePct < 4:
		return PressureCritical
	case freePct < 8:
		return PressureWarn
	default:
		return PressureNormal
	}
}

// nodeAgentRSS sums the resident memory of live coding agents (claude/codex) so
// "free" can be computed net of what the agents the host exists for are holding
// (ADR-031-A cross-agent protection, now a NodeCapacity field).
func nodeAgentRSS() int64 {
	out, err := exec.Command("ps", "-axo", "rss,comm").Output()
	if err != nil {
		return 0
	}
	var total int64
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		name := strings.ToLower(strings.Join(f[1:], " "))
		if strings.Contains(name, "claude") || strings.Contains(name, "codex") {
			if rss, e := strconv.ParseInt(f[0], 10, 64); e == nil {
				total += rss * 1024
			}
		}
	}
	return total
}
