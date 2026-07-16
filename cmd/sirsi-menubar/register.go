// Package main — sirsi-menubar
//
// register.go — CTR router registration (A26/A27).
//
// The menubar is a resident interactive surface, not just a renderer. Per the
// Heartbeat Loop Mandate (A27) it registers one router thread bound to its own
// PID and heartbeats on a bounded interval so Horus/Ra can see it alive.
//
// Registration is best-effort: if no router root is reachable, the menubar runs
// unregistered rather than failing to launch. The bridge to the dashboard
// contract (replacing the Terminal-spawn menu actions) is deliberately NOT here
// — that is Step 2 (ADR-020 surface ladder). This file does Step 1 only.
package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// menubarAgentID is the stable CTR identity of the resident menubar surface.
// Stable across restarts so OS-truth reaping (ADR-022) retires the prior PID's
// record while this launch registers a fresh one bound to os.Getpid().
const menubarAgentID = "sirsi-menubar"

// menubarHeartbeatInterval is deliberately coarse. The earlier lockup traced to
// registry write amplification feeding Spotlight (mds_stores) — a resident
// surface must NOT heartbeat on a frequent stats tick. 60s is the resident-worker
// floor (codex constraint on item 20260601-055029).
const menubarHeartbeatInterval = 60 * time.Second

// resolveRouterRoot locates this workstation's idea-router directory.
// Order: explicit env override, walk up from cwd, then the conventional dev
// checkout. Returns ("", false) when none is found, and the caller skips
// registration — the menubar must launch regardless.
func resolveRouterRoot() (string, bool) {
	if env := os.Getenv("SIRSI_ROUTER_ROOT"); env != "" && isDir(env) {
		return env, true
	}
	if repo, err := router.FindRepoRoot(); err == nil {
		if root := filepath.Join(repo, ".agents", "idea-router"); isDir(root) {
			return root, true
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		cand := filepath.Join(home, "Development", "sirsi-pantheon", ".agents", "idea-router")
		if isDir(cand) {
			return cand, true
		}
	}
	return "", false
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// registerMenubarThread registers the resident menubar surface and starts its
// heartbeat loop. Returns (routerRoot, threadID) so the quit handler can close
// the thread cleanly; empty strings mean registration was skipped or failed.
//
// Idempotency: RegisterThread reuses a live record with the same (agent_id, pid),
// so a re-register within one process lifetime never duplicates. Across launch
// restarts the PID differs; the prior record is retired by OS-truth reaping
// (ADR-022), and `sirsi thread list` reaps dead PIDs before printing, so no
// duplicate *active* record is ever shown.
func registerMenubarThread(ctx context.Context) (routerRoot, threadID string) {
	root, ok := resolveRouterRoot()
	if !ok {
		return "", ""
	}
	host, _ := os.Hostname()
	repo := filepath.Dir(filepath.Dir(root)) // <repo>/.agents/idea-router → <repo>
	thr, err := router.RegisterThread(root, &router.Thread{
		AgentID:       menubarAgentID,
		Surface:       "menubar",
		Repo:          repo,
		PID:           os.Getpid(),
		Host:          host,
		Status:        router.ThreadStatusActive,
		WakeMechanism: "menubar-runloop",
	})
	if err != nil {
		return "", ""
	}
	// Bound registry growth (A27 write-amplification → Spotlight mds_stores).
	// Registration is idempotent on (agent_id, pid), so every relaunch is a NEW
	// PID = a new record; the prior one is reaped by OS-truth (ADR-022) but the
	// terminal record lingers in threads.json until pruned. A resident surface
	// that relaunches often would otherwise accrete unbounded reaped rows. Reap
	// this host's dead PIDs, then drop the menubar's OWN stale terminal records
	// (agent-scoped — never touches other agents' history). Best-effort: a
	// failure here must never stop the menubar from launching.
	pruneOwnStaleRecords(root, thr.ThreadID)
	go heartbeatLoop(ctx, root, thr.ThreadID)
	return root, thr.ThreadID
}

// menubarRecordRetention is how long a terminal (reaped/closed) menubar record
// is kept before this surface prunes its own. Generous enough to preserve recent
// launch history for debugging, short enough that threads.json never accretes.
const menubarRecordRetention = time.Hour

// pruneOwnStaleRecords reaps this host's dead PIDs, then removes the menubar's
// own terminal records older than the retention window, keeping the live record
// (keepThreadID) untouched. Scoped to menubarAgentID so it is safe for a single
// surface to run on every launch without disturbing other agents.
func pruneOwnStaleRecords(routerRoot, keepThreadID string) {
	_, _ = router.ReapDeadThreads(routerRoot) // dead-PID actives → terminal
	reg, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	removed := 0
	for id, t := range reg.Threads {
		if t == nil || id == keepThreadID || t.AgentID != menubarAgentID {
			continue
		}
		if t.Status.IsTerminal() && now.Sub(t.LastSeenAt) > menubarRecordRetention {
			delete(reg.Threads, id)
			removed++
		}
	}
	if removed > 0 {
		_ = router.SaveThreadRegistry(routerRoot, reg)
	}
}

// heartbeatLoop emits a bounded-interval heartbeat until ctx is canceled.
func heartbeatLoop(ctx context.Context, routerRoot, threadID string) {
	t := time.NewTicker(menubarHeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = router.Heartbeat(routerRoot, threadID, router.HeartbeatUpdate{
				Status: router.ThreadStatusActive,
			})
		}
	}
}

// closeMenubarThread marks the resident thread closed on clean shutdown. If this
// never runs (hard kill / crash), OS-truth reaping (ADR-022) retires the dead PID.
func closeMenubarThread(routerRoot, threadID string) {
	if routerRoot == "" || threadID == "" {
		return
	}
	_, _ = router.CloseThread(routerRoot, threadID)
}
