package main

// `sirsi maat capacity get|set` — the fleet's per-machine core capacity
// (internal/maat/schedule/capacity.go), which Reserve/Extend divide by
// schedule.FairShareLanes to compute the floor share every lane is
// guaranteed (owner directive 2026-09-26: never a lockout).

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var maatCapacityCmd = &cobra.Command{
	Use:   "capacity",
	Short: "Get or set a resource's core capacity (drives the floor share)",
}

var maatCapacityGetCmd = &cobra.Command{
	Use:   "get <resource>",
	Short: "Show a resource's configured capacity and floor share",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		cores, err := l.Capacity(args[0])
		if err != nil {
			return err
		}
		floor, err := l.FloorShare(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(map[string]any{"resource": args[0], "cores": cores, "floor": floor})
		}
		fmt.Printf("𓆄 %s: %d cores, floor share %d\n", args[0], cores, floor)
		return nil
	},
}

var maatCapacitySetCmd = &cobra.Command{
	Use:   "set <resource> <cores>",
	Short: "Set a resource's core capacity",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cores, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("maat: cores %q is not an integer", args[1])
		}
		l, err := maatLedger()
		if err != nil {
			return err
		}
		if setErr := l.SetCapacity(args[0], cores); setErr != nil {
			return setErr
		}
		floor, err := l.FloorShare(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(map[string]any{"resource": args[0], "cores": cores, "floor": floor})
		}
		fmt.Printf("𓆄 %s: capacity set to %d cores (floor share %d)\n", args[0], cores, floor)
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{maatCapacityGetCmd, maatCapacitySetCmd} {
		c.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	}
	maatCapacityCmd.AddCommand(maatCapacityGetCmd, maatCapacitySetCmd)
	maatCmd.AddCommand(maatCapacityCmd)
}
