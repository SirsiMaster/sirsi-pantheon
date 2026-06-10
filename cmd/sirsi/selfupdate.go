package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"github.com/spf13/cobra"
)

var (
	selfUpdateConfirm bool
	selfUpdateJSON    bool
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Heal binary-drift: re-sign fresh sirsi over stale CLI copies (AMFI-safe)",
	Long: `Self-Update — heal sirsi binary-drift on this host.

A stale CLI copy of sirsi (left by a plain ` + "`cp`" + ` over an existing binary)
carries a code-signing cdhash bound to the old inode. On macOS the next exec is
SIGKILL'd (137) by AMFI — which is why sirsi has been its own top crash source.

This command finds CLI copies whose content differs from the binary you're
running now and replaces them with it using the AMFI-safe contract (stage a
fresh inode, codesign it, atomic rename over the old one).

  sirsi self-update            # preview drift (read-only, default)
  sirsi self-update --confirm  # heal the drifted copies (asks [y/N])
  sirsi self-update --json     # machine-readable drift report (read-only)

SAFETY: writes ONLY to the known CLI bin dirs (~/.local/bin, ~/go/bin,
/opt/homebrew/bin, /usr/local/bin) and NEVER inside a .app bundle (Rule A19).
The rewrite is destructive and always confirmed — there is no auto-apply flag.`,
	RunE: runSelfUpdate,
}

func runSelfUpdate(_ *cobra.Command, _ []string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running binary: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(self); rerr == nil {
		self = resolved
	}
	selfHash, err := selfupdate.FileHash(self)
	if err != nil {
		return fmt.Errorf("hash running binary: %w", err)
	}

	targets := selfupdate.DetectCLIDrift(self, selfHash)

	// --json is a pure read-only detect — safe for non-interactive callers
	// (CI, hooks, the menubar tick). It never mutates (gate #1).
	if selfUpdateJSON {
		out := map[string]any{
			"running": self,
			"hash":    selfHash,
			"drift":   targets,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	ver := modversion.Current("sirsi").Version
	if len(targets) == 0 {
		output.Success("All CLI copies match the running binary (%s) — nothing to heal.", ver)
		return nil
	}

	// Preview (read-only) — render the drift and the proposed action before any
	// write (gate #1).
	output.Header("Binary drift detected")
	for _, t := range targets {
		output.Warn("  %s", t.Path)
		output.Dim("     present %s → will be replaced with running %s (%s)", shortHash(t.Present), shortHash(t.Expected), ver)
	}

	if !selfUpdateConfirm {
		output.Dim("\n  Preview only. Re-run with --confirm to heal (AMFI-safe: fresh inode + codesign + atomic rename).")
		return nil
	}

	// Apply — explicit --confirm plus an interactive [y/N]. A binary rewrite is
	// destructive; there is deliberately no --yes auto-confirm for it (gate #2).
	if !confirmFix(fmt.Sprintf("Replace %d stale CLI copy(ies) with the running binary?", len(targets))) {
		output.Dim("     → aborted; no changes made")
		return nil
	}

	healed, failed := 0, 0
	for _, t := range targets {
		if err := selfupdate.SafeReplace(self, t.Path); err != nil {
			output.Error("     %s: %v", t.Path, err)
			failed++
			continue
		}
		// Verify-after-convergence — re-hash and prove it matches; don't trust
		// the write (gate #3).
		post, herr := selfupdate.FileHash(t.Path)
		if herr != nil || post != selfHash {
			output.Error("     %s: replaced but did not converge (present %s, want %s)", t.Path, shortHash(post), shortHash(selfHash))
			failed++
			continue
		}
		output.Success("     healed %s (re-signed, converged)", t.Path)
		healed++
	}

	// Running-process restart note — atomic rename means a process already on
	// the old inode keeps it until it restarts (gate #4).
	if healed > 0 {
		output.Dim("\n  Note: any process already running an old copy keeps it until it RESTARTS")
		output.Dim("  (restart the menubar/daemon, and open a new shell to pick up the healed CLI).")
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d copies failed to heal", failed, len(targets))
	}
	output.Success("Healed %d CLI copy(ies).", healed)
	return nil
}

// shortHash truncates a hex digest for display.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func init() {
	selfUpdateCmd.Flags().BoolVar(&selfUpdateConfirm, "confirm", false, "apply the heal (replace stale copies); default is preview only")
	selfUpdateCmd.Flags().BoolVar(&selfUpdateJSON, "json", false, "machine-readable drift report (read-only, never mutates)")
}
