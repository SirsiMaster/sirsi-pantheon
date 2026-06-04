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

	// 3. Advisory for the issues Pantheon must NOT auto-act on.
	for _, f := range report.Findings {
		if f.Severity < guard.SeverityWarn {
			continue
		}
		switch {
		case strings.Contains(f.Check, "Jetsam"), strings.Contains(f.Check, "RAM"),
			strings.Contains(f.Check, "Swap"), strings.Contains(f.Check, "Memory"):
			output.Warn("🧠 %s\n     → run `sirsi monitor` to find and quit the memory hogs (won't kill them for you)", f.Message)
			advisory++
		case strings.Contains(f.Check, "Kernel Panic"):
			output.Warn("💥 %s\n     → hardware/driver issue; `sirsi diagnose --json` has the faulting detail (not auto-fixable)", f.Message)
			advisory++
		case strings.Contains(f.Check, "drift"):
			output.Warn("📦 %s\n     → run `brew upgrade sirsi-pantheon`", f.Message)
			advisory++
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

// confirmFix prompts on stderr (stdout stays clean for piping). Default No.
func confirmFix(prompt string) bool {
	fmt.Fprintf(os.Stderr, "\n  %s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}
