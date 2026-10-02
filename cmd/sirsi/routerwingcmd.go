package main

// `sirsi router wing register <file>` — rs-31a (ADR-066 SNE design confirmed
// 2026-09-12, item 015930): the thin authenticated CLI over the router
// SERVICE/store that choice (b) calls for, NOT a filesystem-scanning second
// authority. This verb schema-validates the file and persists it atomically
// via routerstore.RegisterWing, returning the admitted receipt.
//
// Caller-authority binding (rs-31b: binding the caller to an INDEPENDENTLY
// established project/repo authority) and canonical-path containment
// enforcement (rs-31c) are separate, explicitly named sub-builds layered on
// top of this verb — not added here.

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var routerWingCmd = &cobra.Command{
	Use:   "wing",
	Short: "Stack Lab wing admission (ADR-066)",
}

var routerWingRegisterCmd = &cobra.Command{
	Use:   "register <file>",
	Short: "Schema-validate and admit a Stack Lab wing record",
	Long: `Reads <file>, schema-validates it against contracts/stacklab/v2/wing.schema.json,
and persists it atomically keyed by wing id.

Idempotent on identical bytes: registering the same wing id with the same
content hash returns the existing receipt. A conflicting identity (same id,
different bytes) is rejected — no silent overwrite.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read %s: %w", args[0], err)
		}
		store, err := openRouterStore()
		if err != nil {
			return err
		}
		defer store.Close()

		receipt, err := store.RegisterWing(raw)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "admitted wing %s (project=%s namespace=%s digest=%s created=%s)\n",
			receipt.WingID, receipt.ProjectID, receipt.RouterNamespace, receipt.ContentHash, receipt.Created)
		return nil
	},
}

func init() {
	routerWingCmd.AddCommand(routerWingRegisterCmd)
	routerCmd.AddCommand(routerWingCmd)
}
