package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

const maatRepairLaunchdDisabledCheck = "launchd Disabled Override"

var (
	maatRepairConfirm bool
	// Seams keep command tests wholly local: no test should inspect a live
	// launchd database or mutate a real LaunchAgent while proving the receipt
	// contract.
	maatRepairReadDiagnosis       = guard.Doctor
	maatRepairRestoreLaunchAgents = router.RestoreDisabledManagedLaunchAgents
)

// maatRepairCmd is deliberately a closed repair registry. Ma'at never accepts
// a caller supplied command, label, plist, or shell fragment as repair input.
// Each child must preflight, perform its own bounded operation, re-observe the
// same check, and retain a truthful journal outcome.
var maatRepairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Run a bounded Ma'at repair and retain its verified outcome",
}

var maatRepairLaunchdDisabledCmd = &cobra.Command{
	Use:   "launchd-disabled",
	Short: "Restore only verified managed launchd labels disabled by an override",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !maatRepairConfirm {
			return fmt.Errorf("Ma'at launchd recovery changes managed launchd state; rerun with --confirm after reviewing the exact disabled labels")
		}
		beforeReport, err := maatRepairReadDiagnosis()
		if err != nil {
			return fmt.Errorf("Ma'at launchd recovery preflight: read diagnostic state: %w", err)
		}
		before, found := activeMaatRepairFinding(beforeReport, maatRepairLaunchdDisabledCheck)
		if !found {
			return fmt.Errorf("Ma'at launchd recovery preflight: no actionable managed disabled-override finding exists; no launchd state changed")
		}

		recovery, repairErr := maatRepairRestoreLaunchAgents()
		afterReport, observationErr := maatRepairReadDiagnosis()
		after, cleared := "post-repair diagnostic unavailable", false
		if observationErr == nil {
			after, cleared = maatRepairFindingState(afterReport, maatRepairLaunchdDisabledCheck)
		}

		journal, journalErr := newMaatDecisionJournal()
		if journalErr != nil {
			return fmt.Errorf("Ma'at launchd recovery: open outcome journal after managed repair: %w", journalErr)
		}
		operation := "enable only the preflight-disabled managed labels; bootstrap only those still unloaded"
		outcome := maat.DiagnosticRepair{
			Check: maatRepairLaunchdDisabledCheck, Operation: operation, Before: before, After: after,
			Determination: "failed",
		}
		if repairErr != nil {
			outcome.Detail = "bounded recovery returned an error: " + repairErr.Error()
		} else if observationErr != nil {
			outcome.Detail = "post-repair diagnostic could not be read: " + observationErr.Error()
		} else if !cleared {
			outcome.Detail = "post-repair diagnostic still reports an actionable managed disabled override"
		} else {
			outcome.Determination = "resolved"
			outcome.Detail = fmt.Sprintf("enabled: %s; bootstrapped: %s", joinedLabels(recovery.Enabled), joinedLabels(recovery.Bootstrapped))
		}
		decision, recordErr := maat.RecordDiagnosticRepair(journal, "sirsi maat repair launchd-disabled", outcome)
		if recordErr != nil {
			return fmt.Errorf("Ma'at launchd recovery: retain verified outcome: %w", recordErr)
		}

		result := &output.CommandResult{
			Command:    "sirsi maat repair launchd-disabled",
			BriefTitle: "Ma'at managed LaunchAgent recovery",
			Status:     "ok",
			Summary:    "Managed LaunchAgent recovery was verified and recorded by Ma'at.",
			Evidence: []output.Evidence{
				{Label: "Preflight", Value: before},
				{Label: "Post-repair", Value: after},
				{Label: "Ma'at receipt", Value: decision.Evidence},
			},
			NextActions: []output.NextAction{{
				Label: "Review Ma'at casebook", Command: "sirsi maat casebook", Description: "Inspect the retained repair receipt and its verification evidence.",
			}},
		}
		if outcome.Determination != "resolved" {
			result.Status = "error"
			result.Summary = "Managed LaunchAgent recovery did not verify cleanly. Ma'at retained the incomplete outcome; retry only after resolving the stated error."
			result.Errors = []string{outcome.Detail}
			result.Render()
			if repairErr != nil {
				return fmt.Errorf("Ma'at launchd recovery incomplete: %w", repairErr)
			}
			if observationErr != nil {
				return fmt.Errorf("Ma'at launchd recovery could not verify: %w", observationErr)
			}
			return fmt.Errorf("Ma'at launchd recovery did not clear the managed disabled override")
		}
		result.Render()
		return nil
	},
}

func activeMaatRepairFinding(report *guard.DoctorReport, check string) (string, bool) {
	if report == nil {
		return "", false
	}
	for _, finding := range report.Findings {
		if finding.Check == check && finding.Severity >= guard.SeverityWarn {
			return maatRepairFindingDescription(finding), true
		}
	}
	return "", false
}

// maatRepairFindingState returns a stable post-repair description and whether
// the same diagnostic check is now non-actionable. It refuses to treat a
// missing report as success; an absent check is fine only after the diagnostic
// itself completed, which the caller verifies before calling this helper.
func maatRepairFindingState(report *guard.DoctorReport, check string) (string, bool) {
	if report == nil {
		return "post-repair diagnostic unavailable", false
	}
	for _, finding := range report.Findings {
		if finding.Check == check {
			return maatRepairFindingDescription(finding), finding.Severity < guard.SeverityWarn
		}
	}
	return "diagnostic no longer emitted this check", true
}

func maatRepairFindingDescription(finding guard.DiagnosticFinding) string {
	parts := []string{strings.TrimSpace(finding.Message)}
	if detail := strings.TrimSpace(finding.Detail); detail != "" {
		parts = append(parts, detail)
	}
	return strings.Join(parts, " | ")
}

func init() {
	maatRepairLaunchdDisabledCmd.Flags().BoolVar(&maatRepairConfirm, "confirm", false, "confirm bounded managed LaunchAgent recovery")
	maatRepairCmd.AddCommand(maatRepairLaunchdDisabledCmd)
	maatCmd.AddCommand(maatRepairCmd)
}
