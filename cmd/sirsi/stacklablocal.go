package main

import (
	"encoding/json"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stacklab"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

func newStacklabLocalCmd(home func() (string, error), recipes func() (string, error)) *cobra.Command {
	parent := &cobra.Command{Use: "local", Short: "Local Stack Lab opt-in and packaged recipes; no provider invocation"}
	parent.PersistentFlags().Bool("json", false, "Output local typed state")
	for _, action := range []string{"inspect", "enable", "disable", "catalog"} {
		cmd := &cobra.Command{Use: action, Args: cobra.NoArgs}
		cmd.RunE = func(cmd *cobra.Command, _ []string) error {
			h, err := home()
			if err != nil {
				return err
			}
			state, err := stacklab.InspectLocal(h)
			if err != nil {
				return err
			}
			var result any = state
			switch cmd.Name() {
			case "enable":
				p, _ := cmd.Flags().GetString("provider")
				state, err = stacklab.SetLocal(h, true, p)
				result = state
			case "disable":
				state, err = stacklab.SetLocal(h, false, state.Provider)
				result = state
			case "catalog":
				if !state.Enabled {
					return fmt.Errorf("local Stack Lab is disabled; enable it in the Stack Lab workspace")
				}
				var path string
				path, err = recipes()
				if err == nil {
					result, err = stacklab.LoadPackagedCatalog(path)
				}
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		if action == "enable" {
			cmd.Flags().String("provider", "local", "Provider kind: local, frontier, self-hosted; stores metadata only")
		}
		parent.AddCommand(cmd)
	}
	return parent
}

func installedStacklabRecipes() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// The normal app ships these same contracts; no checkout/PATH search.
	dir := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "Resources", "StackLab"))
	if filepath.Base(filepath.Dir(exe)) == "MacOS" && filepath.Base(filepath.Dir(filepath.Dir(exe))) == "Contents" {
		return dir, nil
	}
	if exe == "/usr/local/libexec/sirsi-pantheon/sirsi" {
		return "/Applications/Pantheon.app/Contents/Resources/StackLab", nil
	}
	return "", fmt.Errorf("installed Stack Lab recipe payload unavailable; open the normal release workspace to repair this installation")
}

func init() { stacklabCmd.AddCommand(newStacklabLocalCmd(os.UserHomeDir, installedStacklabRecipes)) }
