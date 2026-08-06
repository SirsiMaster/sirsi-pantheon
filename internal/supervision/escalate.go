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
}

// Title is the dedup key AND the item title. One stable title per lane means a
// sweep running every 60s produces one open item per lane, not one per pass —
// dedup is by exact title against the owner's open inbox, so the title must not
// embed counts, ages, or anything else that drifts between passes.
func (e Escalation) Title() string {
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
// escalated is the narrow, real case: work exists, and no automated path can
// deliver it to anyone.
func Escalations(lanes []LaneInput, states map[string]LaneState) []Escalation {
	var out []Escalation
	for _, l := range lanes {
		st, ok := states[l.Agent]
		if !ok || !NeedsOwner(st) || l.OpenItems == 0 {
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
	return out
}

func whyUnroutable(l LaneInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is holding %d open item", l.Agent, l.OpenItems)
	if l.OpenItems != 1 {
		b.WriteString("s")
	}
	b.WriteString(" and declares no wake mechanism, so nothing can start it automatically. ")
	if l.ActionableTasks > 0 {
		fmt.Fprintf(&b, "It also carries %d actionable ledger task(s). ", l.ActionableTasks)
	}
	if l.BlockedTasks > 0 {
		fmt.Fprintf(&b, "%d of its tasks record a blocker. ", l.BlockedTasks)
	}
	b.WriteString("Opening a session for this lane is the only thing that clears it — ")
	b.WriteString("re-running the supervisor will not, and waking harder will not.")
	return b.String()
}
