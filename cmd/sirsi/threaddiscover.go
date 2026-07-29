package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

// localProcessSurfaces are agent surfaces that run as an OS process we can
// discover via pgrep/lsof. Non-process surfaces (mcp, api, webhook, worker)
// are never enumerated.
var localProcessSurfaces = map[string]bool{
	"claude": true, "codex": true, "gemini": true, "gemma": true, "qwen": true,
}

var threadDiscoverSelf bool

var threadDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Query running agent sessions and register the mappable ones",
	Long: `Reconcile the live thread registry with reality.

Enumerates running agent processes on THIS host (targeted pgrep/lsof — never a
broad scan), resolves each one's working directory to an agent in agents.json,
and registers any live session that isn't already tracked, anchored to its PID
so the existing watcher/reaper lifecycle owns it.

Sessions whose cwd is not under a known repo (e.g. launched from $HOME) are
reported as unmappable and intentionally NOT registered. Sessions whose cwd
matches more than one agent of the same surface are reported as ambiguous and
left for the operator to disambiguate in agents.json.

  sirsi thread discover          # reconcile every running agent session
  sirsi thread discover --self   # register only the current session (hook use)
  sirsi thread discover --json   # machine-readable report for sweeps`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

		reg, err := router.LoadRegistry(routerRoot)
		if err != nil {
			return err
		}
		// Reap dead-PID threads first so "already registered" reflects reality.
		reapDeadPIDThreads(routerRoot)
		threads, err := router.LoadThreadRegistry(routerRoot)
		if err != nil {
			return err
		}
		host, _ := os.Hostname()

		var procs []router.DiscoveredProc
		if threadDiscoverSelf {
			procs = selfProc()
		} else {
			procs = enumerateAgentProcs(localSurfaces(reg))
		}

		actions := router.ReconcileDiscovery(reg, threads, procs, host)

		registered := 0
		for i := range actions {
			if actions[i].Outcome != router.OutcomeRegister {
				continue
			}
			a := actions[i]
			thr := &router.Thread{
				AgentID: a.AgentID,
				Surface: a.Proc.Surface,
				Repo:    a.Repo,
				PID:     a.Proc.PID,
				Host:    host,
				Watches: []string{a.AgentID},
			}
			if cfg, ok := reg.Agents[a.AgentID]; ok {
				thr.Workstream = cfg.Workstream
				thr.WakeMechanism = cfg.WakeMechanism()
			}
			out, regErr := router.RegisterThread(routerRoot, thr)
			if regErr != nil {
				// Surface the failure honestly; do not count it as registered.
				actions[i].Outcome = router.OutcomeUnmappable
				actions[i].Reason = "register failed: " + regErr.Error()
				continue
			}
			registered++
			actions[i].Reason = "registered " + out.ThreadID

			// ADR-024: discover REGISTERS, it does not arm. It used to fork a
			// `watch-router` bridge here, and that fork is a self-feeding storm:
			// watch-router runs the agent's spawn command, which starts a NEW
			// agent process; that process is unregistered, so the next discover
			// pass finds it, registers it, and forks another watcher — which
			// starts another agent. On the supervisor's cadence the population
			// multiplies every pass. Observed 2026-07-27: 358 `claude` processes
			// and 267 zombies, load average 436, swap 48.5 GB of 49 GB, the
			// machine reporting "system has run out of application memory".
			//
			// This is the same fix ADR-024 already applied to `register` ("a pure
			// handshake — it no longer auto-spawns an fs-watcher; it RETURNS the
			// canonical watcher the surface must arm"). discover was the caller
			// that kept the old behavior. The surface arms its own watcher; a
			// discovered process is ALREADY RUNNING and needs watching, never
			// launching.
			actions[i].Reason += " (watcher: " +
				router.WatcherFor(out.Surface, out.AgentID, out.ThreadID).Type + " — arm at the surface)"
		}

		return renderDiscover(host, actions, registered)
	},
}

// localSurfaces returns the distinct process-surfaces present in the registry,
// sorted for deterministic enumeration.
func localSurfaces(reg *router.Registry) []string {
	set := make(map[string]bool)
	for _, a := range reg.Agents {
		if localProcessSurfaces[a.Type] {
			set[a.Type] = true
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// enumerateAgentProcs finds running processes for each surface via `pgrep -x`
// and resolves each one's cwd via `lsof`. Bounded and side-effect free.
func enumerateAgentProcs(surfaces []string) []router.DiscoveredProc {
	self := os.Getpid()
	seen := make(map[int]bool)
	var procs []router.DiscoveredProc
	for _, s := range surfaces {
		out, err := exec.Command("pgrep", "-x", s).Output()
		if err != nil {
			continue // pgrep exits non-zero when there are no matches
		}
		for _, f := range strings.Fields(string(out)) {
			pid, err := strconv.Atoi(f)
			if err != nil || pid <= 1 || pid == self || seen[pid] {
				continue
			}
			seen[pid] = true
			if isOneShotWorker(pid) {
				continue // skip `--print` workers (incl. our own spawns) and one-shots
			}
			procs = append(procs, router.DiscoveredProc{PID: pid, Surface: s, Cwd: resolveProcCwd(pid)})
		}
	}
	return procs
}

// selfProc builds the single DiscoveredProc for the current session, used by
// the SessionStart hook (`discover --self`). The session process is sirsi's
// grandparent (the agent binary); the project dir comes from the runtime env
// when set, falling back to the current working directory.
func selfProc() []router.DiscoveredProc {
	cwd := os.Getenv("CLAUDE_PROJECT_DIR")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	surface := os.Getenv("CLAUDE_AGENT_SURFACE")
	if surface == "" {
		surface = "claude"
	}
	return []router.DiscoveredProc{{PID: resolveAnchorPID(), Surface: surface, Cwd: cwd}}
}

// resolveProcCwd returns a process's working directory via lsof, or "" if it
// cannot be resolved (process gone, permission denied, etc.).
func resolveProcCwd(pid int) string {
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}

// isOneShotWorker reports whether a process is a non-interactive `--print`
// worker (including watcher-spawned agents) or otherwise not a live session
// we should track. Best-effort: on lookup failure it returns false.
func isOneShotWorker(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return false
	}
	cmdline := string(out)
	return strings.Contains(cmdline, "--print") || strings.Contains(cmdline, " -p ")
}

// oneShotProbe is the injectable one-shot-worker predicate (Rule A16) so the
// register gate (ADR-024 Amendment 1 §2) is testable without spawning real
// `--print` processes.
var oneShotProbe = isOneShotWorker

// ephemeralWorkerSkip reports whether a `thread register` invocation must be
// refused: ADR-024 Amendment 1 §2 gates persistent registration to interactive
// and resident surfaces, and a one-shot (`--print`/`-p`) worker is neither. A
// non-positive anchor is never skipped (unverifiable — fall through to register).
func ephemeralWorkerSkip(anchorPID int) bool {
	return anchorPID > 0 && oneShotProbe(anchorPID)
}

type discoverReport struct {
	Host       string                  `json:"host"`
	Discovered int                     `json:"discovered"`
	Registered int                     `json:"registered"`
	Skipped    int                     `json:"skipped"`
	Unmappable int                     `json:"unmappable"`
	Ambiguous  int                     `json:"ambiguous"`
	Actions    []router.DiscoverAction `json:"actions"`
}

func renderDiscover(host string, actions []router.DiscoverAction, registered int) error {
	rep := discoverReport{Host: host, Discovered: len(actions), Registered: registered, Actions: actions}
	for _, a := range actions {
		switch a.Outcome {
		case router.OutcomeSkip:
			rep.Skipped++
		case router.OutcomeUnmappable:
			rep.Unmappable++
		case router.OutcomeAmbiguous:
			rep.Ambiguous++
		}
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	output.Header("CTR — Thread Discovery")
	fmt.Println()
	fmt.Printf("  discovered=%d  registered=%d  skipped=%d  unmappable=%d  ambiguous=%d\n",
		rep.Discovered, rep.Registered, rep.Skipped, rep.Unmappable, rep.Ambiguous)
	if len(actions) == 0 {
		fmt.Println("\n  No running agent sessions found on this host.")
		return nil
	}
	fmt.Println()
	for _, a := range actions {
		marker := map[router.DiscoverOutcome]string{
			router.OutcomeRegister:   "✅",
			router.OutcomeSkip:       "⏭️",
			router.OutcomeUnmappable: "🏠",
			router.OutcomeAmbiguous:  "❓",
		}[a.Outcome]
		who := a.AgentID
		if who == "" {
			who = a.Proc.Cwd
		}
		fmt.Printf("  %s %-11s pid=%-6d %s\n", marker, a.Outcome, a.Proc.PID, who)
		if a.Reason != "" {
			fmt.Printf("        %s\n", a.Reason)
		}
	}
	return nil
}

func init() {
	threadDiscoverCmd.Flags().BoolVar(&threadDiscoverSelf, "self", false, "Register only the current session (SessionStart hook use)")
	threadCmd.AddCommand(threadDiscoverCmd)
}
