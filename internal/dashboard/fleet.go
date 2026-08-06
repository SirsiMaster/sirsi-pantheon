package dashboard

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/supervision"
)

// Fleet board — the A32 owner-reporting surface.
//
// This replaces `~/.sirsi/ledger-dashboard/server.py`, a Python board that
// lived outside the repo and was therefore never reviewed, tested, or shipped
// with the CLI it reported on. Two independent renderings of one router
// guarantee divergence; this is the surviving one.
//
// Three sections, matching what the retired board actually showed:
//
//   - Summary tiles: completed/total, in-progress/assigned, stalled+blocked.
//   - Activity: real status changes, newest first.
//   - Lanes: every agent, active first, with recency.

// FleetEvent is one observed status transition.
type FleetEvent struct {
	At      string `json:"at"`
	Agent   string `json:"agent"`
	TaskID  string `json:"task_id"`
	Subject string `json:"subject"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// Lane states come from internal/supervision — the authoritative
// classification, not a second one defined here.
//
// The three-state model this replaces (working/blocked/stopped) had no way to
// say IDLE_WITH_WORK: a lane with a full inbox and nothing moving rendered
// identically to one actively shipping, because "working" was inferred from
// having open work rather than from evidence of mutation. That is the exact
// conflation the owner's supervision directive forbids — process existence and
// heartbeat prove session liveness, never work.
const (
	LaneWorking      = string(supervision.StateWorking)
	LaneAssigned     = string(supervision.StateAssigned)
	LaneIdleWithWork = string(supervision.StateIdleWithWork)
	LaneBlocked      = string(supervision.StateBlocked)
	LaneUnroutable   = string(supervision.StateUnroutable)
	LaneComplete     = string(supervision.StateComplete)
)

// FleetLane is one agent's row.
type FleetLane struct {
	Agent   string `json:"agent"`
	State   string `json:"state"`
	Open    int    `json:"open"`
	Active  int    `json:"active"`
	Stalled int    `json:"stalled"`
	Blocked int    `json:"blocked"`
	// Inbox is open ROUTER items — a separate source from tasks. The
	// classification counts it, so the row must show it: a lane read
	// "0 open · IDLE — work waiting", which is only explicable once you can
	// see the inbox work driving it.
	Inbox int `json:"inbox"`
	// Routable reports whether any automated wake path reaches this lane. A
	// false value is why a lane can read UNROUTABLE: work exists and no sweep,
	// retry, or nudge will ever deliver it.
	Routable   bool   `json:"routable"`
	TouchedAgo string `json:"touched_ago,omitempty"`
	touchedAt  time.Time
}

// FleetSummary is the three-tile header.
type FleetSummary struct {
	Total      int `json:"total"`
	Done       int `json:"done"`
	InFlight   int `json:"in_flight"`
	Active     int `json:"active"`
	Assigned   int `json:"assigned"`
	Stalled    int `json:"stalled"`
	Blocked    int `json:"blocked"`
	IdleLanes  int `json:"idle_lanes"`
	PctDone    int `json:"pct_done"`
	LanesTotal int `json:"lanes_total"`
	LanesWork  int `json:"lanes_working"`
}

// FleetSnapshot is the whole payload.
type FleetSnapshot struct {
	GeneratedAt string       `json:"generated_at"`
	Summary     FleetSummary `json:"summary"`
	Activity    []FleetEvent `json:"activity"`
	Lanes       []FleetLane  `json:"lanes"`
	// Seeded reports whether this tracker has observed a prior poll. Until it
	// has, an empty Activity list means "no baseline yet", NOT "nothing is
	// happening" — the UI must say so rather than render a confident silence.
	Seeded bool `json:"seeded"`
}

// maxFleetEvents bounds the activity ring. An unbounded feed on a long-lived
// server is a slow leak, and nobody scrolls back past a couple of hundred
// transitions anyway.
const maxFleetEvents = 200

// FleetTracker turns successive ledger snapshots into a transition feed.
//
// The router store has no durable status-transition log — Task.Timeline is a
// plan structure (day/owner/hours), not a history, and is empty in practice —
// so transitions can only be observed by diffing consecutive reads. That
// carries one honest limitation, shared with the board this replaces: restart
// the server and the feed starts empty, because the baseline is in memory.
// Seeded exists so the UI can say that instead of implying quiet.
//
// Guarded per Rule A21: the HTTP handlers read while polls write.
type FleetTracker struct {
	mu     sync.Mutex
	prev   map[string]string // agent\x00task_id -> last observed status
	events []FleetEvent      // newest first
	seeded bool
	// unroutable is the set of agents with no automated wake path, from the
	// registry. A REQUIRED constructor argument rather than a settable field:
	// as optional state it defaults to empty, every lane silently reads
	// routable, and the bug this replaces returns wearing a different shape.
	// Pass nil only when routability genuinely cannot be determined.
	unroutable map[string]bool
}

func NewFleetTracker(unroutable map[string]bool) *FleetTracker {
	if unroutable == nil {
		unroutable = map[string]bool{}
	}
	return &FleetTracker{prev: map[string]string{}, unroutable: unroutable}
}

func trackKey(agent, taskID string) string { return agent + "\x00" + taskID }

// Observe records a snapshot and returns the rendered fleet payload.
func (ft *FleetTracker) Observe(snap ledger.Snapshot, now time.Time) FleetSnapshot {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	// Non-nil slices: a nil slice marshals to `null`, and a client doing
	// `.length` on it throws. Empty must serialize as [].
	out := FleetSnapshot{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Activity:    []FleetEvent{},
		Lanes:       []FleetLane{},
	}
	seen := make(map[string]bool)

	for _, ag := range snap.Agents {
		lane := FleetLane{Agent: ag.AgentID}
		for _, t := range ag.Tasks {
			key := trackKey(ag.AgentID, t.TaskID)
			seen[key] = true
			status := strings.TrimSpace(t.Status)

			// Seed-don't-burst. On the FIRST sight of a task we record its
			// status and emit nothing. Without this every restart replays the
			// entire ledger as a burst of fake "changes" — the feed's whole
			// value is that movement in it means movement in the ledger.
			prev, known := ft.prev[key]
			switch {
			case !known:
				ft.prev[key] = status
			case prev != status:
				ft.events = append([]FleetEvent{{
					At:      now.Format("15:04:05"),
					Agent:   ag.AgentID,
					TaskID:  t.TaskID,
					Subject: t.Subject,
					From:    prev,
					To:      status,
				}}, ft.events...)
				ft.prev[key] = status
			}

			switch status {
			case "done":
				out.Summary.Done++
			case "in-progress":
				lane.Active++
				lane.Open++
				out.Summary.Active++
			case "blocked":
				lane.Blocked++
				lane.Open++
				out.Summary.Blocked++
			default: // pending and anything unrecognized: open, not started
				lane.Stalled++
				lane.Open++
				out.Summary.Stalled++
			}
			out.Summary.Total++

			if ts, err := time.Parse(time.RFC3339, t.Updated); err == nil && ts.After(lane.touchedAt) {
				lane.touchedAt = ts
			}
		}

		// Classification is delegated, and "working" now REQUIRES evidence of a
		// recent task-record mutation rather than merely having active work.
		//
		// UnmetRequirements is 0 until the canonical requirement registry lands
		// (supervision step 1/7). That is an honest under-count, not a
		// placeholder pretending to be complete: a lane can therefore read
		// COMPLETE while carrying unmet canon requirements, and it must not be
		// treated as a completion claim until the registry feeds this field.
		lane.Inbox = len(ag.Items)
		// Routability is READ, not assumed. This was hardcoded true, which made
		// StateUnroutable unreachable: the classifier could produce it, the only
		// caller forbade it, and seven lanes declaring wake.mechanism=none while
		// holding 18 open items between them all rendered as merely idle. A state
		// that no caller can produce is not a state, it is a comment.
		lane.Routable = !ft.unroutable[ag.AgentID]
		lane.State = string(supervision.Classify(supervision.LaneInput{
			Agent: ag.AgentID,
			Sources: supervision.Sources{
				OpenItems:       len(ag.Items),
				ActionableTasks: lane.Active + lane.Stalled,
				BlockedTasks:    lane.Blocked,
			},
			LastTaskMutation: lane.touchedAt,
			Routable:         lane.Routable,
		}, now, supervision.DefaultWorkWindow))
		if !lane.touchedAt.IsZero() {
			lane.TouchedAgo = ledger.FormatAge(now.Sub(lane.touchedAt).Seconds())
		}
		out.Lanes = append(out.Lanes, lane)
	}

	// Forget tasks that no longer exist, or prev grows without bound as task
	// ids churn — and a deleted-then-recreated id must re-seed rather than
	// report a transition from a status nothing currently holds.
	for k := range ft.prev {
		if !seen[k] {
			delete(ft.prev, k)
		}
	}

	if len(ft.events) > maxFleetEvents {
		ft.events = ft.events[:maxFleetEvents]
	}

	out.Summary.InFlight = out.Summary.Total - out.Summary.Done
	out.Summary.Assigned = out.Summary.Stalled
	if out.Summary.Total > 0 {
		out.Summary.PctDone = out.Summary.Done * 100 / out.Summary.Total
	}
	out.Summary.LanesTotal = len(out.Lanes)
	for _, l := range out.Lanes {
		if l.State == LaneWorking {
			out.Summary.LanesWork++
		}
		if l.Open == 0 {
			out.Summary.IdleLanes++
		}
	}

	// Ordered by ATTENTION, not by activity. IDLE_WITH_WORK and UNROUTABLE come
	// first because they are the states that need somebody to act; WORKING and
	// ASSIGNED are already moving. Sorting "working first" would bury exactly
	// the lanes this classification exists to surface.
	rank := func(st string) int {
		switch st {
		case LaneUnroutable:
			return 0
		case LaneIdleWithWork:
			return 1
		case LaneBlocked:
			return 2
		case LaneAssigned:
			return 3
		case LaneWorking:
			return 4
		default: // COMPLETE
			return 5
		}
	}
	sort.SliceStable(out.Lanes, func(i, j int) bool {
		a, b := out.Lanes[i], out.Lanes[j]
		if rank(a.State) != rank(b.State) {
			return rank(a.State) < rank(b.State)
		}
		if a.Open != b.Open {
			return a.Open > b.Open
		}
		if !a.touchedAt.Equal(b.touchedAt) {
			return a.touchedAt.After(b.touchedAt)
		}
		return a.Agent < b.Agent
	})

	out.Activity = append([]FleetEvent{}, ft.events...)
	ft.seeded = true
	out.Seeded = ft.seeded
	return out
}

// apiFleet serves GET /api/fleet.
func (s *Server) apiFleet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.cfg.FleetFn == nil {
		// Honest degrade, same contract as /api/ledger: no producer wired is a
		// 503, never an empty board that reads as "the fleet has no work".
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "fleet producer not configured"})
		return
	}
	snap, err := s.cfg.FleetFn()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s.fleet.Observe(snap, time.Now()))
}
