//go:build darwin

package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"runtime/debug"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

//go:embed launchdmaintenance_app_darwin.go launchdmaintenance.go launchdmaintenance_app_capability_darwin.go
var legacyAppCommandSource embed.FS

func legacyAppCapability() setup.LegacyAppCapability {
	h := sha256.New()
	for _, name := range []string{"launchdmaintenance_app_darwin.go", "launchdmaintenance.go", "launchdmaintenance_app_capability_darwin.go"} {
		data, _ := legacyAppCommandSource.ReadFile(name)
		_, _ = h.Write([]byte(name + "\x00"))
		_, _ = h.Write(data)
	}
	_, _ = h.Write([]byte(setup.LegacyAppImplementation()))
	native := false
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "CGO_ENABLED" && setting.Value == "1" {
				native = true
			}
		}
	}
	return setup.LegacyAppCapability{Schema: 1, Protocol: "root-relay-local-app-v1", Release: version, Implementation: hex.EncodeToString(h.Sum(nil)), Native: native}
}
func init() {
	launchdCmd.AddCommand(&cobra.Command{Use: "legacy-maintenance-app-capabilities", Args: cobra.NoArgs,
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(legacyAppCapability())
		}})
}
