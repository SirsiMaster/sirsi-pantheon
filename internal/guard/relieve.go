// Package guard — relieve.go
//
// On-demand hang relief: find the live process that is hogging the CPU and lower
// its priority so the user's foreground app regains responsiveness. This is the
// human-/menubar-triggered counterpart to the watchdog's automatic AutoRenice —
// both route through the SAME A1-hardened reniceByPID (the #52 protected-process
// allowlist), so neither can ever deprioritize WindowServer, the kernel, audio,
// the session UI, or sirsi itself. renice is non-destructive and reversible: the
// process keeps running, just yields CPU, and its priority resets when it exits.
package guard

import (
	"fmt"
	"strings"
)

// ReliefCandidate is a live CPU offender that is safe to relieve (non-protected).
type ReliefCandidate struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
}

// FindReliefTarget returns the highest-CPU live process that is (a) above minCPU,
// (b) NOT a protected system process, and (c) — when nameHint is non-empty — whose
// name contains the hint. Returns (nil, nil) when nothing qualifies. Read-only: no
// side effects, so it is safe to call for a dry-run preview. Reuses the watchdog's
// injectable sampleTopCPU (Rule A16/A21), so it is testable without real processes.
func FindReliefTarget(nameHint string, minCPU float64) (*ReliefCandidate, error) {
	procs, err := sampleTopCPU(30)
	if err != nil {
		return nil, fmt.Errorf("scanning processes: %w", err)
	}
	hint := strings.ToLower(strings.TrimSpace(nameHint))
	for _, p := range procs { // sampleTopCPU returns highest-CPU first
		if p.CPUPercent < minCPU {
			break // sorted desc — nothing below this will qualify either
		}
		if isProtectedReniceTarget(p.Name) {
			continue // never relieve a critical process (defense in depth; reniceByPID also refuses)
		}
		if hint != "" && !strings.Contains(strings.ToLower(p.Name), hint) {
			continue
		}
		return &ReliefCandidate{PID: p.PID, Name: p.Name, CPUPercent: p.CPUPercent}, nil
	}
	return nil, nil
}

// Relieve lowers the candidate's scheduler priority (renice +10 + background QoS)
// via the A1-protected reniceByPID. Returns an error if the target is protected
// (belt-and-suspenders) or the OS refuses (e.g. a process the user doesn't own,
// which needs elevated privileges). NEVER kills anything.
func Relieve(c *ReliefCandidate) error {
	if c == nil {
		return fmt.Errorf("no relief target")
	}
	if err := reniceByPID(c.PID, c.Name); err != nil {
		return err
	}
	return nil
}
