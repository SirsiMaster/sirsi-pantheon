package routerboard

import "encoding/json"

// FleetShape projects the board payload into the {summary, lanes} shape the
// menubar decodes.
//
// A PROJECTION, not a second computation. The menubar previously called
// `router fleet --json`, which counts differently from the board — the board
// treats blocked as a SUBSET of active, while the fleet summary reports them as
// separate tallies. Two careful aggregations still disagree, and the owner was
// shown three surfaces reporting three different numbers under interchangeable
// labels. Everything below is derived from one Payload, so the menubar cannot
// drift from the board it is meant to mirror.
type FleetShape struct {
	Summary FleetShapeSummary `json:"summary"`
	Lanes   []FleetShapeLane  `json:"lanes"`
}

type FleetShapeSummary struct {
	Total        int     `json:"total"`
	Done         int     `json:"done"`
	InFlight     int     `json:"in_flight"`
	Active       int     `json:"active"`
	Assigned     int     `json:"assigned"`
	Stalled      int     `json:"stalled"`
	Blocked      int     `json:"blocked"`
	IdleLanes    int     `json:"idle_lanes"`
	PctDone      int     `json:"pct_done"`
	LanesTotal   int     `json:"lanes_total"`
	LanesWorking int     `json:"lanes_working"`
	OpenItems    int     `json:"open_items"`
	PctDoneExact float64 `json:"pct_done_exact"`
}

type FleetShapeLane struct {
	Agent   string `json:"agent"`
	State   string `json:"state"`
	Open    int    `json:"open"`
	Active  int    `json:"active"`
	Stalled int    `json:"stalled"`
	Blocked int    `json:"blocked"`
	Inbox   int    `json:"inbox"`
	// TouchedAgo is the age of the lane's LAST LEDGER UPDATE — the last time it
	// mutated a task record. It is bookkeeping, not liveness: a lane doing real
	// work without recording it reads as stale here, correctly. Surfaces label it
	// "last ledger update" rather than "touched", which invited reading a
	// recordkeeping figure as a heartbeat.
	TouchedAgo string `json:"touched_ago,omitempty"`
}

// ToFleetShape derives the menubar's view from the board payload.
func ToFleetShape(p Payload) FleetShape {
	out := FleetShape{
		Summary: FleetShapeSummary{
			Total:        p.Board.TotalTasks,
			Done:         p.Board.DoneTasks,
			InFlight:     p.Board.TotalTasks - p.Board.DoneTasks,
			Active:       p.Counters.InProgressNow,
			Assigned:     p.Counters.Pending,
			Stalled:      p.Counters.Pending,
			Blocked:      p.Board.BlockedTasks,
			OpenItems:    p.Board.OpenItems,
			LanesTotal:   len(p.Fleet),
			PctDone:      int(p.Board.PctDone + 0.5),
			PctDoneExact: p.Board.PctDone,
		},
		Lanes: []FleetShapeLane{},
	}
	for _, f := range p.Fleet {
		// State names match internal/supervision so the menubar, the board, and
		// the supervisor all use one vocabulary. Derived from the SAME lane
		// fields the board renders — never recomputed from the store.
		state := "IDLE"
		switch {
		case f.IdleWithWork:
			state = "IDLE_WITH_WORK"
		case f.Activity == "active now":
			state = "WORKING"
		case f.TasksTotal == 0 && f.OpenItems == 0:
			state = "NO_WORK"
		case f.Counts["blocked"] > 0 && f.UnblockedOpen == 0:
			state = "BLOCKED"
		}
		if f.IdleWithWork {
			out.Summary.IdleLanes++
		}
		if state == "WORKING" {
			out.Summary.LanesWorking++
		}
		out.Lanes = append(out.Lanes, FleetShapeLane{
			Agent: f.Agent, State: state,
			Open:    f.Counts["in-progress"] + f.Counts["pending"] + f.Counts["blocked"],
			Active:  f.Counts["in-progress"],
			Stalled: f.Counts["pending"],
			Blocked: f.Counts["blocked"],
			Inbox:   f.OpenItems,
			TouchedAgo: func() string {
				if f.LastTouch == "" {
					return ""
				}
				return ageStr(f.LastTouch)
			}(),
		})
	}
	return out
}

// SnapshotFleetShape returns the menubar projection of the current payload.
// version 0 means no poll has completed; the caller must not print zeros.
func (b *Board) SnapshotFleetShape() ([]byte, uint64, error) {
	body, version := b.Snapshot()
	if version == 0 || len(body) == 0 {
		return nil, 0, nil
	}
	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, version, err
	}
	out, err := json.Marshal(ToFleetShape(p))
	return out, version, err
}
