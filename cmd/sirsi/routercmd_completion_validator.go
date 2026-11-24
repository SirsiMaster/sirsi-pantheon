package main

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/completionproof"
	"github.com/spf13/cobra"
)

func newCompletionValidatorCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "completion-validator", Short: "Identify or explicitly export this binary's completion policy",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), completionproof.ValidatorSHA256())
			return err
		},
	}
	command.AddCommand(&cobra.Command{
		Use: "export <absent-file>", Short: "Write identical standalone Python source; never install or overwrite",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := completionproof.ExportValidator(args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), completionproof.ValidatorSHA256())
			return err
		},
	})
	return command
}

func init() { routerCmd.AddCommand(newCompletionValidatorCommand()) }
