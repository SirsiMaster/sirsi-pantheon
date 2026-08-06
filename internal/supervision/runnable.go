// Package supervision turns "never stop" from a promise into a system
// invariant.
//
// Owner directive 2026-08-06: "Operational enforcement must live in Pantheon's
// Go runtime, not prompts, heartbeats, or AGENTS files."
//
// That sentence is the whole reason this package exists. Every prior attempt at
// the same guarantee was a text instruction — a CLAUDE.md rule, an arm-your-
// watcher hook, a 15-minute heartbeat timer. None of them is enforcement: a
// heartbeat proves a process is alive, and a process being alive says nothing
// about whether it is working. An agent can heartbeat perfectly while holding
// no task and leaving an inbox full. Every surface reads green.
//
// This package defines the authoritative work predicate and the honest lane
// classification that follows from it.
package supervision

import (
	"time"
)

// LaneState classifies what a lane is ACTUALLY doing.
//
// The distinction this exists to draw is IdleWithWork vs Working. A lane with
// open items and a live heartbeat used to render as healthy, because liveness
// was measured by process existence. That is the failure mode the owner named:
// "Process existence and heartbeat prove only session liveness — not work."
//
// Working is therefore defined by RECENT TASK-RECORD MUTATION — evidence the
// lane changed the store — never by a process being up.
type LaneState string

const (
	// StateWorking: the lane mutated a task record recently. Evidence of work.
	StateWorking LaneState = "WORKING"
	// StateAssigned: actionable work exists and a valid worker lease is
	// pending. Somebody has claimed it and the claim has not expired.
	StateAssigned LaneState = "ASSIGNED"
	// StateIdleWithWork: work exists, nobody is moving it. THE defect state —
	// previously indistinguishable from WORKING.
	StateIdleWithWork LaneState = "IDLE_WITH_WORK"
	// StateBlocked: a RECORDED technical/security/privacy blocker. Not
	// inferred from inactivity — inactivity is IdleWithWork, and calling it
	// blocked is how a stall acquires a legitimate-sounding excuse.
	StateBlocked LaneState = "BLOCKED"
	// StateUnroutable: work exists and there is no valid wake path to anyone
	// who could do it. Escalates; it cannot be repaired by waking harder.
	StateUnroutable LaneState = "UNROUTABLE"
	// StateComplete: all three sources audited empty. The ONLY legitimate
	// parked state.
	StateComplete LaneState = "COMPLETE"
)

// Sources is the three-source work inventory for one lane, per the owner's
// authoritative predicate:
//
//	runnable = open router item
//	        OR actionable ledger task
//	        OR unmet traced canon requirement
type Sources struct {
	// OpenItems is the count of open inbox items addressed to the lane.
	OpenItems int
	// ActionableTasks counts ledger tasks that are not done and not blocked.
	// A blocked task is real work but not RUNNABLE work — counting it here
	// would make a fully-blocked lane look runnable forever.
	ActionableTasks int
	// UnmetRequirements counts canon requirements with no terminal verified
	// disposition. This is standing work whether or not anyone routed a
	// message about it.
	UnmetRequirements int
	// BlockedTasks counts tasks with a RECORDED blocker.
	BlockedTasks int
}

// Runnable is the authoritative predicate. A lane may park only when this is
// false across all three sources.
//
// Note what is absent: process liveness, heartbeat freshness, thread
// registration. None of them belongs in a predicate about whether WORK EXISTS.
// Conflating the two is precisely how a lane came to read healthy while holding
// an untouched inbox.
func (s Sources) Runnable() bool {
	return s.OpenItems > 0 || s.ActionableTasks > 0 || s.UnmetRequirements > 0
}

// Empty reports whether all three sources are simultaneously drained — the
// only condition under which a completion claim is admissible.
func (s Sources) Empty() bool {
	return s.OpenItems == 0 && s.ActionableTasks == 0 && s.UnmetRequirements == 0 && s.BlockedTasks == 0
}

// Lease is a transactional work claim. Its existence is what separates
// ASSIGNED from IDLE_WITH_WORK: somebody holds this item, and the claim has an
// expiry so a dead holder cannot pin work forever.
type Lease struct {
	LeaseID   string
	ItemID    string
	Holder    string
	ExpiresAt time.Time
}

// Valid reports whether the lease still holds at now. An expired lease is not
// a claim — it is orphaned work, and the lane owning it is IDLE_WITH_WORK, not
// ASSIGNED. Treating an expired lease as a live claim is how work silently
// stops while looking attended.
func (l Lease) Valid(now time.Time) bool {
	return l.LeaseID != "" && now.Before(l.ExpiresAt)
}

// LaneInput is everything needed to classify one lane honestly.
type LaneInput struct {
	Agent string
	Sources
	// LastTaskMutation is when this lane last CHANGED a task record. Zero
	// means never observed. This is the only accepted evidence of working.
	LastTaskMutation time.Time
	// Leases are the lane's outstanding claims, valid or not.
	Leases []Lease
	// Routable reports whether a valid wake path to this lane exists. False
	// means no amount of waking will reach it.
	Routable bool
	// ActiveThreads is the count of non-terminal, non-suspended thread records
	// for this agent. Zero means no registered session or worker is present
	// according to the thread registry. Positive means at least one live or
	// recently-stale thread exists — the lane may be running even if it has no
	// automatic wake mechanism. Startability (Routable) and liveness
	// (ActiveThreads) are different properties and must not be conflated.
	ActiveThreads int
}

// DefaultWorkWindow is how recently a lane must have mutated a task record to
// count as WORKING.
//
// Deliberately generous. The cost of being wrong in each direction is
// asymmetric: calling a working lane idle produces a spurious wake, which is
// cheap; calling an idle lane working hides stopped work, which is the entire
// failure this package exists to prevent. When uncertain, prefer IDLE_WITH_WORK.
const DefaultWorkWindow = 15 * time.Minute

// Classify renders the honest state of one lane.
//
// Order matters and encodes the priorities:
//  1. No work at all → COMPLETE. Nothing below can apply.
//  2. Work exists but cannot be routed → UNROUTABLE. No wake fixes this, so it
//     must not be reported as merely idle.
//  3. Recent task mutation → WORKING. Evidence, not liveness.
//  4. A VALID lease → ASSIGNED. Claimed, not yet evidenced.
//  5. Every actionable task carries a recorded blocker → BLOCKED.
//  6. Otherwise → IDLE_WITH_WORK. The honest default: work exists and nothing
//     is happening. This is the fall-through on purpose — a lane must earn any
//     better classification with evidence, never receive it by default.
func Classify(in LaneInput, now time.Time, window time.Duration) LaneState {
	if window <= 0 {
		window = DefaultWorkWindow
	}
	// COMPLETE is gated on Empty(), NOT on !Runnable(). Runnable() deliberately
	// excludes blocked work — blocked work is not startable — so a lane whose
	// only remaining work is blocked is !Runnable() while being very far from
	// complete. Gating on the predicate here returned COMPLETE for a lane
	// holding two recorded blockers: a false completion claim produced by the
	// supervisor itself.
	if in.Empty() {
		return StateComplete
	}
	if !in.Routable {
		return StateUnroutable
	}
	if !in.LastTaskMutation.IsZero() && now.Sub(in.LastTaskMutation) <= window {
		return StateWorking
	}
	for _, l := range in.Leases {
		if l.Valid(now) {
			return StateAssigned
		}
	}
	// Blocked only when NO runnable work remains and a recorded blocker
	// explains the rest. A lane with one blocked task and three runnable ones
	// is idle, not blocked — reporting it as blocked would hand a stall a
	// legitimate-sounding excuse. Open items and unmet requirements count as
	// runnable here too, so a lane with a full inbox and one blocker is idle.
	if !in.Runnable() && in.BlockedTasks > 0 {
		return StateBlocked
	}
	return StateIdleWithWork
}

// NeedsWake reports whether a lane should be woken now.
//
// IDLE_WITH_WORK is the whole point: work exists, nothing is moving, and a wake
// is the repair. UNROUTABLE also needs attention but a wake cannot deliver it,
// so it escalates instead — see Escalates.
func NeedsWake(st LaneState) bool { return st == StateIdleWithWork }

// Escalates reports whether a lane's state requires human or supervisor
// attention because no automatic repair exists.
func Escalates(st LaneState) bool { return st == StateUnroutable }
