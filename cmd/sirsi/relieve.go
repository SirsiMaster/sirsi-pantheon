package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	relieveConfirm bool
	relieveMinCPU  float64
	relieveMemory  bool
)

// gib is 1 GiB, for byte→GB display math.
const gib = float64(1 << 30)

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
	relieveCmd.Flags().BoolVar(&relieveMemory, "memory", false, "Relieve MEMORY pressure by flushing inactive caches (needs admin) instead of the CPU renice")
}

func runRelieve(cmd *cobra.Command, args []string) error {
	if relieveMemory {
		return runRelieveMemory()
	}
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

// runRelieveMemory relieves MEMORY pressure the only safe, non-destructive way
// macOS offers: flushing the OS's inactive/cached pages with `purge`. Renice
// frees CPU, not RAM, and auto-killing a user's app is not safe to automate — so
// this is the honest memory lever. It measures free memory + pressure before and
// after so the result is PROOF, not a claim. `purge` needs root, so --confirm
// runs it through macOS's native admin-auth dialog: the password goes to the OS,
// never to sirsi.
func runRelieveMemory() error {
	res := &output.CommandResult{Command: "sirsi relieve --memory"}
	before := guard.SampleNodeCapacity()
	freeBefore := float64(before.FreeRAM) / gib

	// Name the actual cause — the top memory consumer — so the user knows WHAT is
	// using the RAM, not just that it's tight. This is the "identify" half the app
	// was skipping.
	hog := ""
	if s, err := guard.SampleMemory(); err == nil && len(s.Top) > 0 {
		name := s.Top[0].Name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:] // basename — "codex", not the full bundle path
		}
		hog = fmt.Sprintf("%s (%.1f GB)", name, float64(s.Top[0].RSS)/gib)
	}

	if !relieveConfirm {
		res.Status = "preview"
		if hog != "" {
			res.Evidence = append(res.Evidence, output.Evidence{Label: "Top memory user", Value: hog})
		}
		res.Evidence = append(res.Evidence,
			output.Evidence{Label: "Free memory", Value: fmt.Sprintf("%.1f GB", freeBefore)},
			output.Evidence{Label: "Pressure", Value: fmt.Sprintf("%s", before.Pressure)},
		)
		guidance := ""
		if hog != "" {
			guidance = fmt.Sprintf(" To free more, quit %s.", hog)
		}
		res.Summary = fmt.Sprintf("Flush inactive caches to relieve memory pressure. This runs `purge` (it asks for your admin password once) — it returns cached pages to the free pool WITHOUT touching any app, and macOS re-caches as it needs to.%s", guidance)
		res.NextActions = []output.NextAction{{
			Label:       "Flush caches now",
			Command:     "relieve --memory --confirm",
			Description: "runs `purge` via the macOS admin prompt — frees inactive memory, nothing killed",
		}}
		res.Render()
		return nil
	}

	// Apply: purge via macOS admin authorization. osascript shows the native
	// password dialog; the credential is handled by the OS, never seen by sirsi.
	script := `do shell script "/usr/sbin/purge" with administrator privileges`
	if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		res.Status = "error"
		res.Errors = []string{strings.TrimSpace(string(out))}
		res.Summary = "Couldn't flush caches — the admin authorization was declined or failed, so nothing changed."
		res.Render()
		return nil
	}

	after := guard.SampleNodeCapacity()
	freeAfter := float64(after.FreeRAM) / gib
	res.Status = "ok"
	res.Evidence = []output.Evidence{
		{Label: "Free before", Value: fmt.Sprintf("%.1f GB", freeBefore)},
		{Label: "Free after", Value: fmt.Sprintf("%.1f GB", freeAfter)},
		{Label: "Pressure", Value: fmt.Sprintf("%s → %s", before.Pressure, after.Pressure)},
	}
	res.Summary = fmt.Sprintf("Flushed inactive caches. Free memory: %.1f GB → %.1f GB · pressure now %s. (macOS re-caches as it needs to, so free may settle back — the pressure relief is the point.)", freeBefore, freeAfter, after.Pressure)
	res.Render()
	return nil
}
