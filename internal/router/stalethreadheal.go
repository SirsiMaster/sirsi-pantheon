// Package router — stalethreadheal.go
//
// Supervisor-context stale-thread reconciliation (A27 — Heartbeat Loop Mandate).
//
// The registry-police files alarms when threads register but stop heartbeating
// ("registered-but-not-looping"). This duty heals those records in the background,
// between SessionStart reconcile runs, without a heavyweight Thoth sync.
//
// Design choices (agreed with EffectiveStale fix, ADR-022, ADR-025):
//
//   - ReapDeadThreads runs first: an "active" record whose PID is confirmed dead
//     by OS truth becomes "reaped" before the stale scan. This alone clears many
//     alarms because the EffectiveStale check refuses to call a PID-dead thread
//     stale until it is actually reaped.
//
//   - Reaped records are intentionally skipped: successor minting requires a live
//     session transcript (which only the owning agent has at SessionStart). Passing
//     ok=false to ReconcileExits appends a ReconcileUnrecoverable outcome but makes
//     zero mutations to the reaped record — the reaped terminal status is preserved.
//
//   - No Thoth sync: a supervisor-context heal runs ~every 15 minutes, often with
//     no associated live session. Shelling out to `sirsi thoth sync` here would
//     read a stale journal and write a meaningless thoth_ref — worse than an empty
//     one. The payload's SuspendedAt + best-effort open items are enough for the
//     operator to resume context on next SessionStart.
package router

import (
	"os"
	"time"
)

// RunStaleThreadReconcileDuty transitions genuinely-stale active threads (soft
// exits that skipped the SessionEnd hook) to suspended. It is called by the
// supervisor's "stale-thread-reconcile" duty and operates entirely within the
// router package — no external process, no Thoth sync.
//
// It intentionally does not mint successors for reaped (hard-killed) records;
// that recovery path requires transcript access and runs at SessionStart.
func RunStaleThreadReconcileDuty(routerRoot, repoRoot string) error {
	// Reap dead PIDs first so OS truth is reflected before reconcile evaluates
	// staleness. Records that are "active" with a dead PID become "reaped" here;
	// EffectiveStale and IsStale both skip reaped records, so the reconcile pass
	// below only sees genuinely-stale live-PID or no-PID records.
	_, _ = ReapDeadThreads(routerRoot)

	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	host, _ := os.Hostname()

	retro := func(t *Thread) (*SuspendPayload, bool) {
		// Reaped records: signal ReconcileExits to record unrecoverable but
		// make NO mutation (ok=false). Transcript-backed recovery defers to
		// the owning agent's SessionStart.
		if t != nil && t.Status == ThreadStatusReaped {
			return nil, false
		}
		// Stale-active (soft exit / harness-cleared session): build a minimal
		// suspend payload. No Thoth sync; attach open items best-effort so
		// resume has context for what work was in flight.
		p := &SuspendPayload{SuspendedAt: now}
		if r, newErr := New(repoRoot); newErr == nil {
			if st, stErr := r.ReadState(); stErr == nil {
				st.NormalizePending()
				if t != nil {
					p.OwnedOpenItems = st.Pending[t.AgentID]
				}
			}
		}
		return p, true
	}

	outcomes := ReconcileExits(reg, host, "" /* all agents */, now, DefaultThreadStaleAfter, retro)

	healed := 0
	for _, o := range outcomes {
		if o.Action == ReconcileSuspendedStale {
			healed++
		}
	}

	if healed > 0 {
		return SaveThreadRegistry(routerRoot, reg)
	}
	return nil
}
