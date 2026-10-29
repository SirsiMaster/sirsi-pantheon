package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/billingevidence"
	"github.com/spf13/cobra"
)

// This command reads a retained aggregate only. It has no provider client,
// service, credential, resource-admission or budget-enforcement side effects.
func newBillingEvidenceCmd() *cobra.Command {
	var maxAge time.Duration
	cmd := &cobra.Command{
		Use:   "billing-evidence [observation.json]",
		Short: "Report retained billing evidence without inventing available budget",
		Long: `Emit a JSON report from an optional retained aggregate observation.

No input means unknown, never zero or unlimited. A complete aggregate remains
delayed provider evidence, not a real-time spend ceiling or admission grant.
The supplied account/table/window are reported, not authenticated. No provider
query, credit lookup, export enablement, credential or billing mutation occurs.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var observation *billingevidence.Observation
			if len(args) == 1 {
				f, err := os.Open(args[0])
				if err != nil {
					return err
				}
				defer f.Close()
				observation, err = billingevidence.Decode(f)
				if err != nil {
					return err
				}
			}
			report, err := billingevidence.Assess(observation, time.Now(), maxAge)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		},
	}
	cmd.Flags().DurationVar(&maxAge, "max-age", 24*time.Hour, "maximum export observation age (reporting only, not enforcement)")
	return cmd
}

func init() { maatCmd.AddCommand(newBillingEvidenceCmd()) }
