package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

// `sirsi hapi` is the live MEMORY governor — the "protect" pillar (ADR-031-A
// Layer 4). It exists because on 2026-06-18 Pantheon's own broker OOM-froze a
// 48 GB host and nothing was watching system memory. Hapi watches free RAM +
// per-process RSS and, BEFORE the kernel Jetsams, suspends (reversibly) and then
// kills GOVERNED compute — while only WARNING about processes it wasn't asked to
// govern. Read-only `status`; `watch` runs the loop; `protect`/`release` manage
// which PIDs Hapi may act on with teeth.

var (
	hapiWatchGovern   bool
	hapiWatchInterval int
)

var hapiCmd = &cobra.Command{
	Use:   "hapi",
	Short: "𓁢 Hapi — the live memory governor: watch free RAM and stop a runaway before the kernel Jetsams",
	Long: `Hapi is the memory counterpart to the Isis CPU watchdog. It samples system
free RAM + per-process resident memory and intervenes before macOS kills things
for you — the layer that was missing when Pantheon's broker froze the host on
2026-06-18.

  sirsi hapi                 # status: free RAM, pressure tier, the top runaway
  sirsi hapi watch           # live loop: warn as memory tightens (no auto-kill)
  sirsi hapi watch --govern  # also suspend/kill GOVERNED compute under pressure
  sirsi hapi protect <pid>   # mark a PID as governed (Hapi may act on it)
  sirsi hapi release <pid>   # stop governing a PID

Safety (A1): Hapi only uses teeth (suspend → kill) on processes that registered
as governed. Everything else it only names and recommends. WindowServer, the
kernel, audio, the session UI, sirsi itself, and live Claude/Codex agents are
never touched. Suspend (SIGSTOP) is reversible — Hapi resumes what it paused once
pressure clears.`,
	RunE: runHapiStatus,
}

var hapiWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run the memory governor live until Ctrl-C",
	RunE:  runHapiWatch,
}

var hapiProtectCmd = &cobra.Command{
	Use:   "protect <pid> [name]",
	Short: "Register a PID as Pantheon-governed compute (Hapi may suspend/kill it under pressure)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runHapiProtect,
}

var hapiReleaseCmd = &cobra.Command{
	Use:   "release <pid>",
	Short: "Stop governing a PID",
	Args:  cobra.ExactArgs(1),
	RunE:  runHapiRelease,
}

func init() {
	hapiWatchCmd.Flags().BoolVar(&hapiWatchGovern, "govern", false, "Act with teeth (suspend→kill) on governed compute; default is warn-only")
	hapiWatchCmd.Flags().IntVar(&hapiWatchInterval, "interval", 3, "Sampling interval in seconds")
	hapiCmd.AddCommand(hapiWatchCmd, hapiProtectCmd, hapiReleaseCmd)
}

// tierGlyph/tierWord give the status line a health color the way the menubar Eye
// tints itself — green when there's headroom, red at emergency.
func tierLabel(t guard.MemTier) string {
	switch t {
	case guard.TierWarn:
		return "🟡 tight"
	case guard.TierCritical:
		return "🟠 critical"
	case guard.TierEmergency:
		return "🔴 emergency"
	default:
		return "🟢 healthy"
	}
}

func runHapiStatus(cmd *cobra.Command, args []string) error {
	res := &output.CommandResult{Command: "sirsi hapi"}
	g := guard.DefaultMemGovernor()

	s, err := guard.SampleMemory()
	if err != nil {
		res.Status = "error"
		res.Errors = []string{err.Error()}
		res.Summary = "Couldn't sample system memory."
		res.Render()
		return nil
	}
	tier := guard.ClassifyMem(s, g.WarnPercent, g.CriticalPercent, g.EmergencyPercent)
	runaway := guard.FindRunaway(s)

	res.Status = "ok"
	res.Evidence = []output.Evidence{
		{Label: "Free RAM", Value: fmt.Sprintf("%s of %s (%.1f%%)", seba.FormatBytes(s.FreeBytes), seba.FormatBytes(s.TotalRAM), s.FreePercent)},
		{Label: "Pressure", Value: tierLabel(tier)},
	}
	if runaway != nil {
		res.Evidence = append(res.Evidence, output.Evidence{
			Label: "Top runaway",
			Value: fmt.Sprintf("%s (pid %d) — %s resident", runaway.Name, runaway.PID, seba.FormatBytes(runaway.RSS)),
		})
	}

	switch tier {
	case guard.TierOK:
		res.Summary = fmt.Sprintf("Memory healthy — %s free (%.0f%%). Nothing is pressuring the host.", seba.FormatBytes(s.FreeBytes), s.FreePercent)
	default:
		who := "a process"
		if runaway != nil {
			who = fmt.Sprintf("%s (pid %d, %s)", runaway.Name, runaway.PID, seba.FormatBytes(runaway.RSS))
		}
		res.Summary = fmt.Sprintf("Memory %s — only %s free (%.0f%%). Largest non-protected process is %s. Run `sirsi hapi watch --govern` to let Hapi stop a governed runaway before the kernel does.", tierLabel(tier), seba.FormatBytes(s.FreeBytes), s.FreePercent, who)
		if runaway != nil {
			res.NextActions = []output.NextAction{{
				Label:       "Watch and govern",
				Command:     "hapi watch --govern",
				Description: "live loop: suspend (reversible) then kill governed compute before Jetsam; warn on the rest",
			}}
		}
	}
	res.Render()
	return nil
}

func runHapiWatch(cmd *cobra.Command, args []string) error {
	g := guard.DefaultMemGovernor()
	g.Govern = hapiWatchGovern
	if hapiWatchInterval > 0 {
		g.Interval = time.Duration(hapiWatchInterval) * time.Second
	}

	mode := "warn-only"
	if g.Govern {
		mode = "GOVERNING (suspend→kill governed compute under pressure)"
	}
	fmt.Fprintf(os.Stderr, "𓁢 Hapi watching memory every %s · %s · Ctrl-C to stop\n", g.Interval, mode)

	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; close(stop) }()

	var lastTier guard.MemTier = -1
	g.Start(stop, func(r guard.GovernResult) {
		// Print every intervention; otherwise only print when the tier changes so
		// the loop is quiet when healthy (no idle churn in the user's terminal).
		for _, a := range r.Actions {
			fmt.Printf("  ⚡ %s\n", a)
		}
		if r.Tier != lastTier {
			lastTier = r.Tier
			who := ""
			if r.Runaway != nil {
				who = fmt.Sprintf(" · top: %s (pid %d, %s)", r.Runaway.Name, r.Runaway.PID, seba.FormatBytes(r.Runaway.RSS))
			}
			fmt.Printf("%s · %s free (%.0f%%)%s\n", tierLabel(r.Tier), seba.FormatBytes(r.Sample.FreeBytes), r.Sample.FreePercent, who)
		}
	})
	fmt.Fprintln(os.Stderr, "𓁢 Hapi stopped — resumed anything it had paused.")
	return nil
}

func runHapiProtect(cmd *cobra.Command, args []string) error {
	res := &output.CommandResult{Command: "sirsi hapi protect"}
	pid, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil {
		res.Status = "error"
		res.Errors = []string{fmt.Sprintf("not a pid: %q", args[0])}
		res.Render()
		return nil
	}
	name := "governed-compute"
	if len(args) > 1 {
		name = strings.Join(args[1:], " ")
	}
	if err := guard.HapiRegisterGoverned(pid, name); err != nil {
		res.Status = "error"
		res.Errors = []string{err.Error()}
		res.Render()
		return nil
	}
	res.Status = "ok"
	res.Summary = fmt.Sprintf("Now governing %s (pid %d). Under memory pressure Hapi may suspend (reversibly) then, at emergency, kill it — never WindowServer, the agents, or sirsi.", name, pid)
	res.Render()
	return nil
}

func runHapiRelease(cmd *cobra.Command, args []string) error {
	res := &output.CommandResult{Command: "sirsi hapi release"}
	pid, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil {
		res.Status = "error"
		res.Errors = []string{fmt.Sprintf("not a pid: %q", args[0])}
		res.Render()
		return nil
	}
	if err := guard.HapiUnregisterGoverned(pid); err != nil {
		res.Status = "error"
		res.Errors = []string{err.Error()}
		res.Render()
		return nil
	}
	res.Status = "ok"
	res.Summary = fmt.Sprintf("No longer governing pid %d.", pid)
	res.Render()
	return nil
}
