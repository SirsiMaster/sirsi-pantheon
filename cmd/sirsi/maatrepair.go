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
const maatRepairLivenessWatchCheck = "liveness-watch"

var (
	maatRepairConfirm                 bool
	maatRepairLaunchdPreviewBinding   string
	maatRepairInitializeFailureMemory bool
	maatRepairInspectFailureMemory    bool
	maatRepairReobserveFailureMemory  bool
	// Seams keep command tests wholly local: no test should inspect a live
	// launchd database or mutate a real LaunchAgent while proving the receipt
	// contract.
	maatRepairReadDiagnosis               = pantheonDoctor
	maatRepairPreviewLaunchAgents         = router.PreviewDisabledManagedLaunchAgents
	maatRepairRestoreReviewedLaunchAgents = router.RestoreReviewedDisabledManagedLaunchAgents
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
		preview, err := maatRepairPreviewLaunchAgents()
		if err != nil {
			return fmt.Errorf("Ma'at launchd recovery preview: %w", err)
		}
		if err := preview.Validate(); err != nil {
			return fmt.Errorf("Ma'at launchd recovery preview is invalid: %w", err)
		}
		if !maatRepairConfirm {
			if JsonOutput || maatJSON {
				return emitJSON(preview)
			}
			fmt.Printf("Ma'at managed LaunchAgent preview (read-only):\n")
			for _, target := range preview.Targets {
				disposition := "enable and bootstrap"
				if target.WasLoaded {
					disposition = "enable; already loaded"
				}
				fmt.Printf("  %s · %s · plist sha256:%s\n", target.Label, disposition, target.SHA256)
			}
			fmt.Printf("Preview binding: %s\nNo service state was changed. Review this exact target set before confirming.\n", preview.Binding)
			return nil
		}
		if !isSHA256Hex(maatRepairLaunchdPreviewBinding) {
			return fmt.Errorf("Ma'at launchd recovery requires the binding from a fresh read-only preview")
		}
		if preview.Binding != maatRepairLaunchdPreviewBinding {
			return fmt.Errorf("Ma'at launchd recovery preview changed; no service state was changed. Refresh the case and review a new preview")
		}
		beforeReport, err := maatRepairReadDiagnosis()
		if err != nil {
			return fmt.Errorf("Ma'at launchd recovery preflight: read diagnostic state: %w", err)
		}
		before, found := activeMaatRepairFinding(beforeReport, maatRepairLaunchdDisabledCheck)
		if !found {
			return fmt.Errorf("Ma'at launchd recovery preflight: no actionable managed disabled-override finding exists; no launchd state changed")
		}

		recovery, repairErr := maatRepairRestoreReviewedLaunchAgents(maatRepairLaunchdPreviewBinding)
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
		operation += " from reviewed preview sha256=" + maatRepairLaunchdPreviewBinding
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
				{Label: "Reviewed targets", Value: joinedLaunchdPreviewLabels(preview.Targets)},
				{Label: "Preview binding", Value: preview.Binding},
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

func joinedLaunchdPreviewLabels(targets []router.ManagedLaunchdPreviewTarget) string {
	labels := make([]string, len(targets))
	for i, target := range targets {
		labels[i] = target.Label
	}
	return strings.Join(labels, ", ")
}

// maatRepairLivenessWatchCmd is the closed Ma'at resolution for an absent
// reboot-proof liveness watch. It never accepts a label, binary, plist, or
// arbitrary command from its caller: preflight and re-observation are both the
// same doctor check that created the case, and installation resolves the exact
// running Pantheon executable through the existing liveness installer.
var maatRepairLivenessWatchCmd = &cobra.Command{
	Use:   "liveness-watch",
	Short: "Install the managed local watch or inspect its Ma'at failure-memory state",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMaatLivenessWatchAction()
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
	maatRepairLaunchdDisabledCmd.Flags().StringVar(&maatRepairLaunchdPreviewBinding, "preview-binding", "", "exact binding returned by the latest read-only managed-label preview")
	maatRepairLivenessWatchCmd.Flags().BoolVar(&maatRepairConfirm, "confirm", false, "confirm bounded liveness-watch installation")
	maatRepairLivenessWatchCmd.Flags().BoolVar(&maatRepairInitializeFailureMemory, "initialize-failure-memory", false, "explicitly initialize the private per-user Ma'at failure-memory store")
	maatRepairLivenessWatchCmd.Flags().BoolVar(&maatRepairInspectFailureMemory, "inspect-failure-memory", false, "inspect default failure-memory and incomplete-attempt state without changing it")
	maatRepairLivenessWatchCmd.Flags().BoolVar(&maatRepairReobserveFailureMemory, "reobserve-incomplete", false, "re-observe an incomplete liveness-watch attempt without replaying installation")
	maatRepairCmd.AddCommand(maatRepairLaunchdDisabledCmd)
	maatRepairCmd.AddCommand(maatRepairLivenessWatchCmd)
	maatCmd.AddCommand(maatRepairCmd)
}
