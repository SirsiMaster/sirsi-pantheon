package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ── lane-to-Mac pins (schema v30, owner decision 2026-11-25) ───────────────
// Run ON THE SERVICE HOST against the service's own backend (--store), exactly like `router token`. Never
// served over the wire and never forwarded by the relay: a lane cannot set, retire or read a pin. REPORT-ONLY:
// a pin changes no request outcome today; the service only counts match/mismatch at Wait.

var routerLanePinStore string

var routerLanePinCmd = &cobra.Command{
	Use:   "lane-pin",
	Short: "Set, retire or list which Mac each lane belongs to (router operator only; report-only today; run on the service host)",
}

var routerLanePinSetCmd = &cobra.Command{
	Use:   "set <agent> <machine-id>",
	Short: "Pin a lane to a machine id (a lane may have several active pins); idempotent",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openServeStore(routerLanePinStore)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		by, _ := cmd.Flags().GetString("by")
		reason, _ := cmd.Flags().GetString("reason")
		p, err := store.SetLanePin(args[0], args[1], by, reason)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "pinned %s -> %s  by %s  created %s (report-only: no request is refused)\n", p.Agent, p.MachineID, p.SetBy, p.Created)
		return nil
	},
}

var routerLanePinRetireCmd = &cobra.Command{
	Use:   "retire <agent> <machine-id>",
	Short: "Retire an active pin (the row is kept as history)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openServeStore(routerLanePinStore)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		if err := store.RetireLanePin(args[0], args[1]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "retired %s -> %s (a running service sees it within 60s)\n", args[0], args[1])
		return nil
	},
}

var routerLanePinListCmd = &cobra.Command{
	Use:   "list",
	Short: "List lane pins (active only unless --all)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := openServeStore(routerLanePinStore)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		all, _ := cmd.Flags().GetBool("all")
		list, err := store.ListLanePins(all)
		if err != nil {
			return err
		}
		for _, p := range list {
			state := "active"
			if p.Retired != "" {
				state = "retired " + p.Retired
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s by %-12s %s  %s  %s\n", p.Agent, p.MachineID, p.SetBy, p.Created, state, strings.TrimSpace(p.Reason))
		}
		return nil
	},
}

func init() {
	routerLanePinCmd.PersistentFlags().StringVar(&routerLanePinStore, "store", os.Getenv("SIRSI_ROUTER_STORE"), "postgres:// DSN or SQLite path of the SERVICE's backend (required; default $SIRSI_ROUTER_STORE)")
	routerLanePinSetCmd.Flags().String("by", "operator", "who set the pin (recorded as set_by)")
	routerLanePinSetCmd.Flags().String("reason", "", "free-text reason (recorded; max 512 bytes)")
	routerLanePinListCmd.Flags().Bool("all", false, "include retired pins (history)")
	routerLanePinCmd.AddCommand(routerLanePinSetCmd, routerLanePinRetireCmd, routerLanePinListCmd)
	routerCmd.AddCommand(routerLanePinCmd)
}
