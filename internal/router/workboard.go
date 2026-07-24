// Package router — workboard.go
//
// The Work Board (owner directive 2026-07-17): every agent sees its work
// packages (open items WITH titles, age, sender), its peers (which agents are
// live beside it and on what surfaces), and the PACE of the fabric (closed
// today / this week, average time-to-close). All derived from the two sources
// of truth that already exist — the items corpus (open + closed, 90-day
// retention) and the thread registry — computed on read, stored nowhere.
package router

import (
	"sort"
	"time"
)

// WorkPackage is one open item as an agent sees it on the board.
type WorkPackage struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	From     string  `json:"from"`
	AgeHours float64 `json:"age_hours"`
}

// AgentBoard is one agent's full picture: packages, liveness, and pace.
type AgentBoard struct {
	AgentID     string        `json:"agent_id"`
	Open        []WorkPackage `json:"open"`
	Live        bool          `json:"live"`     // ≥1 live (non-stale) thread
	Surfaces    []string      `json:"surfaces"` // live surfaces (worker/claude/menubar…)
	ClosedToday int           `json:"closed_today"`
	Closed7d    int           `json:"closed_7d"`
	// AvgCloseHours is the mean open→close time over the 7-day window; 0 when
	// nothing closed. Pace you can feel: "this agent turns work around in Nh".
	AvgCloseHours float64 `json:"avg_close_hours"`
}

// WorkBoard is the whole fabric at a glance.
type WorkBoard struct {
	GeneratedAt   time.Time    `json:"generated_at"`
	Agents        []AgentBoard `json:"agents"` // sorted: most open work first
	TotalOpen     int          `json:"total_open"`
	ClosedToday   int          `json:"closed_today"`
	Closed7d      int          `json:"closed_7d"`
	AvgCloseHours float64      `json:"avg_close_hours"`
}

// ComputeWorkBoard builds the board from the items corpus + thread registry.
func ComputeWorkBoard(routerRoot string) (*WorkBoard, error) {
	items, err := AllItems(routerRoot)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	dayAgo, weekAgo := now.Add(-24*time.Hour), now.Add(-7*24*time.Hour)

	type acc struct {
		open        []WorkPackage
		closedToday int
		closed7d    int
		closeHours  []float64
	}
	agents := map[string]*acc{}
	get := func(id string) *acc {
		if agents[id] == nil {
			agents[id] = &acc{}
		}
		return agents[id]
	}

	board := &WorkBoard{GeneratedAt: now}
	var allCloseHours []float64
	for _, it := range items {
		a := get(it.To)
		switch it.Status {
		case "open":
			age := 0.0
			if t, perr := time.Parse(time.RFC3339, it.Opened); perr == nil {
				age = now.Sub(t).Hours()
			}
			a.open = append(a.open, WorkPackage{ID: it.ID, Title: it.Title, From: it.From, AgeHours: age})
			board.TotalOpen++
		case "closed":
			closed, cerr := time.Parse(time.RFC3339, it.Closed)
			if cerr != nil || closed.Before(weekAgo) {
				continue
			}
			a.closed7d++
			board.Closed7d++
			if closed.After(dayAgo) {
				a.closedToday++
				board.ClosedToday++
			}
			if opened, oerr := time.Parse(time.RFC3339, it.Opened); oerr == nil && closed.After(opened) {
				h := closed.Sub(opened).Hours()
				a.closeHours = append(a.closeHours, h)
				allCloseHours = append(allCloseHours, h)
			}
		}
	}

	// Liveness + surfaces from the thread registry (best-effort: a missing
	// registry still yields a board — packages and pace stand on their own).
	liveSurfaces := map[string][]string{}
	if reg, rerr := LoadThreadRegistry(routerRoot); rerr == nil {
		for _, t := range reg.Threads {
			if t == nil || t.Status.IsTerminal() || t.Status == ThreadStatusSuspended {
				continue
			}
			if t.IsStale(now, DefaultThreadStaleAfter) {
				continue
			}
			found := false
			for _, s := range liveSurfaces[t.AgentID] {
				if s == t.Surface {
					found = true
					break
				}
			}
			if !found {
				liveSurfaces[t.AgentID] = append(liveSurfaces[t.AgentID], t.Surface)
			}
		}
	}
	for id := range liveSurfaces {
		_ = get(id) // a live agent with zero items still appears — it IS a peer
	}

	mean := func(v []float64) float64 {
		if len(v) == 0 {
			return 0
		}
		s := 0.0
		for _, x := range v {
			s += x
		}
		return s / float64(len(v))
	}
	board.AvgCloseHours = mean(allCloseHours)

	for id, a := range agents {
		sort.Slice(a.open, func(i, j int) bool { return a.open[i].AgeHours > a.open[j].AgeHours })
		surfaces := append([]string(nil), liveSurfaces[id]...)
		sort.Strings(surfaces)
		board.Agents = append(board.Agents, AgentBoard{
			AgentID: id, Open: a.open,
			Live: len(surfaces) > 0, Surfaces: surfaces,
			ClosedToday: a.closedToday, Closed7d: a.closed7d,
			AvgCloseHours: mean(a.closeHours),
		})
	}
	sort.Slice(board.Agents, func(i, j int) bool {
		if len(board.Agents[i].Open) != len(board.Agents[j].Open) {
			return len(board.Agents[i].Open) > len(board.Agents[j].Open)
		}
		return board.Agents[i].AgentID < board.Agents[j].AgentID
	})
	return board, nil
}
