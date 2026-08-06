package router

import "time"

// Attended-session liveness (A36 §mechanical enforcement).
//
// The headless build worker defers items younger than SIRSI_WORKER_MIN_AGE so
// it never races an attended session that is already handling them: it is an
// anti-stall BACKSTOP, not a racer. That deferral was unconditional, so when NO
// attended session existed the item still waited the full window with a
// healthy, polling worker deliberately declining it — the mechanical shape of
// "the agent stopped and nothing noticed". Every surface reads green
// throughout: launchctl says running, the worker log shows a normal cadence,
// and the item is simply open.
//
// This turns the constant into a condition: defer only while there is somebody
// to defer TO.

// IsAttendedSurface reports whether a surface is an ATTENDED agent session — an
// interactive session that claims fresh inbox items itself.
//
// This is deliberately NARROWER than "consumer-capable". surfaceWorker is
// non-resident and watches its inbox (WatcherFor: pull-loop, WatchesInbox
// true), so every capability-shaped predicate — including AgentArmed — counts
// the headless backstop worker as a live consumer. Used to gate the worker's
// own deferral that inverts into a deadlock: the worker sees itself as the
// attended session it is supposed to be backing up, defers to itself forever,
// and the item is never processed at all. That is strictly worse than the
// unconditional wait it replaces.
//
// Resident UI surfaces (menubar/TUI/macapp) are excluded for the separate
// reason codex-pantheon ruled on PR #389: a resident heartbeats but does not
// consume, so its liveness is not evidence that anything will claim the item.
func IsAttendedSurface(surface string) bool {
	switch surface {
	case surfaceClaude, surfaceCodex:
		return true
	default:
		return false
	}
}

// AttendedSessionLive reports whether agentID has a live attended session that
// can be expected to claim fresh inbox items.
//
// Liveness itself is threadArmed — the same proof AgentArmed uses — so this
// never becomes a second, drifting definition of "alive". The only thing added
// is the surface narrowing.
//
// Fail direction: an unreadable registry reads as ATTENDED (true). Unknown
// authority must not make spending an agentic build MORE likely; degrading to
// the previous unconditional wait is the conservative outcome, never a worse
// one. This is the opposite direction from AgentArmed, which fails closed to
// surface a possible strand — there, the cost of being wrong is a spurious
// alarm; here, it is a burned build session racing a live operator.
func AttendedSessionLive(routerRoot, agentID string) bool {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return true
	}
	now := time.Now().UTC()
	for _, t := range reg.Threads {
		if t == nil || t.AgentID != agentID {
			continue
		}
		if t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
			continue
		}
		if !IsAttendedSurface(t.Surface) {
			continue
		}
		if threadArmed(t, now) {
			return true
		}
	}
	return false
}
