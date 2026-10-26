package ledger

import "sort"

// FabricFacts is computed once before any shape adapter. Raw status gauges
// and canonical task liveness are distinct from supervised lane classification.
type FabricFacts struct {
	Work     FabricWork
	Messages FabricMessages
	Lanes    map[string]FabricLaneFacts
	Ledger   BoardSummary
	Phases   []FabricPhaseFacts
}

type FabricPhaseFacts struct {
	Agent, Phase                 string
	Total, Done, Active, Blocked int
}

type FabricLaneFacts struct {
	Counts                      map[string]int
	Total, Open, Active         int
	Stalled, Blocked, Unblocked int
	Inbox                       int
	LastTouch                   string
}

func fabricFacts(s Snapshot) FabricFacts {
	f := FabricFacts{Lanes: map[string]FabricLaneFacts{}, Ledger: BoardSummary{GeneratedAt: s.GeneratedAt, Phases: []PhaseGroup{}, Blockers: []BoardBlocker{}}}
	phases := map[string]*PhaseGroup{}
	var order []string
	for _, a := range s.Agents {
		// These are known zeros from a captured lane, not an unavailable read.
		lane := FabricLaneFacts{Counts: map[string]int{"done": 0, "in-progress": 0, "pending": 0, "blocked": 0}, Inbox: len(a.Items)}
		lanePhases := map[string]*FabricPhaseFacts{}
		for _, task := range a.Tasks {
			status := task.Status
			if status == "" {
				status = "pending"
			}
			lane.Counts[status]++
			lane.Total++
			f.Work.Total++
			if task.Updated > lane.LastTouch {
				lane.LastTouch = task.Updated
			}
			if task.Status == "done" {
				f.Work.Done++
			} else {
				lane.Open++
				f.Work.Open++
				switch task.Status {
				case "blocked":
					lane.Blocked++
					f.Work.Blocked++
				case "in-progress":
					if task.Liveness == "stalled" {
						lane.Stalled++
						f.Work.Stalled++
					} else {
						lane.Active++
						f.Work.InProgress++
					}
				default:
					f.Work.AssignedNotStarted++
					if task.Liveness == "stalled" {
						lane.Stalled++
						f.Work.Stalled++
					}
				}
			}
			if (status == "pending" || status == "in-progress") && task.BlockedBy == "" {
				lane.Unblocked++
			}
			label := task.Phase
			if label == "" {
				label = "General"
			}
			if phases[label] == nil {
				phases[label] = &PhaseGroup{Name: label}
				order = append(order, label)
			}
			p := phases[label]
			p.Total++
			if task.Status == "done" {
				p.Done++
			} else {
				p.Active++
				if task.Status == "blocked" {
					p.Blocked++
				}
			}
			if lanePhases[task.Phase] == nil {
				lanePhases[task.Phase] = &FabricPhaseFacts{Agent: a.AgentID, Phase: task.Phase}
			}
			lp := lanePhases[task.Phase]
			lp.Total++
			if task.Status == "done" {
				lp.Done++
			} else {
				lp.Active++
				if task.Status == "blocked" {
					lp.Blocked++
				}
			}
		}
		for _, item := range a.Items {
			f.Messages.Open++
			if item.AgeSeconds > f.Messages.OldestAgeSeconds {
				f.Messages.OldestAgeSeconds = item.AgeSeconds
			}
			if item.Blocked {
				f.Ledger.BlockedItems++
				f.Ledger.Blockers = append(f.Ledger.Blockers, BoardBlocker{Agent: a.AgentID, ItemID: item.ID, Title: item.Title, Age: FormatAge(item.AgeSeconds)})
			}
		}
		f.Lanes[a.AgentID] = lane
		labels := make([]string, 0, len(lanePhases))
		for label := range lanePhases {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			f.Phases = append(f.Phases, *lanePhases[label])
		}
	}
	f.Ledger.TotalTasks, f.Ledger.DoneTasks = f.Work.Total, f.Work.Done
	// The documented active bucket includes blocked; consumers subtract its
	// blocked subset when drawing independent segments, never add it twice.
	f.Ledger.ActiveTasks, f.Ledger.BlockedTasks = f.Work.Open, f.Work.Blocked
	f.Ledger.OpenItems = f.Messages.Open
	if f.Work.Total > 0 {
		f.Ledger.PctDone = f.Work.Done * 100 / f.Work.Total
	}
	for _, name := range order {
		p := phases[name]
		if p.Total > 0 {
			p.PctDone = p.Done * 100 / p.Total
		}
		f.Ledger.Phases = append(f.Ledger.Phases, *p)
	}
	return f
}

// IdleFabricLanes compares canonical enum values, not display labels or ages.
func IdleFabricLanes(lanes []FabricLane) int {
	n := 0
	for _, lane := range lanes {
		if lane.State == "IDLE_WITH_WORK" {
			n++
		}
	}
	return n
}

// Stable task order is also used by the existing legacy phase/detail adapter.
func (s Snapshot) OrderedAgents() []Agent {
	agents := append([]Agent(nil), s.Agents...)
	sort.Slice(agents, func(i, j int) bool { return agents[i].AgentID < agents[j].AgentID })
	return agents
}
