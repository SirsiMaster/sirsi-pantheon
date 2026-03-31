package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/isis"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatSudo bool
	maatFix  bool

	// Audit flags
	auditSkipTests bool

	// Isis / Heal flags
	healFull bool

	// Pulse flags
	pulseSkipTests bool
	pulseJSON      bool
)

var maatCmd = &cobra.Command{
	Use:   "maat",
	Short: "𓆄 Ma'at — QA/QC Governance & Policy Enforcement",
	Long: `𓆄 Ma'at — The Goddess of Truth, Balance, and Cosmic Order

Ma'at manages your workstation's governance and ensures all infrastructure
complies with the Pantheon Charter. It balances the Scale of Truth.

  pantheon maat audit            Run full governance assessment
  pantheon maat scales           Enforce infrastructure policies (Scales)
  pantheon maat heal             Autonomous remediation cycle (Isis)
  pantheon maat pulse            Dynamic measurement heartbeat`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var maatAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "𓆄 Full workstation governance and compliance scan",
	RunE:  runMaatAudit,
}

var maatScalesCmd = &cobra.Command{
	Use:   "scales",
	Short: "𓆄 Enforce infrastructure policies and resolve drifts",
	RunE:  runMaatScales,
}

var maatHealCmd = &cobra.Command{
	Use:   "heal",
	Short: "𓆄 Autonomous remediation cycle (Ma'at → Isis)",
	RunE:  runMaatHeal,
}

var maatPulseCmd = &cobra.Command{
	Use:   "pulse",
	Short: "𓆄 Dynamic measurement heartbeat — the single source of truth",
	Long: `𓆄 Ma'at Pulse — The Heartbeat of Truth

Runs real measurements across the entire Pantheon codebase and writes
a structured .pantheon/metrics.json that all downstream consumers can read:

  • CI pipeline uploads it as an artifact
  • VS Code extension reads it for dynamic status bar numbers
  • BUILD_LOG references it instead of hardcoded strings
  • Thoth sync reads it to update memory.yaml with real numbers

  pantheon maat pulse              Run all measurements
  pantheon maat pulse --skip-test  Skip go test (fast mode, ~2s)
  pantheon maat pulse --json       Output metrics as JSON to stdout`,
	RunE: runMaatPulse,
}

func init() {
	maatAuditCmd.Flags().BoolVar(&maatSudo, "sudo", false, "Scan system-level governance")
	maatAuditCmd.Flags().BoolVar(&auditSkipTests, "skip-test", false, "Skip go test (use cached coverage only)")
	maatScalesCmd.Flags().BoolVar(&maatFix, "fix", false, "Actually apply policy fixes")

	maatHealCmd.Flags().BoolVar(&maatFix, "fix", false, "Apply healing remedies")
	maatHealCmd.Flags().BoolVar(&healFull, "full", false, "Run full (slow) test suite")

	maatPulseCmd.Flags().BoolVar(&pulseSkipTests, "skip-test", false, "Skip go test (fast mode)")
	maatPulseCmd.Flags().BoolVar(&pulseJSON, "json", false, "Output metrics as JSON to stdout")

	maatCmd.AddCommand(maatAuditCmd)
	maatCmd.AddCommand(maatScalesCmd)
	maatCmd.AddCommand(maatHealCmd)
	maatCmd.AddCommand(maatPulseCmd)
}

func runMaatAudit(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("MA'AT — Governance Audit")

	if auditSkipTests {
		output.Info("Skipping tests — using cached coverage only")
	} else {
		output.Info("Running go test -cover ./... (streaming per-package results)")
	}

	assessor := &maat.CoverageAssessor{
		Thresholds: maat.DefaultThresholds(),
		DiffOnly:   !auditSkipTests,
		SkipTests:  auditSkipTests,
		ProgressFn: func(p maat.PackageProgress) {
			prefix := fmt.Sprintf("  [%d/%d]", p.Current, p.Total)
			switch {
			case p.NoTests:
				output.Dim("%s %s — no test files", prefix, p.Package)
			case p.Failed:
				output.Error("%s %s — FAIL (%.1f%%)", prefix, p.Package, p.Coverage)
			case p.Coverage >= 80:
				output.Success("%s %s — %.1f%% coverage", prefix, p.Package, p.Coverage)
			case p.Coverage >= 50:
				output.Warn("%s %s — %.1f%% coverage", prefix, p.Package, p.Coverage)
			default:
				output.Error("%s %s — %.1f%% coverage", prefix, p.Package, p.Coverage)
			}
		},
	}

	report, err := maat.Weigh(assessor)
	if err != nil {
		return err
	}

	// Print per-module verdict table.
	var rows [][]string
	for _, a := range report.Assessments {
		rows = append(rows, []string{
			a.Verdict.Icon(),
			a.Subject,
			a.Message,
			fmt.Sprintf("%d", a.FeatherWeight),
		})
	}
	if len(rows) > 0 {
		output.Table([]string{"", "Module", "Result", "Weight"}, rows)
	}

	output.Dashboard(map[string]string{
		"Verdict":  report.OverallVerdict.Icon() + " " + report.OverallVerdict.String(),
		"Weight":   fmt.Sprintf("%d/100", report.OverallWeight),
		"Passed":   fmt.Sprintf("%d", report.Passes),
		"Warnings": fmt.Sprintf("%d", report.Warnings),
		"Failures": fmt.Sprintf("%d", report.Failures),
	})
	output.Footer(time.Since(start))
	return nil
}

func runMaatScales(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("MA'AT — The Scales of Balance")
	output.Footer(time.Since(start))
	return nil
}

func runMaatHeal(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("MA'AT — The Healing Pulse (Isis)")

	// Step 1: Weigh
	report, _ := maat.Weigh(&maat.CoverageAssessor{Thresholds: maat.DefaultThresholds(), DiffOnly: !healFull})

	// Step 2: Heal
	findings := isis.FromMaatReport(report)
	if len(findings) == 0 {
		output.Success("The feather is balanced. No healing required.")
		return nil
	}

	healer := isis.NewHealer(".")
	res := healer.Heal(findings, !maatFix)

	output.Dashboard(map[string]string{
		"Findings": fmt.Sprintf("%d", len(findings)),
		"Healed":   fmt.Sprintf("%d", res.Healed),
		"Failed":   fmt.Sprintf("%d", res.Failed),
	})
	output.Footer(time.Since(start))
	return nil
}

func runMaatPulse(cmd *cobra.Command, args []string) error {
	start := time.Now()

	if !pulseJSON {
		output.Banner()
		output.Header("MA'AT — The Pulse of Truth")
		output.Info("Measuring all vital signs...")
	}

	cfg := maat.DefaultPulseConfig(".")
	cfg.SkipTests = pulseSkipTests
	cfg.Version = version

	// Try to find the built binary
	if _, err := os.Stat("pantheon"); err == nil {
		cfg.BinaryPath = "pantheon"
	}

	metrics, err := maat.Pulse(cfg)
	if err != nil {
		return fmt.Errorf("pulse failed: %w", err)
	}

	if pulseJSON {
		// Pure JSON to stdout — perfect for CI consumption
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(metrics)
	}

	// ── Beautiful dashboard output ──────────────────────────────
	output.Dashboard(map[string]string{
		"Tests":     fmt.Sprintf("%d passed / %d failed / %d skipped", metrics.TestsPassed, metrics.TestsFailed, metrics.TestsSkipped),
		"Coverage":  fmt.Sprintf("%.1f%%", metrics.Coverage),
		"Source":    fmt.Sprintf("%d lines (%d files)", metrics.SourceLines, metrics.SourceFiles),
		"Go Source": fmt.Sprintf("%d lines", metrics.GoSourceLines),
		"Binary":    metrics.BinarySizeHuman,
		"Deities":   fmt.Sprintf("%d", metrics.Deities),
		"Modules":   fmt.Sprintf("%d", metrics.Modules),
	})

	output.Success("Metrics written to .pantheon/metrics.json")
	output.Footer(time.Since(start))
	return nil
}
