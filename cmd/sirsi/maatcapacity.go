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
	Short: "Get or set a resource's core and memory capacity (drives floor shares)",
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
		memoryGiB, err := l.MemoryCapacityGB(args[0])
		if err != nil {
			return err
		}
		memoryFloorGiB, err := l.FloorShareMemGB(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(map[string]any{"resource": args[0], "cores": cores, "floor": floor, "core_floor": floor, "memory_gib": memoryGiB, "memory_floor_gib": memoryFloorGiB})
		}
		if memoryGiB == 0 {
			fmt.Printf("𓆄 %s: %d cores (floor share %d); memory unconfigured (memory floor share %d GiB)\n", args[0], cores, floor, memoryFloorGiB)
			return nil
		}
		fmt.Printf("𓆄 %s: %d cores (floor share %d); %d GiB memory (floor share %d GiB)\n", args[0], cores, floor, memoryGiB, memoryFloorGiB)
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
		memoryGiB, err := l.MemoryCapacityGB(args[0])
		if err != nil {
			return err
		}
		memoryFloorGiB, err := l.FloorShareMemGB(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(map[string]any{"resource": args[0], "cores": cores, "floor": floor, "core_floor": floor, "memory_gib": memoryGiB, "memory_floor_gib": memoryFloorGiB})
		}
		fmt.Printf("𓆄 %s: core capacity set to %d (floor share %d); memory %s\n", args[0], cores, floor, memoryCapacityLabel(memoryGiB, memoryFloorGiB))
		return nil
	},
}

var maatCapacityMemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Get or set a resource's explicit RAM capacity in GiB",
}

var maatCapacityMemoryGetCmd = &cobra.Command{
	Use:   "get <resource>",
	Short: "Show a resource's configured RAM capacity and floor share",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		l, err := maatLedger()
		if err != nil {
			return err
		}
		memoryGiB, err := l.MemoryCapacityGB(args[0])
		if err != nil {
			return err
		}
		floorGiB, err := l.FloorShareMemGB(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(map[string]any{"resource": args[0], "memory_gib": memoryGiB, "memory_floor_gib": floorGiB})
		}
		fmt.Printf("𓆄 %s: memory %s\n", args[0], memoryCapacityLabel(memoryGiB, floorGiB))
		return nil
	},
}

var maatCapacityMemorySetCmd = &cobra.Command{
	Use:   "set <resource> <gib>",
	Short: "Set a resource's explicit RAM capacity in GiB",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		memoryGiB, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("maat: memory GiB %q is not an integer", args[1])
		}
		l, err := maatLedger()
		if err != nil {
			return err
		}
		if setErr := l.SetCapacityMemGB(args[0], memoryGiB); setErr != nil {
			return setErr
		}
		floorGiB, err := l.FloorShareMemGB(args[0])
		if err != nil {
			return err
		}
		if maatJSON {
			return emitJSON(map[string]any{"resource": args[0], "memory_gib": memoryGiB, "memory_floor_gib": floorGiB})
		}
		fmt.Printf("𓆄 %s: memory capacity set to %d GiB (floor share %d GiB)\n", args[0], memoryGiB, floorGiB)
		return nil
	},
}

func memoryCapacityLabel(memoryGiB, floorGiB int) string {
	if memoryGiB == 0 {
		return fmt.Sprintf("unconfigured (floor share %d GiB)", floorGiB)
	}
	return fmt.Sprintf("%d GiB (floor share %d GiB)", memoryGiB, floorGiB)
}

func init() {
	for _, c := range []*cobra.Command{maatCapacityGetCmd, maatCapacitySetCmd, maatCapacityMemoryGetCmd, maatCapacityMemorySetCmd} {
		c.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	}
	maatCapacityMemoryCmd.AddCommand(maatCapacityMemoryGetCmd, maatCapacityMemorySetCmd)
	maatCapacityCmd.AddCommand(maatCapacityGetCmd, maatCapacitySetCmd)
	maatCapacityCmd.AddCommand(maatCapacityMemoryCmd)
	maatCmd.AddCommand(maatCapacityCmd)
}
