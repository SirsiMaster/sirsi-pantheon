package main

// guardundo.go — `sirsi guard undo <pid>`: reverse one Isis auto-renice
// (nice back to 0, out of Background QoS). ADR-064.

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

var guardUndoCmd = &cobra.Command{
	Use:   "undo <pid>",
	Short: "Reverse an Isis auto-renice: nice 0 and out of Background QoS",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("pid must be an integer: %q", args[0])
		}
		if err := guard.UndoRenice(pid); err != nil {
			return err
		}
		fmt.Printf("𓁵 Isis: PID %d restored (nice 0, foreground QoS)\n", pid)
		return nil
	},
}

func init() {
	guardCmd.AddCommand(guardUndoCmd)
}
