package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/spf13/cobra"
)

var (
	apolloEngine  string
	apolloCores   int
	apolloMemory  int64
	apolloSwap    int64
	apolloEstates string
)

var apolloCmd = &cobra.Command{
	Use:   "apollo",
	Short: "Plan local Apollo inference against this Mac's detected capacity",
	Long: `Apollo is Pantheon's local inference planning surface. It lists the configured
SNE route and detected chip estates, then validates a resource plan without
starting a model or reserving device memory. SNE remains the execution authority.`,
}

var apolloCatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Show this Mac's selectable Apollo engines and chip estates",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate home directory: %w", err)
		}
		catalog, err := apollo.Collect(home)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(catalog)
	},
}

var apolloPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Validate a local Apollo run plan without starting inference",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate home directory: %w", err)
		}
		catalog, err := apollo.Collect(home)
		if err != nil {
			return err
		}
		estates := splitApolloEstates(apolloEstates)
		plan, err := apollo.BuildPlan(catalog, apolloEngine, apolloCores, apolloMemory*1024*1024*1024, apolloSwap*1024*1024*1024, estates)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(plan)
	},
}

func splitApolloEstates(raw string) []string {
	var out []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func init() {
	apolloPlanCmd.Flags().StringVar(&apolloEngine, "engine", "", "Configured Apollo engine id")
	apolloPlanCmd.Flags().IntVar(&apolloCores, "cores", 0, "Requested logical CPU cores")
	apolloPlanCmd.Flags().Int64Var(&apolloMemory, "memory-gib", 0, "Requested unified-memory envelope in GiB")
	apolloPlanCmd.Flags().Int64Var(&apolloSwap, "swap-gib", 0, "Requested swap ceiling in GiB (admission checks live pressure separately)")
	apolloPlanCmd.Flags().StringVar(&apolloEstates, "estates", "", "Comma-separated detected chip estate ids")
	apolloCmd.AddCommand(apolloCatalogCmd, apolloPlanCmd)
}
