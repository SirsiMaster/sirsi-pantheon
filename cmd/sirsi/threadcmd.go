package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

// watcherPidfile returns the per-thread fs-watcher pidfile path.
func watcherPidfile(threadID string) string {
	return fmt.Sprintf("/tmp/sirsi-router-watch-%s.pid", threadID)
}

// spawnRouterWatcher forks a detached `sirsi thread watch-router` subprocess
// that uses fsnotify on the router directory and fires the agent's spawn
// command on every change. Dies when parent_pid exits or `sirsi thread close`
// runs. Dedup via pidfile.
func spawnRouterWatcher(threadID, agentID, routerRoot string, parentPID int) error {
	pf := watcherPidfile(threadID)
	if data, err := os.ReadFile(pf); err == nil {
		if oldPID, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			if processAlive(oldPID) {
				return nil // existing watcher alive, dedup
			}
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "thread", "watch-router",
		"--thread", threadID,
		"--agent", agentID,
		"--router-root", routerRoot,
		"--parent-pid", strconv.Itoa(parentPID))
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = detachedSysProcAttr() // detach so we survive caller exit (Unix Setsid; nil on Windows)
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = os.WriteFile(pf, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
	_ = cmd.Process.Release() // don't wait on it
	return nil
}

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
func reapDeadPIDThreads(routerRoot string) []router.ReapedThread {
	host, _ := os.Hostname()
	reaped, _ := router.ReapDeadThreads(routerRoot, host)
	return reaped
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
			anchor = resolveAnchorPID()
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

		host, _ := os.Hostname()
		thr := &router.Thread{
			ThreadID:      threadRegID,
			AgentID:       threadRegAgent,
			Surface:       threadRegSurface,
			Repo:          repo,
			Workstream:    threadRegWorkstream,
			Watches:       threadRegWatches,
			WakeMechanism: threadRegWake,
			PID:           anchor,
			Host:          host,
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

// resolveAnchorPID returns the grandparent of sirsi (caller's caller),
// which is typically the agent runtime binary when register is invoked
// from a hook script. Falls back to PPID if grandparent lookup fails.
func resolveAnchorPID() int {
	ppid := os.Getppid()
	// macOS: ps -p <ppid> -o ppid= returns ppid's parent
	out, err := exec.Command("ps", "-p", strconv.Itoa(ppid), "-o", "ppid=").Output()
	if err != nil {
		return ppid
	}
	gp, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || gp <= 1 {
		return ppid
	}
	return gp
}

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
)

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

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-watcher.Events:
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
		reapedNow := reapDeadPIDThreads(routerRoot)
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
			rows = append(rows, row{thr: t, stale: t.IsStale(now, stale)})
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
	threadRegisterCmd.Flags().IntVar(&threadRegAnchorPID, "anchor-pid", 0, "PID to anchor the fs-watcher lifetime to (defaults to grandparent of sirsi)")

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

	threadCmd.AddCommand(threadRegisterCmd, threadHeartbeatCmd, threadCloseCmd, threadListCmd, threadPruneCmd, threadWatchRouterCmd)
}
