package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/isis"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

var (
	isisFixMode      bool
	isisLintOnly     bool
	isisVetOnly      bool
	isisCoverageOnly bool
	isisCanonOnly    bool
	isisFullWeigh    bool
)

var isisCmd = &cobra.Command{
	Use:   "isis",
	Short: "𓁐 Isis — The Healer (Autonomous Remediation)",
	Long: `𓁐 Isis — The Healer

"She who reassembled Osiris from scattered pieces."

Isis autonomously remediates quality findings from Ma'at.
The healing cycle: Ma'at weighs → Isis heals → Ma'at re-weighs.

By default, runs in dry-run mode (preview only).
Use --fix to apply changes.`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var isisHealCmd = &cobra.Command{
	Use:   "heal",
	Short: "Run the Ma'at→Isis healing cycle",
	Long: `Heal runs the full remediation cycle:

1. Invokes Ma'at to weigh code quality (uses cached coverage by default)
2. Converts findings into Isis healing targets
3. Dispatches to strategy handlers (lint, vet, coverage, canon)
4. Reports results — what was healed, what needs manual attention

By default, this is a DRY RUN (preview). Use --fix to apply changes.
Coverage uses cached results for speed. Use --full-weigh to run go test.

  pantheon isis heal              Preview all remediations (~1s)
  pantheon isis heal --fix        Apply all safe fixes
  pantheon isis heal --lint-only  Only fix formatting issues
  pantheon isis heal --full-weigh Run go test for fresh coverage data`,
	RunE: runIsisHeal,
}

func init() {
	isisHealCmd.Flags().BoolVar(&isisFixMode, "fix", false, "Apply changes (default is dry-run preview)")
	isisHealCmd.Flags().BoolVar(&isisLintOnly, "lint-only", false, "Only run lint remediation")
	isisHealCmd.Flags().BoolVar(&isisVetOnly, "vet-only", false, "Only run vet remediation")
	isisHealCmd.Flags().BoolVar(&isisCoverageOnly, "coverage-only", false, "Only run coverage analysis")
	isisHealCmd.Flags().BoolVar(&isisCanonOnly, "canon-only", false, "Only run canon sync")
	isisHealCmd.Flags().BoolVar(&isisFullWeigh, "full-weigh", false, "Run full go test for coverage (slow)")

	isisCmd.AddCommand(isisHealCmd)
}

func runIsisHeal(cmd *cobra.Command, args []string) error {
	start := time.Now()

	// Find repo root
	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("𓁐 isis: %w", err)
	}

	dryRun := !isisFixMode

	mode := "DRY RUN"
	if !dryRun {
		mode = "FIX"
	}

	fmt.Printf("𓁐 Isis — The Healer [%s]\n", mode)
	fmt.Printf("  Project: %s\n\n", repoRoot)

	// Step 1: Weigh with Ma'at
	if isisFullWeigh {
		fmt.Print("  Step 1: Ma'at is weighing (full — this runs go test)... ")
	} else {
		fmt.Print("  Step 1: Ma'at is weighing (cached)... ")
	}
	maatReport, err := runMaatAssessment(repoRoot)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return err
	}
	fmt.Printf("done (%d assessments, %s)\n", len(maatReport.Assessments), time.Since(start).Round(time.Millisecond))

	// Step 2: Convert to Isis findings
	findings := isis.FromMaatReport(maatReport)
	if len(findings) == 0 {
		fmt.Println("\n  𓂀 The feather is perfectly balanced. No healing required.")
		return nil
	}
	fmt.Printf("  Step 2: %d finding(s) require healing\n", len(findings))

	// Step 3: Filter by strategy if requested
	if isisLintOnly || isisVetOnly || isisCoverageOnly || isisCanonOnly {
		findings = filterFindings(findings, repoRoot)
		if len(findings) == 0 {
			fmt.Println("\n  ℹ️  No findings match the selected filter.")
			return nil
		}
	}

	// Step 4: Heal
	fmt.Print("  Step 3: Isis is healing... ")
	healer := isis.NewHealer(repoRoot)
	report := healer.Heal(findings, dryRun)
	fmt.Printf("done (%s)\n\n", report.Duration.Round(time.Millisecond))

	// Step 5: Report
	fmt.Println(report.Format())

	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Printf("  Total cycle time: %s\n", elapsed)

	return nil
}

// runMaatAssessment runs Ma'at's assessment.
// By default uses cached coverage for speed (~instant).
// With --full-weigh, runs actual go test (slow but accurate).
func runMaatAssessment(repoRoot string) (*maat.Report, error) {
	coverageAssessor := &maat.CoverageAssessor{
		Thresholds:  maat.DefaultThresholds(),
		ProjectRoot: repoRoot,
		DiffOnly:    true,
	}

	if !isisFullWeigh {
		// Fast mode: use cached coverage only — no go test invocation.
		// If no cache exists, generate synthetic "no data" results so
		// Ma'at produces warning-level assessments (not a 5-min test run).
		coverageAssessor.Runner = func() (string, error) {
			cachePath := coverageAssessor.CachePath
			if cachePath == "" {
				home, _ := os.UserHomeDir()
				cachePath = filepath.Join(home, ".config", "pantheon", "maat", "coverage-cache.json")
			}
			data, err := os.ReadFile(cachePath)
			if err != nil {
				// No cache — return empty so Ma'at sees "no coverage data"
				return "", nil
			}
			return string(data), nil
		}
	}

	return maat.Weigh(coverageAssessor)
}

// filterFindings restricts findings to only the selected strategy domain.
func filterFindings(findings []isis.Finding, repoRoot string) []isis.Finding {
	var filtered []isis.Finding

	for _, f := range findings {
		switch {
		case isisLintOnly:
			s := isis.NewLintStrategy(repoRoot)
			if s.CanHeal(f) {
				filtered = append(filtered, f)
			}
		case isisVetOnly:
			s := isis.NewVetStrategy(repoRoot)
			if s.CanHeal(f) {
				filtered = append(filtered, f)
			}
		case isisCoverageOnly:
			s := isis.NewCoverageStrategy(repoRoot)
			if s.CanHeal(f) {
				filtered = append(filtered, f)
			}
		case isisCanonOnly:
			s := isis.NewCanonStrategy(repoRoot)
			if s.CanHeal(f) {
				filtered = append(filtered, f)
			}
		}
	}

	return filtered
}
