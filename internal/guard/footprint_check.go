package guard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// Footprint thresholds as a FRACTION OF PHYSICAL RAM, not absolute bytes.
//
// Absolute thresholds are how a check becomes useless on the machine that needs
// it most. 20 GB is unremarkable on a 64 GB workstation and terminal on a 16 GB
// laptop — and 16 GB is the target machine (SIRSI_V2_APPLICATION §2). A single
// process holding a third of RAM is the same danger everywhere; that is what
// gets measured.
const (
	footprintWarnFraction     = 0.33 // one process holding a third of RAM
	footprintCriticalFraction = 0.50 // one process holding half of RAM
)

// checkProcessFootprint flags any single process whose PHYSICAL FOOTPRINT is a
// dangerous fraction of RAM.
//
// This is the check that was missing on 2026-07-27, when this machine OOM'd
// three times in 24 hours and `sirsi diagnose` reported 100/100 across 16
// signals throughout. The culprit was the gemma broker — Sirsi's own local
// model — at a 43.94 GB footprint on a 48 GB machine, of which 39.87 GB was
// compressed and only 0.68 GB resident. Every existing check sampled `ps -o
// rss` and saw an innocent process. A 65x error at the moment it mattered.
//
// Two properties this check must keep:
//
//  1. It reports PEAK as well as live. A process that spikes to 40 GB during an
//     inference and settles at 19 GB is a Jetsam risk even though it looks calm
//     when you happen to sample it. The kernel chooses its victim at the spike.
//
//  2. It NAMES SIRSI'S OWN PROCESSES. The largest consumer on the owner's
//     workstation is Sirsi. A health surface that exempts its own components is
//     the green-surface-over-a-dead-thing failure this codebase keeps recording,
//     and the local-model broker is the single most likely offender.
func checkProcessFootprint(p platform.Platform, report *DoctorReport) {
	finding := DiagnosticFinding{Check: "Process Footprint"}

	totalRAM := totalPhysicalRAM(p)
	if totalRAM == 0 {
		finding.Severity = SeverityOK
		finding.Message = "physical RAM size unavailable — footprint check skipped"
		report.Findings = append(report.Findings, finding)
		return
	}

	procs, err := getProcessListWith(p)
	if err != nil {
		finding.Severity = SeverityOK
		finding.Message = "process list unavailable — footprint check skipped"
		report.Findings = append(report.Findings, finding)
		return
	}

	// Rank by the larger of live and peak: a process that already peaked near
	// the ceiling is a standing risk regardless of where it sits right now.
	type entry struct {
		name       string
		pid        int
		live, peak int64
	}
	var ranked []entry
	for _, pr := range procs {
		if pr.Footprint == 0 && pr.PeakFootprint == 0 {
			continue // unavailable — do not invent a number
		}
		ranked = append(ranked, entry{pr.Name, pr.PID, pr.Footprint, pr.PeakFootprint})
	}
	if len(ranked) == 0 {
		finding.Severity = SeverityOK
		finding.Message = "physical footprint unavailable on this platform — falling back to RSS elsewhere"
		report.Findings = append(report.Findings, finding)
		return
	}
	sort.Slice(ranked, func(i, j int) bool {
		return max64(ranked[i].live, ranked[i].peak) > max64(ranked[j].live, ranked[j].peak)
	})

	top := ranked[0]
	worst := max64(top.live, top.peak)
	frac := float64(worst) / float64(totalRAM)

	switch {
	case frac >= footprintCriticalFraction:
		finding.Severity = SeverityCritical
		finding.Message = fmt.Sprintf(
			"%s (pid %d) holds %.1f GB of %.0f GB RAM (%.0f%%) — one process at half the machine; the kernel will Jetsam something to survive it",
			top.name, top.pid, fpGB(worst), fpGB(totalRAM), frac*100)
	case frac >= footprintWarnFraction:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf(
			"%s (pid %d) holds %.1f GB of %.0f GB RAM (%.0f%%) — a third of the machine in one process",
			top.name, top.pid, fpGB(worst), fpGB(totalRAM), frac*100)
	default:
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("largest process %s at %.1f GB of %.0f GB (%.0f%%) — no single-process pressure",
			top.name, fpGB(worst), fpGB(totalRAM), frac*100)
	}

	// The detail is where the RSS lie becomes visible. Showing live, peak AND
	// the divergence is the point: an operator who has only ever seen RSS needs
	// to be shown why the number they trusted was wrong.
	if top.peak > top.live && top.live > 0 {
		finding.Detail = fmt.Sprintf("live %.1f GB · peak %.1f GB (%.1fx) — peak is what the kernel judges",
			fpGB(top.live), fpGB(top.peak), float64(top.peak)/float64(top.live))
	} else {
		finding.Detail = fmt.Sprintf("live %.1f GB · peak %.1f GB", fpGB(top.live), fpGB(top.peak))
	}
	if len(ranked) > 1 {
		finding.Detail += fmt.Sprintf(" | next: %s %.1f GB", ranked[1].name, fpGB(max64(ranked[1].live, ranked[1].peak)))
	}

	report.Findings = append(report.Findings, finding)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func fpGB(b int64) float64 { return float64(b) / (1 << 30) }

// totalPhysicalRAM returns installed RAM in bytes, or 0 when unknown. Goes
// through the platform seam so the check is testable with a fake machine size —
// the 16 GB target must be exercisable from a 48 GB dev box.
func totalPhysicalRAM(p platform.Platform) int64 {
	out, err := p.Command("sysctl", "-n", "hw.memsize")
	if err != nil {
		return 0
	}
	var n int64
	if _, serr := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n); serr != nil {
		return 0
	}
	return n
}
