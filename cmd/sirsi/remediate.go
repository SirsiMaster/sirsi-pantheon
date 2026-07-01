package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

// adminShell runs a shell command with macOS admin privileges via the OS's OWN
// authorization dialog (osascript `with administrator privileges`). The password
// is typed into macOS's dialog and handled entirely by the OS — sirsi never sees,
// stores, or forwards it (Rule A11). This is the shared gate for every remediation
// lever that needs root: purge, tmutil thinning, mdutil, DNS flush, font caches.
func adminShell(prompt, shellCmd string) ([]byte, error) {
	esc := strings.ReplaceAll(shellCmd, `"`, `\"`)
	script := fmt.Sprintf(`do shell script "%s" with prompt "%s" with administrator privileges`, esc, prompt)
	return exec.Command("osascript", "-e", script).CombinedOutput()
}

// diskFreeBytes returns the free bytes on the volume containing path — used to
// PROVE how much a reclaim actually freed (before/after), not just claim it.
func diskFreeBytes(path string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

var remediateConfirm bool

// ── reclaim-snapshots ─────────────────────────────────────────────────────────

var reclaimSnapshotsCmd = &cobra.Command{
	Use:   "reclaim-snapshots",
	Short: "Reclaim disk from local Time Machine (APFS) snapshots — thin them, free space",
	Long: `Local Time Machine snapshots quietly hold disk space on your startup volume.
They're useful, but when you're low on space they're the biggest painless win.

  sirsi reclaim-snapshots           # preview: how many snapshots, disk free now
  sirsi reclaim-snapshots --confirm # thin them (asks for your admin password once)

Thinning runs ` + "`tmutil thinlocalsnapshots / … 4`" + ` (macOS's own tool, most
aggressive urgency). It removes LOCAL snapshots only — your real Time Machine
backups on an external/network drive are untouched. Reversible in the sense that
macOS re-creates local snapshots automatically later.`,
	RunE: runReclaimSnapshots,
}

func runReclaimSnapshots(_ *cobra.Command, _ []string) error {
	res := &output.CommandResult{Command: "sirsi reclaim-snapshots"}

	snaps := guard.LocalSnapshots("/")
	if len(snaps) == 0 {
		res.Status = "ok"
		res.Summary = "No local snapshots to reclaim — nothing to do."
		res.Render()
		return nil
	}
	freeBefore := diskFreeBytes("/")

	if !remediateConfirm {
		res.Status = "preview"
		res.Evidence = []output.Evidence{
			{Label: "Local snapshots", Value: fmt.Sprintf("%d", len(snaps))},
			{Label: "Disk free now", Value: humanGB(freeBefore)},
		}
		res.Summary = fmt.Sprintf("%d local Time Machine snapshot(s) on this disk. Thinning them frees space (your real external/network backups are untouched). Runs `tmutil thinlocalsnapshots` — asks for your admin password once.", len(snaps))
		res.NextActions = []output.NextAction{{
			Label:       "Reclaim snapshot space now",
			Command:     "reclaim-snapshots --confirm",
			Description: "thins local APFS snapshots via the macOS admin prompt — external backups untouched",
		}}
		res.Render()
		return nil
	}

	// Thin up to 64 GB at the most aggressive urgency (4). macOS frees what it can.
	out, err := adminShell("Sirsi needs admin to thin local Time Machine snapshots",
		"/usr/bin/tmutil thinlocalsnapshots / 68719476736 4")
	if err != nil {
		res.Status = "error"
		res.Errors = []string{strings.TrimSpace(string(out))}
		res.Summary = "Couldn't thin snapshots — admin authorization was declined or failed, so nothing changed."
		res.Render()
		return nil
	}
	freeAfter := diskFreeBytes("/")
	reclaimed := freeAfter - freeBefore
	if reclaimed < 0 {
		reclaimed = 0
	}
	res.Status = "ok"
	res.Evidence = []output.Evidence{
		{Label: "Disk free before", Value: humanGB(freeBefore)},
		{Label: "Disk free after", Value: humanGB(freeAfter)},
		{Label: "Snapshots remaining", Value: fmt.Sprintf("%d", len(guard.LocalSnapshots("/")))},
	}
	res.Summary = fmt.Sprintf("Thinned local snapshots — reclaimed ~%s. Disk free: %s → %s. Your external/network Time Machine backups are untouched.", humanGB(reclaimed), humanGB(freeBefore), humanGB(freeAfter))
	res.Render()
	return nil
}

func humanGB(b int64) string { return fmt.Sprintf("%.1f GB", float64(b)/(1<<30)) }

func init() {
	reclaimSnapshotsCmd.Flags().BoolVar(&remediateConfirm, "confirm", false, "Apply the reclaim (default: preview only)")
	rootCmd.AddCommand(reclaimSnapshotsCmd)
}
