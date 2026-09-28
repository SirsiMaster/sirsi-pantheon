package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatCalibrationScreenEvidence   string
	maatCalibrationFrontierEvidence string
	maatCalibrationFrontierGate     string
)

// maatCalibrateCmd records an independently supplied final review outcome
// for an existing local auto-pass. It never performs the review, starts work,
// or changes the assessed system.
var maatCalibrateCmd = &cobra.Command{
	Use:   "calibrate --screen-evidence <ref> --frontier-evidence <ref> --frontier-gate <pass|changes|block>",
	Short: "Record an independent review outcome for System One calibration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return fmt.Errorf("open Ma'at decision journal: %w", err)
		}
		decision, calibration, err := maat.RecordSystemOneCalibration(journal, "sirsi maat calibrate", maatCalibrationScreenEvidence, maatCalibrationFrontierEvidence, maat.GateDecision(strings.TrimSpace(maatCalibrationFrontierGate)))
		if err != nil {
			return err
		}
		if JsonOutput || maatJSON {
			return emitJSON(struct {
				Evidence    string           `json:"evidence"`
				Calibration maat.Calibration `json:"calibration"`
			}{Evidence: decision.Evidence, Calibration: calibration})
		}
		result := &output.CommandResult{
			Command: "sirsi maat calibrate", BriefTitle: "Ma'at System One Calibration", Status: "ok",
			Summary:     fmt.Sprintf("Recorded %d auto-pass sample(s); %d were overturned (%.1f%%).", calibration.AutoPasses, calibration.AutoPassOverturn, calibration.OverturnRate*100),
			Evidence:    []output.Evidence{{Label: "Calibration evidence", Value: decision.Evidence}, {Label: "Screen", Value: maatCalibrationScreenEvidence}, {Label: "Independent outcome", Value: strings.TrimSpace(maatCalibrationFrontierGate)}},
			NextActions: []output.NextAction{{Label: "Review Ma'at casebook", Command: "sirsi maat casebook --kind governance", Description: "Inspect the screen, independent review link, and calibration record together."}},
		}
		result.Render()
		return nil
	},
}

func init() {
	maatCalibrateCmd.Flags().StringVar(&maatCalibrationScreenEvidence, "screen-evidence", "", "exact evidence reference of a recorded local System One auto-pass")
	maatCalibrateCmd.Flags().StringVar(&maatCalibrationFrontierEvidence, "frontier-evidence", "", "exact evidence reference from an independent review")
	maatCalibrateCmd.Flags().StringVar(&maatCalibrationFrontierGate, "frontier-gate", "", "final independent gate: pass, changes, or block")
	maatCalibrateCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatCmd.AddCommand(maatCalibrateCmd)
}
