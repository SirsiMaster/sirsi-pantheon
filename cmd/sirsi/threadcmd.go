package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

// watcherPidfile returns the per-thread fs-watcher pidfile path.
func watcherPidfile(threadID string) string {
	return fmt.Sprintf("/tmp/sirsi-router-watch-%s.pid", threadID)
}

// The fs-watcher is SPAWNED BY THE SURFACE, never by the router.
//
// `spawnRouterWatcher` used to live here and was forked by `thread discover`
// for every process it adopted. That is a self-feeding storm: the forked
// watch-router runs the agent's spawn command, starting a NEW agent process
// which is itself unregistered, so the next discover pass adopts it and forks
// again. Removed 2026-07-27 after it took the workstation to 358 `claude`
// processes, load average 436, and swap 48.5 GB of 49 GB.
//
// killRouterWatcher stays: watchers armed BY A SURFACE still need stopping on
// close/suspend, and the pidfile contract is unchanged.

// killRouterWatcher cleanly stops the fs-watcher for a thread, if any.
func killRouterWatcher(threadID string) {
	pf := watcherPidfile(threadID)
	data, err := os.ReadFile(pf)
	if err != nil {
		return
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
		_ = terminateProcess(pid)
	}
	_ = os.Remove(pf)
}

// reapDeadPIDThreads sweeps threads whose recorded PID is dead by OS truth —
// gone OR defunct (zombie Z) — to the terminal `reaped` status, so a late
// heartbeat can no longer revive them to `active`. It scopes to this host only
// (remote process tables are unobservable) and delegates the OS-truth check to
// router.ReapDeadThreads, which detects zombies that `kill -0` cannot.
//
// Called automatically at the top of `sirsi thread list` so orphans get
// swept whenever anyone reads the registry — no daemon, no polling,
// per AGENTS.md §Lean #1 (the read IS the event).
// reapDeadPIDThreads returns every thread retired this pass and any error that
// prevented a full sweep. A non-nil error means the OS-truth reconciliation is
// incomplete: the returned slice contains whatever was salvaged, but callers
// MUST surface the error — a failed sweep is byte-identical to "nothing to
// reap", which causes dead threads to appear 🟢 active.
func reapDeadPIDThreads(routerRoot string) ([]router.ReapedThread, error) {
	reaped, err1 := router.ReapDeadThreads(routerRoot)
	// ADR-024: after dead-PID actives retire, sweep superseded strays (duplicate
	// suspends/ghosts of a surface a live watcher already holds), so the read
	// enforces one-live-watcher-per-surface — not just OS-truth on actives.
	strays, err2 := router.ReapStrayThreads(routerRoot)
	return append(reaped, strays...), errors.Join(err1, err2)
}

var (
	threadRegAgent      string
	threadRegSurface    string
	threadRegRepo       string
	threadRegWorkstream string
	threadRegWatches    []string
	threadRegWake       string
	threadRegID         string
	threadRegAnchorPID  int
	// threadRegConsumerCapable is claimed by a RESIDENT worker publishing its own
	// inbox-consumer capability (guarded in RunE — see the resident check there).
	threadRegConsumerCapable bool

	threadHbID      string
	threadHbStatus  string
	threadHbItem    string
	threadHbError   string
	threadCloseID   string
	threadListAll   bool
	threadListStale time.Duration
)

var threadCmd = &cobra.Command{
	Use:   "thread",
	Short: "CTR — register and track live agent threads",
	Long: `CTR thread registry. Every active agent thread/session (claude, codex,
gemini, gemma, qwen, mcp, api, webhook) should register a thread so
Horus can show which conversations are alive on this workstation.

  sirsi thread register --agent claude-pantheon --surface claude --repo .
  sirsi thread heartbeat --thread thr-abcd1234
  sirsi thread list
  sirsi thread close --thread thr-abcd1234`,
}

var threadRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register the current thread/session with CTR",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := threadRegRepo
		if repo == "" {
			rr, err := router.FindRepoRoot()
			if err != nil {
				return fmt.Errorf("no idea-router found and --repo not provided: %w", err)
			}
			repo = rr
		}
		absRepo, err := filepath.Abs(repo)
		if err == nil {
			repo = absRepo
		}
		routerRoot := filepath.Join(repo, ".agents", "idea-router")
		if _, statErr := os.Stat(routerRoot); statErr != nil {
			return fmt.Errorf("router directory not found at %s", routerRoot)
		}

		if threadRegAgent == "" {
			return fmt.Errorf("--agent is required")
		}
		if threadRegSurface == "" {
			return fmt.Errorf("--surface is required (claude|codex|gemini|gemma|qwen|mcp|api|webhook|worker)")
		}
		// The registry PID must be the long-lived agent process. If we store
		// this `sirsi thread register` process, the read-time reaper closes the
		// thread immediately after register exits.
		anchor := threadRegAnchorPID
		if anchor <= 0 {
			anchor, err = resolveAnchorPID(threadRegSurface)
			if err != nil {
				return fmt.Errorf("resolve durable thread anchor: %w; pass --anchor-pid for resident surfaces without a recognizable runtime", err)
			}
		}

		// ADR-024 Amendment 1 §2: a one-shot (`--print`/`-p`) worker is neither an
		// interactive nor a resident surface, so it MUST NOT enroll as a persistent
		// CTR thread (the ephemeral-worker accretion source). Refuse-to-register is
		// a no-op, not an error — a worker shouldn't fail because it tried, and it
		// may still read/act on the router without enrolling.
		if ephemeralWorkerSkip(anchor) {
			if JsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"skipped": "one-shot worker — not an interactive/resident surface (ADR-024 Amendment 1 §2)",
					"pid":     anchor,
				})
			}
			fmt.Printf("thread register: skipped — pid %d is a one-shot (--print/-p) worker, not an interactive/resident surface (ADR-024 Amendment 1). It may read/act on the router without enrolling a persistent thread.\n", anchor)
			return nil
		}

		// A36/A27 follow-up to PR #389: a RESIDENT consumer publishes and
		// heartbeats its OWN consumer-capable thread. The wake watcher refuses to
		// borrow that capability (`capable := consumer != nil && !consumer.Resident`
		// in RunWakeLoop) because a startup health check proves a binary can print
		// a string, not that a consumer is alive — gemma-pantheon's is literally
		// `sirsi-gemma-worker.sh --version`. Liveness you did not observe is not
		// liveness you can borrow. This flag is the publish side of that inversion.
		//
		// Guarded, because self-asserted capability is the same false-green class:
		// an armed lane suppresses its own rescue in WakePass, so only an agent
		// whose registry entry DECLARES a resident consumer may claim it. A
		// watch-only loop cannot arm itself by passing a flag.
		if threadRegConsumerCapable {
			reg, regErr := router.LoadRegistry(routerRoot)
			if regErr != nil {
				return fmt.Errorf("--consumer-capable requires the agent registry, which is unreadable: %w", regErr)
			}
			cfg, ok := reg.Agents[threadRegAgent]
			if !ok {
				return fmt.Errorf("--consumer-capable: agent %q is not registered — only a registered agent declaring consumer.mode=resident may publish consumer capability", threadRegAgent)
			}
			if !cfg.DeclaresResidentConsumer() {
				return fmt.Errorf("--consumer-capable: agent %q does not declare consumer.mode=resident — capability is published by the resident worker itself, and a lane whose inbox is drained by a spawned consumer.command must not claim it", threadRegAgent)
			}
		}

		host, _ := os.Hostname()
		thr := &router.Thread{
			ThreadID: threadRegID,
			AgentID:  threadRegAgent,
			Surface:  threadRegSurface,
			// Only ever RAISED here; credit LAPSES by going stale in WakePass's
			// armed predicate, which is the acceptance clause "a resident that
			// stops publishing stops being credited". Nothing needs to clear it.
			ConsumerCapable: threadRegConsumerCapable,
			Repo:            repo,
			Workstream:      threadRegWorkstream,
			Watches:         threadRegWatches,
			WakeMechanism:   threadRegWake,
			PID:             anchor,
			Host:            host,
			// Session-keyed lease: capture the stable app-hosted session identity
			// so re-registrations from subsequent hook fires renew the same record
			// instead of minting fresh ones (claude-home mint-churn fix). No-op
			// for process-backed surfaces (non-Claude) where this env var is unset.
			SessionID: router.CurrentSessionID(),
		}
		// Fill wake mechanism from registry if not provided.
		if thr.WakeMechanism == "" {
			if reg, regErr := router.LoadRegistry(routerRoot); regErr == nil {
				if cfg, ok := reg.Agents[threadRegAgent]; ok {
					thr.WakeMechanism = cfg.WakeMechanism()
					if thr.Workstream == "" {
						thr.Workstream = cfg.Workstream
					}
				}
			}
		}
		// Default watches to the agent's own inbox.
		if len(thr.Watches) == 0 {
			thr.Watches = []string{threadRegAgent}
		}

		out, err := router.RegisterThread(routerRoot, thr)
		if err != nil {
			return err
		}

		// Thread-liveness piece 1: record session→agent so the PRD-keepalive Stop
		// hook can resolve this session's agent even when cwd=$HOME (CCD sessions).
		// Best-effort: a marker failure must never fail registration.
		if mErr := router.WriteSessionAgentMarker(router.CurrentSessionID(), out.AgentID); mErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write session→agent marker: %v\n", mErr)
		}

		// ADR-024: register is a pure handshake. It no longer auto-spawns an
		// fs-watcher; it RETURNS the canonical watcher the surface must arm.
		// The router owns the surface→watcher mapping (R4 inventory in code);
		// the surface owns the arming. register always returns the spec, even
		// when the supervisor is off (SIRSI_SUPERVISOR=0 suppresses managed
		// arming/enforcement only — the spec stays visible for diagnostics).
		spec := router.WatcherFor(out.Surface, out.AgentID, out.ThreadID)

		// ADR-024 §6 (discover-bridge lifecycle guard): a self-register for
		// this (agent_id, pid) is authoritative — the real session is present
		// and will arm the canonical watcher above. Supersede any adoption
		// fs-watcher the `discover` bridge spawned for this thread, else the
		// bridge AND the prescribed watcher both run = duplicate accretion
		// (codex follow-up, router item 205359 #1). Always safe: the bridge is
		// only ever the discover-spawned `watch-router` fork, never the
		// surface's canonical watcher, so removing it can never strand a thread.
		killRouterWatcher(out.ThreadID)

		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(struct {
				*router.Thread
				Watcher router.WatcherSpec `json:"watcher"`
			}{out, spec})
		}
		fmt.Printf("Registered thread %s\n", out.ThreadID)
		fmt.Printf("  agent: %s (surface=%s)\n", out.AgentID, out.Surface)
		fmt.Printf("  watches: %s\n", strings.Join(out.Watches, ", "))
		fmt.Printf("  repo:  %s\n", out.Repo)
		if out.WakeMechanism != "" {
			fmt.Printf("  wake:  %s\n", out.WakeMechanism)
		}
		fmt.Printf("  status: %s\n", out.Status)
		fmt.Println()
		fmt.Println("Watcher (arm exactly this — ADR-024):")
		fmt.Printf("  type: %s  (heartbeat %ds, watches_inbox=%v, resident=%v)\n",
			spec.Type, spec.HeartbeatIntervalS, spec.WatchesInbox, spec.Resident)
		fmt.Printf("  %s\n", spec.ArmInstruction)
		fmt.Println()
		fmt.Println("Send heartbeats with:")
		fmt.Printf("  sirsi thread heartbeat --thread %s\n", out.ThreadID)
		return nil
	},
}

// threadAttendedCmd exists for the headless build worker, which is bash and
// must not re-derive liveness with jq over `thread list --json`. A second
// definition of "alive" in shell is how the two drift apart.
//
// Exit status IS the answer so the caller is one `if` with no parsing:
// 0 = an attended session is live, 1 = none. Any real failure returns an error
// (exit 1 as well, with a message on stderr) — and 1 is the safe direction for
// this caller, which reads it as "nobody is attending, take the item".
var threadAttendedCmd = &cobra.Command{
	Use:   "attended <agent-id>",
	Short: "Exit 0 if the agent has a live ATTENDED session (claude/codex) consuming its inbox",
	Long: `Reports whether an agent has a live attended session — an interactive
claude/codex session actually consuming its inbox, proven by loop evidence for
claude and heartbeat freshness for codex.

Deliberately narrower than "armed": a headless worker loop is armed and
consumer-capable but is NOT an attended session, so it never counts as somebody
to defer to. The backstop worker uses this to decide whether deferring a fresh
item has anyone to defer TO.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		live := router.AttendedSessionLive(routerRoot, args[0])
		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(map[string]any{"agent_id": args[0], "attended": live}); err != nil {
				return err
			}
		} else if !quietMode {
			if live {
				fmt.Printf("attended session live for %s\n", args[0])
			} else {
				fmt.Printf("no attended session for %s\n", args[0])
			}
		}
		if !live {
			// SilenceUsage/SilenceErrors: a false answer is a normal result,
			// not a usage error — printing the whole help text here would bury
			// it in the worker's log every poll.
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			return errNoAttendedSession
		}
		return nil
	},
}

// errNoAttendedSession carries the nonzero exit for the false answer without
// printing a second error line over the message already written above.
var errNoAttendedSession = errors.New("")

var threadHeartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Send a heartbeat for a registered thread",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		if threadHbID == "" {
			return fmt.Errorf("--thread is required")
		}
		upd := router.HeartbeatUpdate{}
		if threadHbStatus != "" {
			upd.Status = router.ThreadStatus(threadHbStatus)
		}
		if cmd.Flags().Changed("current-item") {
			upd.CurrentItem = &threadHbItem
		}
		if cmd.Flags().Changed("last-error") {
			upd.LastError = &threadHbError
		}
		thr, err := router.Heartbeat(routerRoot, threadHbID, upd)
		if err != nil {
			return err
		}
		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(thr)
		}
		fmt.Printf("Heartbeat ok — %s (status=%s, last_seen=%s)\n",
			thr.ThreadID, thr.Status, thr.LastSeenAt.Format(time.RFC3339))
		return nil
	},
}

var threadCloseCmd = &cobra.Command{
	Use:   "close",
	Short: "Mark a thread as closed",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		if threadCloseID == "" {
			return fmt.Errorf("--thread is required")
		}
		thr, err := router.CloseThread(routerRoot, threadCloseID)
		if err != nil {
			return err
		}
		killRouterWatcher(threadCloseID)
		// Thread-liveness piece 1: drop this session's marker on close (idempotent).
		if mErr := router.RemoveSessionAgentMarker(router.CurrentSessionID()); mErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not remove session→agent marker: %v\n", mErr)
		}
		fmt.Printf("Closed thread %s (agent=%s)\n", thr.ThreadID, thr.AgentID)
		return nil
	},
}

var (
	threadPruneOlderThan       time.Duration
	threadPruneSuspendedOlderT time.Duration
)

var threadPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove terminal threads (closed/reaped) older than --older-than",
	Long: `Permanently delete terminal thread records (closed or reaped) whose
last_seen_at is older than the cutoff. Live, idle, blocked, and stale threads
are never pruned. This keeps threads.json from accumulating tombstones — the
registry churn that re-triggers Spotlight indexing on every write.

Suspended threads (ADR-025) are NON-terminal and are NEVER pruned by default —
they are resumable. Pass --suspended-older-than <dur> to ALSO remove suspended
records whose suspend time is older than that window (abandoned pauses never
resumed); this is the opt-in retention bound that stops suspended state from
accreting unbounded (it skips recent suspends, preserving resume-later).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		reg, err := router.LoadThreadRegistry(routerRoot)
		if err != nil {
			return err
		}
		before := len(reg.Threads)
		// --older-than 0 means "prune every terminal record regardless of age".
		// PruneClosed treats maxAge<=0 as disabled (a wipe-all guard), so map 0
		// to the smallest positive window to express prune-all intent.
		cutoff := threadPruneOlderThan
		if cutoff <= 0 {
			cutoff = time.Nanosecond
		}
		removed := reg.PruneClosed(time.Now().UTC(), cutoff)
		// ADR-025 opt-in suspended retention: only when the flag is set, never by
		// default (suspended is resumable). Keep retention<=0 as a no-op, matching
		// PruneStaleSuspended's wipe-all guard.
		suspRemoved := 0
		if cmd.Flags().Changed("suspended-older-than") {
			suspRemoved = reg.PruneStaleSuspended(time.Now().UTC(), threadPruneSuspendedOlderT)
		}
		if removed+suspRemoved > 0 {
			if err := router.SaveThreadRegistry(routerRoot, reg); err != nil {
				return err
			}
		}
		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]int{"before": before, "removed": removed, "suspended_removed": suspRemoved, "remaining": before - removed - suspRemoved})
		}
		fmt.Printf("Pruned %d terminal + %d stale-suspended thread(s) (%d → %d records)\n",
			removed, suspRemoved, before, before-removed-suspRemoved)
		return nil
	},
}

var (
	watchThreadID   string
	watchAgentID    string
	watchRouterRoot string
	watchParentPID  int
	watchDebounce   = 800 * time.Millisecond
	watchAliveCheck = 30 * time.Second
	// store-wake goroutine pacing (fork-storm guards, ADR-036/037 cutover):
	watchWaitBackoff = 30 * time.Second // after a failed `router wait`, before retry
	watchDrainPoll   = 5 * time.Second  // between drain checks after a wake, before re-arming
	// Cap the post-wake drain wait so a poison item (unroutable, an agent with no
	// spawn command, an item no handler ever closes) can't wedge the loop forever
	// and silence later items for this agent. After the cap we re-arm anyway; a
	// still-open item just re-wakes at most once per cap window — bounded, not a
	// storm — and re-arming re-reads ALL open items so item B is never stranded.
	watchDrainMaxPolls = 60 // 60 × 5s = 5 min
)

// sleepCtx sleeps for d or until ctx is canceled. Returns false if canceled
// (caller should stop), true if the full duration elapsed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

var threadWatchRouterCmd = &cobra.Command{
	Use:    "watch-router",
	Short:  "Internal: long-running per-thread fs-watcher (spawned by `thread register`)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if watchThreadID == "" || watchAgentID == "" || watchRouterRoot == "" || watchParentPID <= 0 {
			return fmt.Errorf("--thread, --agent, --router-root, --parent-pid all required")
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		defer watcher.Close()

		for _, path := range []string{
			filepath.Join(watchRouterRoot, "state.json"),
			filepath.Join(watchRouterRoot, "items"),
			filepath.Join(watchRouterRoot, "proposals"),
		} {
			if _, err := os.Stat(path); err == nil {
				_ = watcher.Add(path)
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Liveness ticker: exit when parent PID dies.
		go func() {
			t := time.NewTicker(watchAliveCheck)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if !processAlive(watchParentPID) {
						cancel()
						return
					}
				}
			}
		}()

		// Debounce: coalesce rapid bursts into one dispatch.
		var debounceTimer *time.Timer
		fire := func() {
			handleRouterEvent(watchThreadID, watchAgentID, watchRouterRoot)
		}

		// Post-cutover (ADR-036/037) the items/ directory is no longer written,
		// so fsnotify never fires for router items and this durable watcher would
		// go deaf. When the store-wake flag is on, park a goroutine on the store's
		// per-agent wake FIFO (`router wait`) and feed its wakes into the SAME
		// debounce+fire path — so the RunAtLoad watcher wakes on store rows, while
		// fsnotify still covers state.json / proposals / any legacy files.
		storeEvents := make(chan struct{}, 1)
		if routercfg.StoreWake() {
			if self, err := os.Executable(); err == nil {
				// `router wait` resolves the repo via FindRepoRoot (cwd/git), NOT the
				// --router-root we were handed; a launchd cwd of / or $HOME would make
				// it error every iteration. Anchor the subprocess to this repo.
				repoDir := filepath.Dir(filepath.Dir(watchRouterRoot)) // <repo> from <repo>/.agents/idea-router
				go func() {
					for ctx.Err() == nil {
						c := exec.CommandContext(ctx, self, "router", "wait", watchAgentID, "--timeout", "50")
						c.Dir = repoDir
						out, waitErr := c.CombinedOutput()
						if ctx.Err() != nil {
							return
						}
						if waitErr != nil {
							// wait failed (repo resolution, store open, …). Back off so a
							// persistent failure can't become a fork-storm.
							sleepCtx(ctx, watchWaitBackoff)
							continue
						}
						if !strings.Contains(string(out), "• ") {
							continue // timed out with an empty inbox — re-block, no event
						}
						// Work is present. Deliver ONE wake, then wait for the dispatched
						// session to DRAIN the inbox before re-arming. `router wait` is
						// level-triggered (returns instantly while any item is open), so
						// without this the loop would re-fork `router wait` continuously
						// for the whole time the item stays open — a fork-storm that also
						// keeps resetting the debounce so fire() never runs.
						select {
						case storeEvents <- struct{}{}:
						default:
						}
						for polls := 0; ctx.Err() == nil && polls < watchDrainMaxPolls; polls++ {
							if !sleepCtx(ctx, watchDrainPoll) {
								return
							}
							p := exec.CommandContext(ctx, self, "router", "pull", watchAgentID)
							p.Dir = repoDir
							pout, _ := p.CombinedOutput()
							if !strings.Contains(string(pout), "• ") {
								break // inbox drained — safe to re-arm the wait
							}
							// else keep polling until drained or the cap — then re-arm
							// anyway so a poison item can't permanently wedge this agent.
						}
					}
				}()
			}
		}

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-watcher.Events:
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(watchDebounce, fire)
			case <-storeEvents:
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(watchDebounce, fire)
			case <-watcher.Errors:
				// Non-fatal; keep going.
			}
		}
	},
}

// handleRouterEvent is called per debounced fsnotify burst. It checks the
// agent's inbox; if items exist, runs the agent's spawn command from
// agents.json with a ctr prompt over stdin. Same shape as dispatch.sh but
// scoped to one agent and one thread.
func handleRouterEvent(threadID, agentID, routerRoot string) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	// Pull this agent's inbox via sirsi router pull.
	out, err := exec.Command(self, "router", "pull", agentID).CombinedOutput()
	if err != nil {
		return
	}
	hasItems := false
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "• ") {
			hasItems = true
			break
		}
	}
	if !hasItems {
		return
	}

	// Find agent's spawn command + cwd in agents.json.
	reg, err := router.LoadRegistry(routerRoot)
	if err != nil {
		return
	}
	agent, ok := reg.Agents[agentID]
	if !ok || len(agent.Command) == 0 {
		return
	}

	prompt := fmt.Sprintf("ctr\n\nYou are %s on this workstation (thread %s).\nRead %s/state.json and act on items addressed to %s. Write router artifacts, ack/close, then stop.\n",
		agentID, threadID, routerRoot, agentID)

	cmd := exec.Command(agent.Command[0], agent.Command[1:]...)
	if agent.Cwd != "" {
		cmd.Dir = agent.Cwd
	}
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = detachedSysProcAttr()
	if err := cmd.Start(); err == nil {
		_ = cmd.Process.Release()
	}
}

var (
	threadWatchInstall   bool
	threadWatchUninstall bool
	threadWatchAgent     string
)

// resolveCurrentAgent determines which registered agent THIS thread/session
// belongs to, without requiring the caller to know its own agent id — the same
// resolution order `sirsi thread heartbeat`/the Stop hook rely on:
//  1. an explicit --agent flag (operator override),
//  2. $SIRSI_AGENT_ID (the canonical per-session env, if set),
//  3. the session→agent marker written at `thread register`
//     (~/.claude/run/agent-by-session/<session_id>),
//  4. a sole live, non-terminal registered thread on this host (unambiguous).
//
// Returns "" with a helpful reason when it cannot be resolved unambiguously, so
// the caller can tell the operator to pass --agent rather than guessing.
func resolveCurrentAgent(routerRoot, override string) (string, string) {
	if a := strings.TrimSpace(override); a != "" {
		return a, "flag"
	}
	if a := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID")); a != "" {
		return a, "env SIRSI_AGENT_ID"
	}
	if a := router.ReadSessionAgentMarker(router.CurrentSessionID()); a != "" {
		return a, "session marker"
	}
	// Sole-live-thread fallback: unambiguous only when exactly one non-terminal
	// thread is registered on this host.
	if reg, err := router.LoadThreadRegistry(routerRoot); err == nil {
		var candidates []string
		seen := map[string]bool{}
		now := time.Now().UTC()
		for _, t := range reg.SortedThreads() {
			if t.Status.IsTerminal() {
				continue
			}
			if router.EffectiveStale(t, now, router.DefaultThreadStaleAfter) {
				continue
			}
			if !seen[t.AgentID] {
				seen[t.AgentID] = true
				candidates = append(candidates, t.AgentID)
			}
		}
		if len(candidates) == 1 {
			return candidates[0], "sole live thread"
		}
	}
	return "", "could not resolve the current agent — pass --agent <id> (no $SIRSI_AGENT_ID, no session marker, and not a sole live thread)"
}

// threadWatchCmd is the thread-scoped, self-resolving alias over the existing
// per-agent wake LaunchAgent channel (`sirsi router wake-install <agent>`). A27
// asks every thread to arm a DURABLE, cross-session watcher; that durable channel
// already exists as the launchd pull-loop installed by router.InstallWakeLaunchAgent
// and run by `router wake-loop`. This verb adds NO new watcher subsystem — it just
// (a) resolves the CURRENT thread's agent the way heartbeat does, so a resuming
// session need not know its own agent id, and (b) delegates install/uninstall to
// that one existing path (Rule 0, reuse-not-reinvent).
//
//	sirsi thread watch              → show whether the durable watcher is installed
//	sirsi thread watch --install    → install it for the current thread's agent
//	sirsi thread watch --uninstall  → remove it (clean off)
var threadWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Install/inspect this thread's durable cross-session wake channel (A27)",
	Long: `Arm (or inspect) the DURABLE, cross-session watcher for the current thread.

A27 requires every registered thread to run a heartbeat loop that survives session
restarts. For worker/headless surfaces that persistent loop is a per-agent launchd
pull-loop — the SAME channel installed by ` + "`sirsi router wake-install <agent>`" + ` and
run by ` + "`sirsi router wake-loop`" + `. This command is a thin, thread-scoped alias over
that existing channel: it resolves the current thread's agent (via --agent,
$SIRSI_AGENT_ID, the session→agent marker, or a sole live thread) and installs,
removes, or reports the durable wake LaunchAgent for it. It introduces no second
watcher — ` + "`sirsi thread watch --install`" + ` == ` + "`sirsi router wake-install <that agent>`" + `.

Interactive claude sessions heartbeat via /loop (see the router README "Heartbeat
Loop"); the launchd pull-loop is for worker/headless surfaces.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

		agentID, how := resolveCurrentAgent(routerRoot, threadWatchAgent)
		if agentID == "" {
			return fmt.Errorf("%s", how)
		}
		reg, err := router.LoadRegistry(routerRoot)
		if err != nil {
			return fmt.Errorf("load agents: %w", err)
		}
		cfg, err := reg.Lookup(agentID)
		if err != nil {
			return fmt.Errorf("agent %q (resolved via %s) not registered: %w", agentID, how, err)
		}

		switch {
		case threadWatchInstall && threadWatchUninstall:
			return fmt.Errorf("pass only one of --install / --uninstall")

		case threadWatchUninstall:
			removed, path, uerr := router.UninstallWakeLaunchAgent(*cfg)
			if uerr != nil {
				return uerr
			}
			if JsonOutput {
				return jsonPrint(map[string]any{"agent": agentID, "resolved_via": how, "removed": removed, "path": path})
			}
			if removed {
				fmt.Printf("✔ Removed durable wake channel for %s: %s\n", agentID, path)
			} else {
				fmt.Printf("✓ No durable wake channel installed for %s (nothing to remove): %s\n", agentID, path)
			}
			return nil

		case threadWatchInstall:
			// Delegate to the EXISTING install path — zero duplicate logic.
			changed, path, ierr := router.InstallWakeLaunchAgent(*cfg, "")
			if ierr != nil {
				return ierr
			}
			if JsonOutput {
				return jsonPrint(map[string]any{"agent": agentID, "resolved_via": how, "installed": true, "changed": changed, "path": path})
			}
			if changed {
				fmt.Printf("✔ Installed durable wake channel for %s (resolved via %s): %s\n", agentID, how, path)
				fmt.Printf("  Load it: launchctl load -w %s\n", path)
				fmt.Printf("  (equivalent to `sirsi router wake-install %s`)\n", agentID)
			} else {
				fmt.Printf("✓ Durable wake channel already installed for %s (no change): %s\n", agentID, path)
			}
			return nil

		default:
			installed := router.WakeLaunchAgentInstalled(agentID)
			path := filepath.Join(router.WakeLaunchAgentLabel(agentID) + ".plist")
			if JsonOutput {
				return jsonPrint(map[string]any{"agent": agentID, "resolved_via": how, "installed": installed, "label": router.WakeLaunchAgentLabel(agentID)})
			}
			if installed {
				fmt.Printf("● Durable wake channel INSTALLED for %s (resolved via %s)\n", agentID, how)
				fmt.Printf("  label: %s\n", router.WakeLaunchAgentLabel(agentID))
				fmt.Printf("  Remove it: sirsi thread watch --uninstall\n")
			} else {
				fmt.Printf("○ No durable wake channel installed for %s (resolved via %s)\n", agentID, how)
				fmt.Printf("  Install it: sirsi thread watch --install\n")
				_ = path
			}
			return nil
		}
	},
}

// jsonPrint marshals v as indented JSON to stdout (shared by thread watch modes).
func jsonPrint(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

var threadListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered threads (active by default)",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		// Sweep dead/defunct-PID threads to `reaped` before reading (OS truth).
		reapedNow, reapErr := reapDeadPIDThreads(routerRoot)
		reg, err := router.LoadThreadRegistry(routerRoot)
		if err != nil {
			return err
		}
		stale := threadListStale
		if stale <= 0 {
			stale = router.DefaultThreadStaleAfter
		}
		now := time.Now().UTC()

		type row struct {
			thr   *router.Thread
			stale bool
		}
		var rows []row
		for _, t := range reg.SortedThreads() {
			if t.Status.IsTerminal() && !threadListAll {
				continue
			}
			// Loop-evidence-aware (A28): a thread with a live watcher loop is NOT
			// stale even if its heartbeat aged out (harness-gated surfaces). This
			// is the `.stale` field the registry-police trusts. Write-free.
			rows = append(rows, row{thr: t, stale: router.EffectiveStale(t, now, stale)})
		}

		if JsonOutput {
			payload := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				payload = append(payload, map[string]any{
					"thread":       r.thr,
					"stale":        r.stale,
					"idle_seconds": now.Sub(r.thr.LastSeenAt).Seconds(),
				})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		}

		output.Header("CTR — Live Threads")
		fmt.Println()
		// OS-truth integrity warning: surface what the reaper just retired so
		// the operator knows the registry disagreed with the live process table.
		// A sweep error is a separate warning: a failed reconciliation is
		// byte-identical to "nothing to reap", so dead threads would silently
		// render as 🟢 active without this notice (D-TL-1).
		if reapErr != nil {
			fmt.Printf("  ⚠️  integrity: OS-truth sweep incomplete — dead threads may appear active: %v\n", reapErr)
			fmt.Println()
		}
		if len(reapedNow) > 0 {
			fmt.Printf("  ⚠️  integrity: reaped %d dead/defunct thread(s) against OS truth this read:\n", len(reapedNow))
			for _, r := range reapedNow {
				fmt.Printf("       %s (agent=%s pid=%d %s)\n", r.ThreadID, r.AgentID, r.PID, r.State)
			}
			fmt.Println()
		}
		if len(rows) == 0 {
			fmt.Println("  No registered threads. Run `sirsi thread register --agent <id> --surface <surface>`.")
			return nil
		}
		for _, r := range rows {
			marker := "🟢"
			if r.stale {
				marker = "⚠️"
			}
			if r.thr.Status == router.ThreadStatusClosed {
				marker = "⚫"
			} else if r.thr.Status == router.ThreadStatusReaped {
				marker = "💀"
			} else if r.thr.Status == router.ThreadStatusBlocked {
				marker = "⛔"
			} else if r.thr.Status == router.ThreadStatusIdle {
				marker = "💤"
			}
			fmt.Printf("  %s %s  agent=%s surface=%s status=%s\n",
				marker, r.thr.ThreadID, r.thr.AgentID, r.thr.Surface, r.thr.Status)
			fmt.Printf("      last_seen=%s (idle %.0fs)\n",
				r.thr.LastSeenAt.Format(time.RFC3339),
				now.Sub(r.thr.LastSeenAt).Seconds())
			if len(r.thr.Watches) > 0 {
				fmt.Printf("      watches=%s\n", strings.Join(r.thr.Watches, ","))
			}
			if r.thr.CurrentItem != "" {
				fmt.Printf("      current_item=%s\n", r.thr.CurrentItem)
			}
			if r.thr.LastError != "" {
				fmt.Printf("      last_error=%s\n", r.thr.LastError)
			}
		}
		// Legend: only print when at least one stale row was shown so the
		// threshold isn't a mystery (D-TL medium — ⚠ must name its criterion).
		for _, r := range rows {
			if r.stale {
				fmt.Printf("\n  (⚠ = heartbeat older than %s; override with --stale-after)\n", stale)
				break
			}
		}
		return nil
	},
}

func init() {
	threadRegisterCmd.Flags().StringVar(&threadRegAgent, "agent", "", "Registered agent ID (e.g., claude-pantheon)")
	threadRegisterCmd.Flags().StringVar(&threadRegSurface, "surface", "", "Surface: claude|codex|gemini|gemma|qwen|mcp|api|webhook|worker")
	threadRegisterCmd.Flags().StringVar(&threadRegRepo, "repo", "", "Repository root (defaults to current router root)")
	threadRegisterCmd.Flags().StringVar(&threadRegWorkstream, "workstream", "", "Workstream name (optional)")
	threadRegisterCmd.Flags().StringSliceVar(&threadRegWatches, "watch", nil, "Inboxes this thread watches (defaults to --agent)")
	threadRegisterCmd.Flags().StringVar(&threadRegWake, "wake", "", "Wake mechanism (defaults to agent registry entry)")
	threadRegisterCmd.Flags().StringVar(&threadRegID, "thread", "", "Reuse a known thread_id instead of generating a new one")
	threadRegisterCmd.Flags().IntVar(&threadRegAnchorPID, "anchor-pid", 0, "PID to anchor thread lifetime to (default: verified durable runtime ancestor for known interactive surfaces)")
	threadRegisterCmd.Flags().BoolVar(&threadRegConsumerCapable, "consumer-capable", false, "This thread DRAINS the agent's inbox (resident consumers only — the worker publishes its own capability instead of the wake watcher borrowing it)")

	threadHeartbeatCmd.Flags().StringVar(&threadHbID, "thread", "", "Thread ID to heartbeat (required)")
	threadHeartbeatCmd.Flags().StringVar(&threadHbStatus, "status", "", "Set status: active|idle|blocked")
	threadHeartbeatCmd.Flags().StringVar(&threadHbItem, "current-item", "", "Currently active work item ID")
	threadHeartbeatCmd.Flags().StringVar(&threadHbError, "last-error", "", "Last error string")

	threadCloseCmd.Flags().StringVar(&threadCloseID, "thread", "", "Thread ID to close (required)")

	threadListCmd.Flags().BoolVar(&threadListAll, "all", false, "Include closed threads")
	threadListCmd.Flags().DurationVar(&threadListStale, "stale-after", router.DefaultThreadStaleAfter, "Stale threshold")

	threadWatchRouterCmd.Flags().StringVar(&watchThreadID, "thread", "", "Thread ID")
	threadWatchRouterCmd.Flags().StringVar(&watchAgentID, "agent", "", "Agent ID")
	threadWatchRouterCmd.Flags().StringVar(&watchRouterRoot, "router-root", "", "Router root directory")
	threadWatchRouterCmd.Flags().IntVar(&watchParentPID, "parent-pid", 0, "Parent process to anchor lifetime to")

	threadPruneCmd.Flags().DurationVar(&threadPruneOlderThan, "older-than", 24*time.Hour, "Delete terminal threads whose last_seen is older than this")
	threadPruneCmd.Flags().DurationVar(&threadPruneSuspendedOlderT, "suspended-older-than", 0, "Also delete suspended (ADR-025) threads whose suspend time is older than this (opt-in retention; off by default)")

	threadWatchCmd.Flags().BoolVar(&threadWatchInstall, "install", false, "Install the durable cross-session wake LaunchAgent for the current thread's agent")
	threadWatchCmd.Flags().BoolVar(&threadWatchUninstall, "uninstall", false, "Remove the durable wake LaunchAgent for the current thread's agent")
	threadWatchCmd.Flags().StringVar(&threadWatchAgent, "agent", "", "Agent id override (defaults to the current thread's agent)")

	threadCmd.AddCommand(threadRegisterCmd, threadHeartbeatCmd, threadCloseCmd, threadListCmd, threadPruneCmd, threadWatchRouterCmd, threadWatchCmd, threadAttendedCmd)
}
