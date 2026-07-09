package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/isis"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var (
	maatSudo bool
	maatFix  bool
	maatDocs bool

	// Audit flags
	auditSkipTests bool
	auditFull      bool

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

  sirsi maat audit            Run full governance assessment
  sirsi maat scales           Enforce infrastructure policies (Scales)
  sirsi maat heal             Autonomous remediation cycle (Isis)
  sirsi maat pulse            Dynamic measurement heartbeat`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if maatDocs {
			output.Info("Opening Ma'at docs...")
			return help.OpenDocs("maat")
		}
		return cmd.Help()
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

  sirsi maat pulse              Run all measurements
  sirsi maat pulse --skip-test  Skip go test (fast mode, ~2s)
  sirsi maat pulse --json       Output metrics as JSON to stdout`,
	RunE: runMaatPulse,
}

func init() {
	maatCmd.Flags().BoolVar(&maatDocs, "docs", false, "Open Ma'at web documentation in browser")

	maatAuditCmd.Flags().BoolVar(&maatSudo, "sudo", false, "Scan system-level governance")
	maatAuditCmd.Flags().BoolVar(&auditFull, "full", false, "Run a live go test -cover pass (slow); default uses cached coverage")
	maatAuditCmd.Flags().BoolVar(&auditSkipTests, "skip-test", false, "Deprecated: fast/cached is now the default")
	_ = maatAuditCmd.Flags().MarkHidden("skip-test")
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

	// Ma'at weighs a REPOSITORY. Outside one (the menubar shells this verb
	// from $HOME) the old path fell back to a hardcoded threshold set and
	// weighed the user's home directory — fabricating "❌ 59/100 fail" on an
	// owner-facing surface (2026-07-05 popover). Unmeasured is not unhealthy:
	// say so plainly and point at the fix (Rule A14; surfaces canon).
	if _, inRepo := findGoRepoRoot(); !inRepo {
		res := &output.CommandResult{
			Command:    "sirsi maat audit",
			BriefTitle: "Quality & Governance",
			Summary:    "Not inside a code repository — Ma'at has nothing to weigh here.",
			Status:     "unmeasured",
			Duration:   time.Since(start),
		}
		res.NextActions = append(res.NextActions, output.NextAction{
			Label:       "Audit a repository",
			Command:     "cd <your-repo> && sirsi maat audit",
			Description: "Ma'at weighs a repo's coverage, canon, and pipeline health from its root.",
		})
		res.Render()
		return nil
	}

	// Fast by default (cached coverage); --full runs the slow go test pass.
	// --skip-test is kept as a back-compat alias for the (now default) fast mode.
	skipTests := !auditFull || auditSkipTests

	if !JsonOutput {
		output.Banner()
		output.Header("Quality & Governance Audit")

		if skipTests {
			output.Info("Using cached coverage (fast). Run with --full for a live go test pass.")
		} else {
			output.Info("Running go test -cover ./... (streaming per-package results)")
		}
	}

	assessor := &maat.CoverageAssessor{
		Thresholds: maat.DefaultThresholds(),
		DiffOnly:   skipTests,
		SkipTests:  skipTests,
	}
	if !JsonOutput {
		assessor.ProgressFn = func(p maat.PackageProgress) {
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
		}
	}

	report, err := maat.Weigh(assessor)
	if err != nil {
		return err
	}

	if !JsonOutput {
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
	}

	elapsed := time.Since(start)

	cr := &output.CommandResult{
		Command:  "sirsi audit",
		Summary:  fmt.Sprintf("Quality score: %s %d/100 (%d passed, %d warnings, %d failures)", report.OverallVerdict.Icon(), report.OverallWeight, report.Passes, report.Warnings, report.Failures),
		Duration: elapsed,
	}
	cr.AddEvidence("Verdict", report.OverallVerdict.Icon()+" "+report.OverallVerdict.String())
	cr.AddEvidence("Feather weight", fmt.Sprintf("%d/100", report.OverallWeight))
	cr.AddEvidence("Passed", fmt.Sprintf("%d", report.Passes))
	if report.Warnings > 0 {
		cr.AddEvidence("Warnings", fmt.Sprintf("%d", report.Warnings))
	}
	if report.Failures > 0 {
		cr.AddEvidence("Failures", fmt.Sprintf("%d", report.Failures))
		cr.AddNextAction("sirsi maat heal", "Auto-remediate quality issues")
	}
	cr.AddNextAction("sirsi maat pulse", "Quick coverage summary")
	cr.AddNextAction("sirsi scan", "Scan for infrastructure waste")
	cr.Render()
	return nil
}

func runMaatScales(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("Quality Assessment")
	output.Footer(time.Since(start))
	output.NextSteps(output.SuggestSteps(suggest.Context{Deity: "maat", Subcommand: "scales"}))
	return nil
}

func runMaatHeal(cmd *cobra.Command, args []string) error {
	start := time.Now()
	res := &output.CommandResult{Command: "sirsi maat heal", BriefTitle: "Auto-Remediation"}

	// Fast by default: weigh from CACHED coverage (SkipTests), exactly like
	// audit post-#167. DiffOnly's cold-cache path falls back to the FULL
	// go test suite — behind the popover's heal button that was a silent
	// multi-minute "Working…" with no result, no cancel (owner, 2026-07-09:
	// "no interaction whatsoever"). The mechanical remediations heal can
	// apply don't need fresh coverage; --full opts into the deep weigh.
	report, _ := maat.Weigh(&maat.CoverageAssessor{
		Thresholds: maat.DefaultThresholds(),
		DiffOnly:   healFull,
		SkipTests:  !healFull,
	})

	findings := isis.FromMaatReport(report)
	if len(findings) == 0 {
		res.Summary = "The feather is balanced — no healing required."
		res.Status = "ok"
		res.Duration = time.Since(start)
		res.Render()
		return nil
	}

	healer := isis.NewHealer(".")
	hr := healer.Heal(findings, !maatFix)

	verb := "would be healed (pass --fix to apply)"
	if maatFix {
		verb = "healed"
	}
	res.Summary = fmt.Sprintf("%d finding(s): %d %s, %d cannot be auto-fixed.", len(findings), hr.Healed, verb, hr.Failed)
	res.Status = "ok"
	res.AddEvidence("Findings", fmt.Sprintf("%d", len(findings)))
	res.AddEvidence("Healed", fmt.Sprintf("%d", hr.Healed))
	res.AddEvidence("Not auto-fixable", fmt.Sprintf("%d", hr.Failed))
	if !healFull {
		res.NextActions = append(res.NextActions, output.NextAction{
			Label: "Deep heal (runs the test suite)", Command: "sirsi maat heal --full",
			Description: "Re-measures coverage live before healing — minutes, terminal-recommended.",
		})
	}
	res.Duration = time.Since(start)
	res.Render()
	return nil
}

func runMaatPulse(cmd *cobra.Command, args []string) error {
	start := time.Now()

	// Honor both the global --json flag and the local --json flag.
	wantJSON := pulseJSON || JsonOutput
	if !wantJSON {
		output.Banner()
		output.Header("Coverage Pulse")
		output.Info("Measuring all vital signs...")
	}

	cfg := maat.DefaultPulseConfig(".")
	cfg.SkipTests = pulseSkipTests
	cfg.Version = version

	// Try to find the built binary
	if _, err := os.Stat("sirsi"); err == nil {
		cfg.BinaryPath = "sirsi"
	}

	metrics, err := maat.Pulse(cfg)
	if err != nil {
		return fmt.Errorf("pulse failed: %w", err)
	}

	if wantJSON {
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

	output.Success("Metrics written to .sirsi/metrics.json")
	output.Footer(time.Since(start))
	actions := suggest.After(suggest.Context{Deity: "maat", Subcommand: "pulse"})
	var steps [][]string
	for _, a := range actions {
		steps = append(steps, []string{a.Command, a.Description})
	}
	output.NextSteps(steps)
	return nil
}
