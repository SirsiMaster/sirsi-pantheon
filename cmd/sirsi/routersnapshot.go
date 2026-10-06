package main

import (
	"github.com/spf13/cobra"
)

// routerSnapshotCmd prints the exact snapshot the Horus dashboard serves at
// /api/router, from the same producer (collectDashboardRouter), as JSON. It exists so
// the native menubar (which shells out to `sirsi ... --json`, never HTTP) renders the
// router's own decisions instead of recomputing them: one producer, two surfaces.
var routerSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Print the router snapshot Horus shows (same producer as /api/router) as JSON",
	Long: `Prints the router snapshot as JSON: lane verdicts with their reason and evidence,
open queue by recipient, consumer slots, registry pin, known failures, release notes,
swap, and the attention list with typed next steps, plus generated_at, built_ms and
per-stage timings. Output is always JSON (there is no human form: use the dashboard or
'sirsi router ping' for that). A lane's missing evidence field means unknown, never healthy.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		snap, err := collectDashboardRouter()
		if err != nil {
			return err
		}
		return jsonPrint(snap)
	},
}

func init() {
	routerCmd.AddCommand(routerSnapshotCmd)
}
