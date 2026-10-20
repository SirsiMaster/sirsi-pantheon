package main

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

func init() {
	var evidence string
	var generation uint64
	cmd := &cobra.Command{Use: "provider-available <agent>", Short: "Record verified provider availability after a local quota hold", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := router.RecordProviderAvailability(args[0], evidence, generation); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "provider availability recorded; prior failure history retained")
		return nil
	}}
	cmd.Flags().StringVar(&evidence, "evidence", "", "Nonsecret reference proving legitimate provider availability change (required)")
	cmd.Flags().Uint64Var(&generation, "generation", 0, "Exact currently inspected quota failure generation (required)")
	routerCmd.AddCommand(cmd)
}
