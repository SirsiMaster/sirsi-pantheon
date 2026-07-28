// Package govern is the resource governor: the single place that decides
// whether this machine can afford a piece of work, and the single place that
// enforces a limit once it has been granted.
//
// It exists because of a measured architectural property, not a hunch. On
// 2026-07-27 this repo had 196 call sites able to spawn an OS process and zero
// enforcing a memory or process budget. The consequences recur — 19,195 leaked
// sessions (2026-07-03/04), a 358-process fork storm (2026-07-27), and three
// OOM kills in a single day caused by one process. Ka and Anubis are janitors
// hired because the building has no doors. This package is the doors.
//
// Ceiling enforcement is the first piece, because the immediate wound is a
// declared cap that does not cap.
package govern

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/vitals"
)

// Verdict is what the governor decided about a process against its ceiling.
type Verdict int

const (
	VerdictUnknown     Verdict = iota // footprint unreadable — say so, never assume OK
	VerdictOK                         // comfortably under
	VerdictApproaching                // over the soft mark; warn, do not act
	VerdictOverCeiling                // over the hard ceiling; act
)

func (v Verdict) String() string {
	switch v {
	case VerdictOK:
		return "ok"
	case VerdictApproaching:
		return "approaching"
	case VerdictOverCeiling:
		return "over-ceiling"
	default:
		return "unknown"
	}
}

// Assessment is a ceiling decision with the evidence that produced it, so a
// surface can show WHY rather than just a color.
type Assessment struct {
	PID      int
	Verdict  Verdict
	Live     uint64 // current physical footprint, bytes
	Peak     uint64 // lifetime max physical footprint, bytes
	Ceiling  uint64 // the hard limit applied
	SoftMark uint64 // the warn threshold
	Reason   string
	Err      error
}

// Ceiling describes a limit on one process's PHYSICAL FOOTPRINT.
//
// Footprint, not RSS, and this distinction is the whole point. The gemma broker
// is launched with `gemma-capped-server.py 22320611328` — a 20.8 GiB "cap" — and
// reached a 43.94 GB footprint anyway, because that argument calls
// mx.set_memory_limit(), which bounds MLX's own allocator and nothing else. A
// declared cap that the process can exceed by 2x is worse than no cap: it buys
// false confidence. Enforcement has to live OUTSIDE the process, watching the
// number the kernel itself judges by.
type Ceiling struct {
	// HardBytes is the footprint at or above which the governor acts.
	HardBytes uint64
	// SoftFraction of HardBytes at which to warn. 0 means 0.8.
	SoftFraction float64
	// UsePeak makes the verdict consider lifetime peak as well as live
	// footprint. Required for inference workloads: the broker sat at 19 GB and
	// peaked at 40.5 GB, and the kernel chose its victim at the peak. A ceiling
	// that only reads "now" will pass a process that is regularly the largest
	// thing on the machine.
	UsePeak bool
}

// CeilingForRAM derives a per-process ceiling from installed RAM.
//
// A fraction, never an absolute: 20 GB is unremarkable on a 64 GB workstation
// and fatal on the 16 GB machine Sirsi targets. One process is allowed a third
// of the machine — beyond that the kernel starts choosing victims, and it does
// not choose convenient ones.
func CeilingForRAM(totalRAM uint64) Ceiling {
	return Ceiling{
		HardBytes:    totalRAM / 3,
		SoftFraction: 0.8,
		UsePeak:      true,
	}
}

// Assess reports where a process sits against its ceiling.
//
// Fails to VerdictUnknown with the error attached rather than to OK. An
// unreadable footprint is not a healthy one, and reporting green on missing
// evidence is the specific failure this codebase keeps recording.
func Assess(pid int, c Ceiling) Assessment {
	a := Assessment{PID: pid, Ceiling: c.HardBytes}
	if c.SoftFraction <= 0 {
		c.SoftFraction = 0.8
	}
	a.SoftMark = uint64(float64(c.HardBytes) * c.SoftFraction)

	live, err := vitals.PhysFootprint(pid)
	if err != nil {
		a.Verdict = VerdictUnknown
		a.Err = err
		a.Reason = fmt.Sprintf("footprint unreadable for pid %d: %v", pid, err)
		return a
	}
	a.Live = live
	if peak, perr := vitals.PeakPhysFootprint(pid); perr == nil {
		a.Peak = peak
	}

	judged := live
	judgedLabel := "live"
	if c.UsePeak && a.Peak > judged {
		judged = a.Peak
		judgedLabel = "peak"
	}

	switch {
	case judged >= c.HardBytes:
		a.Verdict = VerdictOverCeiling
		a.Reason = fmt.Sprintf("%s footprint %.1f GB is over the %.1f GB ceiling",
			judgedLabel, toGB(judged), toGB(c.HardBytes))
	case judged >= a.SoftMark:
		a.Verdict = VerdictApproaching
		a.Reason = fmt.Sprintf("%s footprint %.1f GB is approaching the %.1f GB ceiling",
			judgedLabel, toGB(judged), toGB(c.HardBytes))
	default:
		a.Verdict = VerdictOK
		a.Reason = fmt.Sprintf("%s footprint %.1f GB, ceiling %.1f GB",
			judgedLabel, toGB(judged), toGB(c.HardBytes))
	}
	return a
}

func toGB(b uint64) float64 { return float64(b) / (1 << 30) }
