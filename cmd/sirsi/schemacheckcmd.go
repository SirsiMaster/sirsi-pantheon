package main

// schemacheckcmd.go — install.sh plumbing: exit 0 if this binary can open
// the live router store, non-zero if not.
//
// Called by scripts/install.sh before replacing the live binary:
//
//	${TMPDIR}/sirsi schema-check [--db PATH]
//
// The command is hidden from `sirsi help` — it is not a user-facing feature.

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"github.com/spf13/cobra"
)

var schemaCheckDBFlag string

var schemaCheckCmd = &cobra.Command{
	Use:    "schema-check",
	Short:  "Exit 0 if this binary can open the live router store, non-zero if not",
	Hidden: true, // install.sh plumbing, not a user-facing command
	RunE:   runSchemaCheck,
}

// runSchemaCheck is the gate: the new binary (running from a temp dir) checks
// whether its MaxSupportedSchemaVersion covers the live store's user_version.
// Absence → pass (no live schema to protect). Unreadable → fail closed.
func runSchemaCheck(_ *cobra.Command, _ []string) error {
	dbPath := schemaCheckDBFlag
	if dbPath == "" {
		var err error
		dbPath, err = routerstore.LocalPath()
		if err != nil {
			return err
		}
	}
	liveSchema, err := resolveLiveSchema(dbPath)
	if err != nil {
		return fmt.Errorf("schema-check: %w", err)
	}
	info := modversion.Info{RouterSchemaMax: routerstore.MaxSupportedSchemaVersion()}
	return selfupdate.CheckSchemaCeiling(info, liveSchema)
}

func init() {
	schemaCheckCmd.Flags().StringVar(&schemaCheckDBFlag, "db", "", "local router store path (default: $SIRSI_ROUTER_DB or ~/.sirsi/router.db; refused when SIRSI_ROUTER_URL is set)")
	rootCmd.AddCommand(schemaCheckCmd)
}
