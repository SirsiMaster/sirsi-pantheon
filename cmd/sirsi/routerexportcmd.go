package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var routerExportDir string

// routerExportCmd is the bulk export contract (docs/router/BULK_EXPORT_CONTRACT.md): bounded pages,
// rendered on this Mac, never a snapshot.
var routerExportCmd = &cobra.Command{
	Use:   "export --dir <path>",
	Args:  cobra.NoArgs,
	Short: "Write every router item as <id>.md into a local directory (paged; not a point-in-time snapshot)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if routerExportDir == "" {
			return errors.New("--dir is required")
		}
		f, err := openTaskFacade()
		if err != nil {
			return err
		}
		defer f.Close()
		r, err := routerstore.ExportMarkdownPaged(context.Background(), f.Store(), routerExportDir, routerstore.ListPageMax)
		_ = json.NewEncoder(os.Stdout).Encode(r)
		return err
	},
}

func init() {
	routerExportCmd.Flags().StringVar(&routerExportDir, "dir", "", "Local directory to write <id>.md files into (required)")
	routerCmd.AddCommand(routerExportCmd)
}
