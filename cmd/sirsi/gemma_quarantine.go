package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// gemma quarantine / unquarantine — the only sanctioned way to set or clear
// router.QuarantineMarkerPath. Hand-editing the file works too (it's a plain
// stat check), but these verbs exist so quarantining is discoverable and
// never requires knowing the internal path.
var gemmaQuarantineCmd = &cobra.Command{
	Use:   "quarantine",
	Short: "Stop the local SNE broker and tell every restorer to stand down",
	Long: `Stops the broker (if running) and writes the quarantine marker every
gemma-liveness restorer checks BEFORE acting on a down/wedged probe. Without
this, "the broker isn't running" has always meant "restore it" — there was no
way to say "I stopped this on purpose."

  sirsi gemma quarantine
  sirsi gemma unquarantine`,
	RunE: runGemmaQuarantine,
}

var gemmaUnquarantineCmd = &cobra.Command{
	Use:   "unquarantine",
	Short: "Clear the quarantine marker so gemma-liveness resumes restoring the broker",
	RunE:  runGemmaUnquarantine,
}

func init() {
	gemmaCmd.AddCommand(gemmaQuarantineCmd)
	gemmaCmd.AddCommand(gemmaUnquarantineCmd)
}

func runGemmaQuarantine(cmd *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("gemma quarantine: %w", err)
	}
	// Best-effort graceful stop via the same path defaultGemmaServe uses —
	// quarantine is meaningful even if nothing is currently running, so a
	// failed stop here is not fatal to writing the marker.
	exe, exeErr := os.Executable()
	if exeErr != nil || exe == "" {
		exe = "sirsi"
	}
	_ = exec.Command(exe, "gemma", "serve", "--stop").Run()

	path := router.QuarantineMarkerPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("gemma quarantine: %w", err)
	}
	if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600); err != nil {
		return fmt.Errorf("gemma quarantine: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Broker quarantined. gemma-liveness will not restore it until `sirsi gemma unquarantine`.\n")
	return nil
}

func runGemmaUnquarantine(cmd *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("gemma unquarantine: %w", err)
	}
	path := router.QuarantineMarkerPath(home)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("gemma unquarantine: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Quarantine cleared. gemma-liveness will restore the broker on its next tick if it is down.\n")
	return nil
}
