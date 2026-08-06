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
// Fail direction: an unreadable registry reads as NOT ATTENDED (false), so the
// worker takes the item.
//
// The earlier reasoning here was that returning true is "conservative" because
// it degrades to the previous unconditional wait. That is exactly backwards
// under A36/ADR-057. Returning true fabricates an attended consumer out of an
// authority failure: the registry is the only thing that can say whether a
// human-attended session exists, and when it cannot answer, inventing a "yes"
// re-creates the fixed idle window this function exists to remove — the item
// sits unclaimed while every surface reports that someone is handling it.
// Silently waiting is not the safe default; it is the failure mode.
//
// Taking the item is the recoverable direction. Its cost is a build session
// that may duplicate an operator's work, which is visible and bounded. The
// cost of the other direction is work that is never picked up at all and no
// signal that anything is wrong. This is now the same fail-closed direction as
// AgentArmed: an unreadable authority must surface as absence, never presence.
func AttendedSessionLive(routerRoot, agentID string) bool {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		return false
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
