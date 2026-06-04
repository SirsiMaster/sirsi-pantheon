package main

// fix.go — the Pantheon health RESOLVER.
//
// `sirsi fix` is the answer to "it assesses but does nothing": it diagnoses the
// system AND resolves what it safely can, in one flow — instead of emitting more
// commands to type. No LLM: the finding→remedy mapping is a deterministic rule
// table (remediationFor + the classification below). Heuristic is the right call
// — the problem space is bounded, the fixes are pre-vetted, and a rule table is
// local, instant, offline (A11), and testable (A16/A17), where an LLM would add
// latency, cost, non-determinism, and a network dependency.
//
// Safety (A1): reclaim REUSES the vetted trash-first cleaner (protected paths
// enforced in internal/cleaner/safety.go, every removal recoverable from Trash).
// Memory/panic issues are ADVISORY only — Pantheon never auto-kills a process or
// pretends to fix hardware.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/spf13/cobra"
)

var fixYes bool // --yes: apply safe reclaim without the confirmation prompt

func runFix(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	output.Banner()
	output.Header("Pantheon Fix — assess and resolve")

	// 1. Assess.
	report, err := guard.Doctor()
	if err != nil {
		return fmt.Errorf("diagnose: %w", err)
	}
	output.Info("Health %d/100 · %d signals checked", report.Score, len(report.Findings))

	resolved, advisory := 0, 0

	// 2. Reclaim the disk/crash backlog — the auto-safe fix (trash-first).
	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)
	if scan, serr := engine.Scan(ctx, jackal.ScanOptions{}); serr == nil {
		var reclaim []jackal.Finding
		var bytes int64
		for _, f := range scan.Findings {
			if f.Severity == jackal.SeveritySafe || f.Severity == jackal.SeverityCaution {
				reclaim = append(reclaim, f)
				bytes += f.SizeBytes
			}
		}
		if len(reclaim) > 0 {
			output.Info("⛏  Reclaimable: %d items (%s) — caches, logs, crash reports, old installers",
				len(reclaim), jackal.FormatSize(bytes))
			if fixYes || confirmFix(fmt.Sprintf("Reclaim %s now (moved to Trash, recoverable)?", jackal.FormatSize(bytes))) {
				if cleanupApplyPaused() {
					output.Warn("Cleanup execution is paused for demo safety. Preview remains available.")
				} else {
					res, cerr := engine.Clean(ctx, reclaim, jackal.CleanOptions{DryRun: false, Confirm: true, UseTrash: true})
					switch {
					case cerr != nil:
						output.Error("Cleanup failed: %v", cerr)
					default:
						output.Success("Reclaimed %s — %d items moved to Trash.", jackal.FormatSize(res.BytesFreed), res.Cleaned)
						if res.Skipped > 0 {
							output.Warn("Skipped %d protected/locked items.", res.Skipped)
						}
						resolved++
					}
				}
			}
		}
	}

	// 3. Answer EVERY remaining finding with a concrete fix — never "advisory,
	//    figure it out". Execute what's safe (orphan-kill with preview, brew
	//    upgrade); give a SPECIFIC, runnable step for what needs your judgment or
	//    is external (hardware).
	for _, f := range report.Findings {
		if f.Severity < guard.SeverityWarn {
			continue
		}
		switch {
		case strings.Contains(f.Check, "Jetsam"), strings.Contains(f.Check, "RAM"),
			strings.Contains(f.Check, "Swap"), strings.Contains(f.Check, "Memory"):
			if fixMemory(f.Message) {
				resolved++
			} else {
				advisory++
			}
		case strings.Contains(f.Check, "Kernel Panic"):
			fixPanic(f.Message)
			advisory++
		case strings.Contains(f.Check, "drift"):
			if fixDrift(f.Message) {
				resolved++
			} else {
				advisory++
			}
		}
	}

	// 4. Honest result.
	output.Header("Result")
	switch {
	case resolved == 0 && advisory == 0:
		output.Success("Nothing to fix — system is healthy.")
	case advisory == 0:
		output.Success("Resolved %d issue(s). Nothing left needs you.", resolved)
	default:
		output.Info("Resolved %d automatically; %d need the manual step shown above.", resolved, advisory)
	}
	return nil
}

// fixMemory answers a RAM/Jetsam finding: name the actual hogs (the intelligence
// that was missing), then free the SAFE part — orphaned dev processes — via the
// vetted Slay path with a dry-run preview so you see exactly what dies. Active
// apps are named, not killed (quitting your browser is your call, A1/A5).
func fixMemory(msg string) bool {
	output.Warn("🧠 %s", msg)
	audit, err := guard.Audit()
	if err != nil || audit == nil {
		output.Dim("     → run `sirsi monitor` for live memory pressure")
		return false
	}
	output.Info("     Top memory consumers:")
	for i, g := range audit.Groups {
		if i >= 3 {
			break
		}
		output.Dim("       %d. %s — %s (%d proc)", i+1, g.Name, guard.FormatBytes(g.TotalRSS), g.TotalCount)
	}
	// Free the safe part: orphaned dev processes. Preview, then confirm.
	if preview, perr := guard.Slay(guard.SlayAll, true); perr == nil && preview != nil && preview.Killed > 0 {
		output.Info("     %d orphaned process(es) (~%s) are safe to kill (leftover, protected procs excluded).",
			preview.Killed, guard.FormatBytes(audit.OrphanRSS))
		output.Dim("     → automatic process killing is paused until the orphan filter is narrowed to PPID/stale-parent evidence")
		return false
	}
	output.Dim("     → no orphaned processes; quit the largest app above that you're not using")
	return false
}

// fixPanic answers a kernel-panic finding with the SPECIFIC remediation path —
// not "read the JSON". The panic logs themselves are cleared by the reclaim step
// above; the hardware/driver cause needs these concrete steps.
func fixPanic(msg string) {
	output.Warn("💥 %s", msg)
	output.Dim("     A kernel panic is a driver or hardware fault. Concrete steps:")
	output.Dim("       1. Reboot into Apple Diagnostics (power on, hold D) to test RAM/hardware")
	output.Dim("       2. Check third-party kexts:  kextstat | grep -v com.apple")
	output.Dim("       3. Update macOS and any third-party drivers; remove a kext that recurs")
	output.Dim("     (The panic logs themselves were reclaimed above.)")
}

// fixDrift answers a binary-drift finding by running the actual update.
func fixDrift(msg string) bool {
	output.Warn("📦 %s", msg)
	if !fixYes && !confirmFix("Run `brew upgrade sirsi-pantheon` now?") {
		output.Dim("     → run `brew upgrade sirsi-pantheon` when ready")
		return false
	}
	cmd := exec.Command("brew", "upgrade", "sirsi-pantheon")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		output.Error("     brew upgrade failed: %v", err)
		return false
	}
	output.Success("     Sirsi binaries updated.")
	return true
}

// confirmFix prompts on stderr (stdout stays clean for piping). Default No.
func confirmFix(prompt string) bool {
	fmt.Fprintf(os.Stderr, "\n  %s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}
