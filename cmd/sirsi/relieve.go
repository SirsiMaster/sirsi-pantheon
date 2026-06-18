package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	relieveConfirm bool
	relieveMinCPU  float64
)

// `sirsi relieve` is the on-demand "Relieve the live cause" action: it finds the
// process currently hogging the CPU and lowers its priority so the foreground app
// regains responsiveness. Dry-run by default (Rule A1); --confirm to apply. Routes
// through the A1-protected reniceByPID, so a critical process is never touched.
var relieveCmd = &cobra.Command{
	Use:   "relieve [process]",
	Short: "Relieve the live cause — lower the priority of the top CPU hog (reversible, nothing killed)",
	Long: `Relieve finds the process currently saturating the CPU — the live cause of a
hang, beachball, or dropped frames — and lowers its scheduler priority so the app
you're actually using gets the cycles back.

  sirsi relieve                 # preview the top live CPU hog (no change)
  sirsi relieve Chrome          # target a specific process by name
  sirsi relieve --confirm       # actually lower its priority

It is gentle and reversible: nothing is killed, the process keeps running (just
yields CPU), and its priority resets when it exits. Critical system processes
(WindowServer, the kernel, audio, the session UI, sirsi itself) are never touched.`,
	Args: cobra.ArbitraryArgs,
	RunE: runRelieve,
}

func init() {
	relieveCmd.Flags().BoolVar(&relieveConfirm, "confirm", false, "Apply the renice (default: preview only)")
	relieveCmd.Flags().Float64Var(&relieveMinCPU, "min-cpu", 15, "Only relieve a process above this %CPU")
}

func runRelieve(cmd *cobra.Command, args []string) error {
	hint := strings.TrimSpace(strings.Join(args, " "))
	res := &output.CommandResult{Command: "sirsi relieve"}

	c, err := guard.FindReliefTarget(hint, relieveMinCPU)
	if err != nil {
		res.Status = "error"
		res.Errors = []string{err.Error()}
		res.Summary = "Couldn't scan for a relief target."
		res.Render()
		return nil
	}
	if c == nil {
		res.Status = "ok"
		if hint != "" {
			res.Summary = fmt.Sprintf("Nothing to relieve — no process matching %q is above %.0f%% CPU right now.", hint, relieveMinCPU)
		} else {
			res.Summary = fmt.Sprintf("Nothing to relieve — no process is above %.0f%% CPU right now (or only protected system processes are busy). The hang isn't live this moment.", relieveMinCPU)
		}
		res.Render()
		return nil
	}

	res.Evidence = []output.Evidence{
		{Label: "Process", Value: fmt.Sprintf("%s (pid %d)", c.Name, c.PID)},
		{Label: "CPU", Value: fmt.Sprintf("%.0f%%", c.CPUPercent)},
	}

	if !relieveConfirm {
		res.Status = "preview"
		res.Summary = fmt.Sprintf("Top live offender: %s (pid %d) at %.0f%% CPU. Lowering its priority hands the CPU back to your foreground app — reversible, nothing killed.", c.Name, c.PID, c.CPUPercent)
		res.NextActions = []output.NextAction{{
			Label:       "Relieve it now",
			Command:     fmt.Sprintf("relieve %q --confirm", c.Name),
			Description: "renice +10 + background QoS — reversible, resets when the process exits",
		}}
		res.Render()
		return nil
	}

	if err := guard.Relieve(c); err != nil {
		res.Status = "error"
		res.Errors = []string{err.Error()}
		res.Summary = fmt.Sprintf("Couldn't relieve %s (pid %d): %v. (Processes you don't own need elevated privileges; critical ones are refused by design.)", c.Name, c.PID, err)
		res.Render()
		return nil
	}
	res.Status = "ok"
	res.Summary = fmt.Sprintf("Relieved %s (pid %d) — lowered to background priority. Your foreground app should regain responsiveness; it resets when %s exits.", c.Name, c.PID, c.Name)
	res.Render()
	return nil
}
