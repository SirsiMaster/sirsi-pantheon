package main

// threadsuspend.go — ADR-025 (Thoth-gated exit) CLI verbs: suspend, resume, and
// reconcile. These complete A27's lifecycle (register → heartbeat → suspend ⇄
// resume → close) and give dirty exits an authoritative heal at SessionStart.
//
// Helpers (resolveAnchorPID, killRouterWatcher, reapDeadPIDThreads, JsonOutput)
// live in threadcmd.go in this same package.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

// bestEffortThothSync captures memory before a suspend/reconcile so the payload's
// thoth_ref is fresh. It returns the memory cross-ref (short commit SHA of the
// repo HEAD after sync) and whether capture SUCCEEDED — the actionable signal
// ADR-025 §4 needs: if sync failed (sirsi missing, no transcript, error), memory
// was NOT captured and a reaped record must warn rather than mint a successor.
func bestEffortThothSync(repoRoot string) (ref string, ok bool) {
	self, err := os.Executable()
	if err != nil {
		return "", false
	}
	sync := exec.Command(self, "thoth", "sync")
	sync.Dir = repoRoot
	if syncErr := sync.Run(); syncErr != nil {
		return "", false // memory capture failed — caller treats as unrecoverable
	}
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", true // synced, but no git ref to cross-reference (still recoverable)
	}
	return strings.TrimSpace(string(out)), true
}

// countUncommitted returns the number of uncommitted (modified/untracked) files
// in repoRoot via `git status --porcelain`. Read-only — never mutates the tree.
// Used to surface working-tree state that may be stranded when a thread is reaped
// (ADR-024 §6 / stranded-work finding; claude-home ruling 053800). 0 on error.
func countUncommitted(repoRoot string) int {
	out, err := exec.Command("git", "-C", repoRoot, "status", "--porcelain").Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// reapedClass reports whether a reconcile outcome processed a reaped record —
// the signal that a thread just died, so any uncommitted WIP may be stranded.
func reapedClass(a router.ReconcileAction) bool {
	return a == router.ReconcileMintedSuccessor || a == router.ReconcileUnrecoverable
}

// ownedOpenItems returns the router item ids still addressed to agentID, so a
// suspended thread carries its unfinished work into resume (A26 owned items).
func ownedOpenItems(repoRoot, agentID string) []string {
	r, err := router.New(repoRoot)
	if err != nil {
		return nil
	}
	st, err := r.ReadState()
	if err != nil {
		return nil
	}
	st.NormalizePending()
	return st.Pending[agentID]
}

// resolveSelfThreadID finds the live (non-terminal, non-suspended) thread on THIS
// host whose recorded PID is the current session's anchor — the SessionEnd-hook
// entry point for `suspend --self`. Errors if no live thread matches (nothing to
// suspend).
func resolveSelfThreadID(routerRoot string) (string, error) {
	anchor, err := resolveAnchorPID("")
	if err != nil {
		return "", fmt.Errorf("resolve current session anchor: %w; pass --thread explicitly", err)
	}
	host, _ := os.Hostname()
	reg, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return "", err
	}
	for id, t := range reg.Threads {
		if t == nil || t.PID != anchor {
			continue
		}
		if host != "" && t.Host != "" && t.Host != host {
			continue
		}
		if t.Status.IsTerminal() || t.Status == router.ThreadStatusSuspended {
			continue
		}
		return id, nil
	}
	return "", fmt.Errorf("no live thread found for this session (anchor pid=%d); pass --thread explicitly", anchor)
}

var (
	threadSuspendID     string
	threadSuspendSelf   bool
	threadSuspendPrompt string
	threadSuspendNoSync bool
)

var threadSuspendCmd = &cobra.Command{
	Use:   "suspend",
	Short: "Suspend a thread to the resumable state (ADR-025) — captures memory + open items",
	Long: `Flip an active thread to the resumable 'suspended' state, snapshotting a
continuation payload: a fresh thoth_ref (memory is synced first), the router items
it still owns, and an optional one-line resume prompt. Suspended is non-terminal
and non-live — it survives prune, the reaper skips it, and heartbeats are rejected
until 'sirsi thread resume' restores it. This is the default Thoth-gated exit (a
pause, not a death). Idempotent.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

		threadID := threadSuspendID
		if threadSuspendSelf {
			id, resolveErr := resolveSelfThreadID(routerRoot)
			if resolveErr != nil {
				return resolveErr
			}
			threadID = id
		}
		if threadID == "" {
			return fmt.Errorf("--thread or --self is required")
		}

		reg, err := router.LoadThreadRegistry(routerRoot)
		if err != nil {
			return err
		}
		rec := reg.Threads[threadID]
		if rec == nil {
			return fmt.Errorf("thread %q not registered", threadID)
		}

		payload := &router.SuspendPayload{
			ResumePrompt: threadSuspendPrompt,
			SuspendedAt:  time.Now().UTC(),
		}
		if !threadSuspendNoSync {
			if ref, ok := bestEffortThothSync(repoRoot); ok {
				payload.ThothRef = ref
			}
		}
		payload.OwnedOpenItems = ownedOpenItems(repoRoot, rec.AgentID)

		thr, err := router.SuspendThread(routerRoot, threadID, payload)
		if err != nil {
			return err
		}
		killRouterWatcher(threadID) // suspended is not live — stop its fs-watcher

		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(thr)
		}
		fmt.Printf("Suspended thread %s (agent=%s)\n", thr.ThreadID, thr.AgentID)
		if p := thr.SuspendPayload; p != nil {
			if p.ThothRef != "" {
				fmt.Printf("  thoth_ref: %s\n", p.ThothRef)
			}
			fmt.Printf("  owned open items: %d\n", len(p.OwnedOpenItems))
			if p.ResumePrompt != "" {
				fmt.Printf("  resume prompt: %s\n", p.ResumePrompt)
			}
		}
		fmt.Printf("Resume with: sirsi thread resume --thread %s\n", thr.ThreadID)
		return nil
	},
}

var threadResumeID string

var threadResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a suspended thread (ADR-025) — restores owned items, re-arms the watcher",
	Long: `Restore a suspended thread to active: re-surface the router items it still
owns, print its resume prompt and memory ref, and return the canonical watcher
spec for the surface to re-arm (ADR-024 handshake). The persisted suspend_payload
is cleared on resume. Errors if the thread is not suspended.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		if threadResumeID == "" {
			return fmt.Errorf("--thread is required")
		}

		thr, err := router.ResumeThread(routerRoot, threadResumeID)
		if err != nil {
			return err
		}
		// ADR-024: resume re-arms the watcher via the same handshake as register —
		// return the canonical spec; the surface owns the actual arming.
		spec := router.WatcherFor(thr.Surface, thr.AgentID, thr.ThreadID)

		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(struct {
				*router.Thread
				Watcher router.WatcherSpec `json:"watcher"`
			}{thr, spec})
		}
		fmt.Printf("Resumed thread %s (agent=%s, status=%s)\n", thr.ThreadID, thr.AgentID, thr.Status)
		if p := thr.SuspendPayload; p != nil {
			if len(p.OwnedOpenItems) > 0 {
				fmt.Println("  owned items to pick back up:")
				for _, it := range p.OwnedOpenItems {
					fmt.Printf("    • %s\n", it)
				}
			}
			if p.ResumePrompt != "" {
				fmt.Printf("  resume prompt: %s\n", p.ResumePrompt)
			}
			if p.ThothRef != "" {
				fmt.Printf("  memory ref: %s\n", p.ThothRef)
			}
		}
		fmt.Println()
		fmt.Println("Watcher (re-arm exactly this — ADR-024):")
		fmt.Printf("  %s\n", spec.ArmInstruction)
		fmt.Printf("Heartbeat with: sirsi thread heartbeat --thread %s\n", thr.ThreadID)
		return nil
	},
}

var threadReconcileAgent string

var threadReconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Heal dirty exits (ADR-025 §4) — the authoritative SessionStart gate",
	Long: `Heal threads that exited without a clean suspend/close — the gate that always
runs at SessionStart, since SessionEnd cannot block and /clear or hard-kill may
skip it. Reaps dead PIDs first, then:

  - a STALE ACTIVE record (soft-exit / clear) is healed in place: memory is
    retro-synced and it transitions active→suspended.
  - a REAPED record (hard kill) is never revived; if memory is recoverable a new
    suspended SUCCESSOR is minted (reaped_from), else an unrecoverable warning is
    surfaced — never silent.

Scope to one agent with --agent (each surface heals its own lineage at its own
start). Honors SIRSI_SUPERVISOR=0 (managed action — skipped when off; manual
suspend/resume always work).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("SIRSI_SUPERVISOR") == "0" {
			if JsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]string{"skipped": "SIRSI_SUPERVISOR=0"})
			}
			fmt.Println("reconcile skipped (SIRSI_SUPERVISOR=0 — managed actions disabled)")
			return nil
		}
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

		// Reap dead PIDs first so reaped records reflect OS truth before healing.
		if _, reapErr := reapDeadPIDThreads(routerRoot); reapErr != nil {
			fmt.Fprintf(os.Stderr, "warning: OS-truth sweep incomplete (reconcile may see stale actives): %v\n", reapErr)
		}

		reg, err := router.LoadThreadRegistry(routerRoot)
		if err != nil {
			return err
		}
		host, _ := os.Hostname()
		retro := func(t *router.Thread) (*router.SuspendPayload, bool) {
			ref, ok := bestEffortThothSync(repoRoot)
			p := &router.SuspendPayload{
				ThothRef:       ref,
				OwnedOpenItems: ownedOpenItems(repoRoot, t.AgentID),
				SuspendedAt:    time.Now().UTC(),
			}
			return p, ok
		}
		outcomes := router.ReconcileExits(reg, host, threadReconcileAgent, time.Now().UTC(), router.DefaultThreadStaleAfter, retro)
		if len(outcomes) > 0 {
			if err := router.SaveThreadRegistry(routerRoot, reg); err != nil {
				return err
			}
		}

		// Stranded-work surfacing (ADR-024 §6 / claude-home ruling 053800): when a
		// reaped thread was freshly reconciled AND the working tree has uncommitted
		// changes, those changes MAY be stranded from the dead thread. Read-only —
		// surface for adopt-or-discard; never stash (repo-global, cross-lane harm)
		// and never attribute (git can't map hunks to threads). A18 is the real
		// prevention; this is the safety net.
		reapedFreshly := false
		for _, o := range outcomes {
			if reapedClass(o.Action) {
				reapedFreshly = true
				break
			}
		}
		strandedN := 0
		if reapedFreshly {
			strandedN = countUncommitted(repoRoot)
		}

		if JsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{"healed": len(outcomes), "outcomes": outcomes, "stranded_uncommitted": strandedN})
		}
		if len(outcomes) == 0 {
			fmt.Println("reconcile: no dirty exits to heal")
			return nil
		}
		for _, o := range outcomes {
			switch o.Action {
			case router.ReconcileSuspendedStale:
				fmt.Printf("healed (stale→suspended): %s [%s]\n", o.ThreadID, o.AgentID)
			case router.ReconcileMintedSuccessor:
				fmt.Printf("healed (reaped→successor): %s [%s] → %s\n", o.ThreadID, o.AgentID, o.SuccessorID)
			case router.ReconcileUnrecoverable:
				fmt.Fprintf(os.Stderr, "⚠ memory UNRECOVERABLE for reaped %s [%s] — no transcript, no successor minted\n", o.ThreadID, o.AgentID)
			}
		}
		if strandedN > 0 {
			fmt.Fprintf(os.Stderr, "⚠ %d uncommitted file(s) may be stranded from reaped thread(s) — review with `git status`; adopt (commit) or discard explicitly (never auto-stashed)\n", strandedN)
		}
		return nil
	},
}

func init() {
	threadSuspendCmd.Flags().StringVar(&threadSuspendID, "thread", "", "Thread id to suspend")
	threadSuspendCmd.Flags().BoolVar(&threadSuspendSelf, "self", false, "Suspend the current session's thread (SessionEnd hook use)")
	threadSuspendCmd.Flags().StringVar(&threadSuspendPrompt, "resume-prompt", "", "One-line continuation recorded in the payload (e.g. a NOTEBOOKS resume name)")
	threadSuspendCmd.Flags().BoolVar(&threadSuspendNoSync, "no-sync", false, "Skip the thoth sync (payload carries no fresh thoth_ref)")

	threadResumeCmd.Flags().StringVar(&threadResumeID, "thread", "", "Thread id to resume")

	threadReconcileCmd.Flags().StringVar(&threadReconcileAgent, "agent", "", "Scope reconciliation to one agent id (SessionStart hook use)")

	threadCmd.AddCommand(threadSuspendCmd, threadResumeCmd, threadReconcileCmd)
}
