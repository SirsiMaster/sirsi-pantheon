package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatScreenInput            string
	maatScreenConfirm          bool
	maatScreenExpectedHead     string
	maatScreenPreview          bool
	maatScreenExpectedInputSHA string
)

// maatScreenCmd turns a closed, typed local observation into Ma'at's
// JEV-like System One verdict. It is deliberately provider-neutral: the
// deterministic policy validates the supplied observation, refuses malformed
// schema, and appends the same result that Casebook projects everywhere.
var maatScreenCmd = &cobra.Command{
	Use:   "screen --input <system-one-screen.json>",
	Short: "Record an unqualified ADR070 v2 source screen",
	Long: `Import an explicit maat.adr070.source-screen v2 packet pinned by --expect-head.

Imported floor checks are declared evidence: this command does not execute or
authenticate them. Failed/incomplete floor blocks; otherwise unqualified screens
escalate. Imported qualified claims reject. Legacy v1 journal reads and local host
triage are separate. No mutation, release, router gate or binding approval results.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if maatScreenPreview && maatScreenConfirm {
			return fmt.Errorf("--preview and --confirm are mutually exclusive")
		}
		if strings.TrimSpace(maatScreenInput) == "" {
			return fmt.Errorf("--input is required")
		}
		raw, err := readMaatSourceScreen(maatScreenInput)
		if err != nil {
			return fmt.Errorf("read Ma'at System One input: %w", err)
		}
		sum := sha256.Sum256(raw)
		inputSHA := hex.EncodeToString(sum[:])
		if maatScreenExpectedInputSHA != "" && maatScreenExpectedInputSHA != inputSHA {
			return fmt.Errorf("source input digest changed or malformed; preview and review the selected file again")
		}
		input, err := maat.DecodeSourceScreen(raw)
		if err != nil {
			return err
		}
		verdict, err := maat.ScreenSource(input, maatScreenExpectedHead)
		if err != nil {
			return err
		}
		if maatScreenPreview {
			return emitJSON(struct {
				maat.MaatVerdict
				InputSHA256 string `json:"input_sha256"`
			}{verdict, inputSHA})
		}
		// A valid screen is still a durable journal write. Validate the closed
		// packet first so an operator gets useful feedback, then require the same
		// explicit confirmation used by Ma'at owner review and acceptance before
		// opening the decision writer.
		if !maatScreenConfirm {
			return fmt.Errorf("recording a Ma'at System One screen writes the decision journal; rerun with --confirm after reviewing the deterministic gate")
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return fmt.Errorf("open Ma'at decision journal: %w", err)
		}
		decision, err := maat.RecordSystemOne(journal, "sirsi maat screen", verdict)
		if err != nil {
			return err
		}
		if JsonOutput || maatJSON {
			return emitJSON(verdict)
		}
		result := &output.CommandResult{
			Command:    "sirsi maat screen",
			BriefTitle: "Ma'at System One",
			Status:     string(verdict.Gate),
			Summary:    systemOneSummary(verdict),
			Evidence: []output.Evidence{
				{Label: "Subject", Value: verdict.Subject.Kind + " " + verdict.Subject.Ref},
				{Label: "Head", Value: verdict.Subject.HeadSHA},
				{Label: "Evidence", Value: decision.Evidence},
			},
			NextActions: []output.NextAction{{
				Label: "Review Ma'at casebook", Command: "sirsi maat casebook",
				Description: "Inspect the same evidence-bound System One result in the shared local Casebook.",
			}},
		}
		if verdict.Gate == maat.GateEscalate {
			result.NextActions = append([]output.NextAction{{
				Label: "Request frontier review", Command: "sirsi maat casebook --kind governance --status open",
				Description: "A screen is not a binding decision. Review the retained evidence and record an explicit owner conclusion when appropriate.",
			}}, result.NextActions...)
		}
		result.Render()
		return nil
	},
}

func systemOneSummary(verdict maat.MaatVerdict) string {
	if verdict.SchemaVersion == maat.SourceScreenSchemaVersion {
		return "Nonbinding ADR070 source import: " + string(verdict.Gate) + ". Floor evidence is declared only; no checks or authenticated calibration ran."
	}
	switch verdict.Gate {
	case maat.GatePass:
		return "Local System One found no issues above its configured threshold. This is evidence, not mutation authority."
	case maat.GateChanges:
		return "Local System One found bounded changes to address. The Casebook retains the evidence and a guided decision route."
	case maat.GateBlock:
		return "Local System One blocked this screen on deterministic or high-confidence evidence. It did not change the assessed system."
	case maat.GateEscalate:
		return "Local System One escalated this screen. A sensitive boundary or insufficient confidence requires an explicit frontier review."
	default:
		return "Local System One recorded an unrecognized screen result."
	}
}

func init() {
	maatScreenCmd.Flags().StringVar(&maatScreenInput, "input", "", "closed Ma'at System One screen JSON")
	maatScreenCmd.Flags().BoolVar(&maatScreenConfirm, "confirm", false, "confirm recording the validated Ma'at System One screen")
	maatScreenCmd.Flags().StringVar(&maatScreenExpectedHead, "expect-head", "", "required independently selected source head; not imported from the packet")
	maatScreenCmd.Flags().BoolVar(&maatScreenPreview, "preview", false, "validate and return declared nonbinding v2 evidence and exact input digest without journal writes")
	maatScreenCmd.Flags().StringVar(&maatScreenExpectedInputSHA, "expect-input-sha256", "", "deny recording if the exact reviewed input bytes changed (never grants authority)")
	maatScreenCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatCmd.AddCommand(maatScreenCmd)
}

func readMaatSourceScreen(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maat.SourceScreenMaxBytes {
		return nil, fmt.Errorf("source input must be a regular file of at most 1 MiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maat.SourceScreenMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maat.SourceScreenMaxBytes {
		return nil, fmt.Errorf("source input exceeds 1 MiB")
	}
	return raw, nil
}
