package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	reopenReason string
	reopenAgent  string
)

var routerReopenCmd = &cobra.Command{
	Use:   "reopen <id> --reason <text|@file>",
	Short: "Undo a close: return a closed item to open, keeping its close result",
	Long: `Returns a closed item to open. The previous close result is preserved in the
item's body together with who reopened it, when, and why, so nothing is lost.
Same authority as close: the item's recipient, or an actor with close:any. An
item addressed to the owner can only be reopened by the owner.

  sirsi router reopen 20260930-180909-… --reason @why.md
  sirsi router reopen <id> --reason "closed in error; still a live request"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reason, err := loadOrLiteralIfSet(cmd, "reason", reopenReason)
		if err != nil {
			return err
		}
		f, actor, err := openFacadeAs(reopenAgent)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if err := f.Reopen(actor, args[0], reason); err != nil {
			return err
		}
		fmt.Printf("  Reopened %s\n", args[0])
		return nil
	},
}

func init() {
	routerReopenCmd.Flags().StringVar(&reopenReason, "reason", "", "Why the item is being reopened (literal text, or @file); required")
	routerReopenCmd.Flags().StringVar(&reopenAgent, "agent", "", "Acting agent id (otherwise resolved from the current session)")
	routerCmd.AddCommand(routerReopenCmd)
}
