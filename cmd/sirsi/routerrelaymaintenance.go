package main

import (
	"encoding/json"
	"errors"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

func init() {
	routerRelayCmd.AddCommand(&cobra.Command{
		Use: "maintenance-capabilities", Short: "Read the signed engine's root startup-maintenance implementation identity without starting a relay",
		Args:             cobra.NoArgs,
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(routerstore.RootMaintenanceCapability())
		},
	})
	var path string
	var pid int
	image := &cobra.Command{Use: "maintenance-image", Short: "Bounded read-only root maintenance code-identity probe", Args: cobra.NoArgs, PersistentPreRun: func(*cobra.Command, []string) {}, RunE: func(cmd *cobra.Command, _ []string) error {
		var identity setup.MaintenanceImageIdentity
		var err error
		if path == "" {
			if !cmd.Flags().Changed("pid") {
				return errors.New("legacy maintenance: --pid is required when --payload is omitted")
			}
			identity, err = setup.InspectRootLoadedMaintenanceImage(pid)
		} else {
			identity, err = setup.InspectRootMaintenanceImage(path, pid)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(identity)
	}}
	image.Flags().StringVar(&path, "payload", "", "Existing protected signed payload to compare; never executed (omit to read only the kernel PID hash)")
	image.Flags().IntVar(&pid, "pid", 0, "Kernel PID to bind (required without --payload), or zero for static replacement signature")
	routerRelayCmd.AddCommand(image)
}
