//go:build darwin

package main

import (
	"encoding/json"
	"errors"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

type routerMCPConfigMigrator interface {
	Plan(setup.RouterMCPSelection) (setup.RouterMCPMigrationPlan, error)
	Apply(setup.RouterMCPSelection, string) (setup.RouterMCPMigrationResult, error)
}

func newSetupRouterMCPCmd(factory func() (routerMCPConfigMigrator, error)) *cobra.Command {
	root := &cobra.Command{Use: "router-mcp", Short: "Preview or confirm one exact legacy router MCP configuration migration", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {}}
	for _, action := range []string{"plan", "apply"} {
		selection := setup.RouterMCPSelection{}
		var confirmed string
		cmd := &cobra.Command{Use: action, Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
			RunE: func(cmd *cobra.Command, _ []string) error {
				m, err := factory()
				if err != nil {
					return err
				}
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if action == "plan" {
					p, err := m.Plan(selection)
					if err != nil {
						return err
					}
					return encoder.Encode(p)
				}
				result, applyErr := m.Apply(selection, confirmed)
				if result.Status != "" {
					return errors.Join(applyErr, encoder.Encode(result))
				}
				return applyErr
			}}
		cmd.Flags().StringVar(&selection.Client, "client", "", "Exact client: claude-code or cursor")
		cmd.Flags().StringVar(&selection.Config, "config", "", "Explicit existing canonical owner-local client config")
		cmd.Flags().StringVar(&selection.Server, "server", "", "Exact existing mcpServers key")
		cmd.Flags().StringVar(&selection.Project, "project", "", "Exact Claude projects-map key; omit for top-level scope")
		for _, flag := range []string{"client", "config", "server"} {
			_ = cmd.MarkFlagRequired(flag)
		}
		if action == "apply" {
			cmd.Flags().StringVar(&confirmed, "confirm", "", "Exact digest from a freshly reviewed plan")
			_ = cmd.MarkFlagRequired("confirm")
		}
		root.AddCommand(cmd)
	}
	return root
}

func init() {
	setupCmd.AddCommand(newSetupRouterMCPCmd(func() (routerMCPConfigMigrator, error) { return setup.NewRouterMCPMigration() }))
}
