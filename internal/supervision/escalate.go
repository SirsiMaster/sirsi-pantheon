package supervision

import (
	"fmt"
	"sort"
	"strings"
)

// Escalation is one lane that needs the owner, because no automatic repair
// exists for it.
//
// This type exists because Escalates() had no caller. The classifier could
// already say a lane was UNROUTABLE and the dashboard could already paint it
// red, but nothing turned that into a message anyone would receive. A state
// only a watched surface reveals is not supervision — it is a dashboard that
// happens to be correct while nobody is looking at it.
type Escalation struct {
	Agent string
	State LaneState
	// OpenItems is the inbox depth that cannot be delivered.
	OpenItems int
	// Why is owner-facing prose: what is stuck and what clears it.
	Why string
	// Lanes is non-empty only on a ROLLUP escalation, which stands for every
	// lane it names instead of one. See rollup().
	Lanes []string
}

// Title is the dedup key AND the item title. One stable title per lane means a
// sweep running every 60s produces one open item per lane, not one per pass —
// dedup is by exact title against the owner's open inbox, so the title must not
// embed counts, ages, or anything else that drifts between passes.
func (e Escalation) Title() string {
	if len(e.Lanes) > 0 {
		// Deliberately carries NO count. The set of unroutable lanes drifts by
		// one every time a lane is opened or closed; a title like "7 lanes"
		// would mint a fresh card on each drift and rebuild the exact flood
		// this rollup exists to end.
		return "Lanes need you: multiple lanes cannot be reached automatically"
	}
	return fmt.Sprintf("Lane needs you: %s cannot be reached automatically", e.Agent)
}

// NeedsOwner reports whether a lane state requires the owner rather than a wake.
//
// Deliberately narrower than Escalates(): identical today, but kept separate so
// the escalation bar can rise without changing what the BOARD is allowed to
// display. The board must show every true state; the owner must only be paged
// for states no agent can clear. Collapsing the two is how a truthful surface
// becomes a pager that gets muted.
func NeedsOwner(st LaneState) bool { return Escalates(st) }

// Escalations returns the lanes to route to the owner, sorted by descending
// backlog so the deepest is read first.
//
// Lanes with NO open work are excluded even when unroutable. An unwakeable lane
// with an empty inbox is a correctly parked lane, not an incident —
// internal/router/strand.go made exactly this call for the stranded report, and
// reversing it here would resurrect the noise that reasoning killed. What is
// escalated is the narrow, real case: work exists, no automated path can deliver
// it to anyone, AND there is no registered live thread — i.e. the lane is
// genuinely stopped. A lane with active threads may be executing right now; the
// claim "cannot be reached automatically" is false for a lane that is running.
func Escalations(lanes []LaneInput, states map[string]LaneState) []Escalation {
	var out []Escalation
	for _, l := range lanes {
		st, ok := states[l.Agent]
		if !ok || !NeedsOwner(st) || l.OpenItems == 0 {
			continue
		}
		// Suppress if the lane has live thread records: startability (no wake
		// mechanism) and liveness (thread registry) are different properties.
		// A lane with wake:none can be perfectly alive — it was started by the
		// owner, not by horus. Escalating "nothing can start this lane" about a
		// lane that is currently executing is a false positive.
		if l.ActiveThreads > 0 {
			continue
		}
		out = append(out, Escalation{
			Agent:     l.Agent,
			State:     st,
			OpenItems: l.OpenItems,
			Why:       whyUnroutable(l),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OpenItems != out[j].OpenItems {
			return out[i].OpenItems > out[j].OpenItems
		}
		return out[i].Agent < out[j].Agent
	})
	return rollup(out)
}

// rollupThreshold is how many simultaneously-unroutable lanes stop being
// separate incidents and start being one fleet condition.
//
// Three is where the owner board stopped being readable in practice: on
// 2026-08-07 a single pass produced 21 per-lane cards, each individually true
// and correctly deduped, which buried the four unrelated items underneath them.
// Per-lane dedup was never the defect — it works exactly as documented. The
// defect is that "no supervisor exists for wake:none lanes" is ONE root cause
// and was being reported once per lane it happened to affect.
const rollupThreshold = 3

// rollup collapses a fleet-wide escalation into a single owner card.
//
// Below the threshold each lane keeps its own card: two stopped lanes are two
// incidents an owner can act on individually, and flattening them would lose
// the agent name from the title where it is most useful.
//
// The rollup's Why still names every lane and its backlog, so nothing is hidden
// — the collapse is in how many items the owner's inbox receives, not in how
// much they are told.
func rollup(in []Escalation) []Escalation {
	if len(in) < rollupThreshold {
		return in
	}
	var b strings.Builder
	total := 0
	names := make([]string, 0, len(in))
	for _, e := range in {
		total += e.OpenItems
		names = append(names, e.Agent)
	}
	fmt.Fprintf(&b, "%d lanes are holding %d open items between them and none can be reached automatically: ", len(in), total)
	b.WriteString(strings.Join(names, ", "))
	b.WriteString(".\n\nAutomatic delivery is unavailable, and no fresh usable thread was observed for these lanes. ")
	b.WriteString("This does not establish their wake configuration or whether stale registrations exist. ")
	b.WriteString("Inspect each lane's wake configuration, latest delivery result and host reachability before choosing a repair. ")
	b.WriteString("Opening a session can restore a usable consumer; queued work still needs a delivery acknowledgment.\n\nPer lane:\n")
	for _, e := range in {
		fmt.Fprintf(&b, "  - %s: %d open, state %s\n", e.Agent, e.OpenItems, e.State)
	}
	return []Escalation{{
		State:     in[0].State,
		OpenItems: total,
		Why:       b.String(),
		Lanes:     names,
	}}
}

func whyUnroutable(l LaneInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is holding %d open item", l.Agent, l.OpenItems)
	if l.OpenItems != 1 {
		b.WriteString("s")
	}
	b.WriteString(", automatic delivery is unavailable, and no fresh usable thread was observed. ")
	b.WriteString("This does not establish its wake configuration or whether stale registrations exist. ")
	if l.ActionableTasks > 0 {
		fmt.Fprintf(&b, "It also carries %d actionable ledger task(s). ", l.ActionableTasks)
	}
	if l.BlockedTasks > 0 {
		fmt.Fprintf(&b, "%d of its tasks record a blocker. ", l.BlockedTasks)
	}
	b.WriteString("Inspect its wake configuration, latest delivery result and host reachability before choosing a repair. ")
	b.WriteString("Opening a session can restore a usable consumer; queued work still needs a delivery acknowledgment.")
	return b.String()
}
