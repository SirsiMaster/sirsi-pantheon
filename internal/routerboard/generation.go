package routerboard

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

// FromGeneration is a read-only legacy shape adapter. It has no reader, store,
// command, clock or actor-authority dependency. All common gauges come from the
// sealed producer facts, while existing phase/detail fields keep their scope.
func FromGeneration(generation *ledger.FabricGeneration, uiBuild string) (Payload, error) {
	v, err := generation.View()
	if err != nil {
		return Payload{}, err
	}
	at, err := time.Parse(time.RFC3339Nano, v.Read.Snapshot.GeneratedAt)
	if err != nil {
		return Payload{}, err
	}
	p := Payload{Build: uiBuild, GeneratedAt: v.Read.Snapshot.GeneratedAt, Generation: v.ID, Fabric: &v.Board,
		DataErrors: append([]string{}, v.Read.Metadata.Errors...), Activity: []Event{}, Fleet: []Lane{}, Threads: []Thread{}, RegistrationGaps: []string{}, Tasks: []TaskDetail{}}
	if v.Read.Node == nil {
		p.DataErrors = append(p.DataErrors, "node-status-unavailable")
	}
	if v.Read.Metadata.Lanes == nil {
		p.DataErrors = append(p.DataErrors, "registry-metadata-unavailable")
	}
	canonical := map[string]ledger.FabricLane{}
	for _, lane := range v.Board.Lanes {
		canonical[lane.Agent] = lane
	}
	consumers := map[string]*Consumer{}
	if v.Read.Node != nil {
		for _, t := range v.Read.Node.LiveThreads {
			c := consumers[t.AgentID]
			if c == nil {
				c = &Consumer{}
				consumers[t.AgentID] = c
			}
			c.Armed = c.Armed || t.Armed
			if t.LoopState != "" {
				c.Loop = t.LoopState
			}
		}
		for _, t := range v.Read.Node.StaleThreads {
			if consumers[t.AgentID] == nil {
				consumers[t.AgentID] = &Consumer{Loop: "unknown"}
			}
		}
	}
	for _, t := range v.Read.Metadata.Threads {
		p.Threads = append(p.Threads, Thread{ThreadID: t.ID, Agent: t.Agent, Workstream: t.Workstream, IdleSeconds: t.IdleSeconds, Stale: t.Stale})
	}
	for _, a := range v.Read.Snapshot.OrderedAgents() {
		f := v.Facts.Lanes[a.AgentID]
		c := canonical[a.AgentID]
		meta := v.Read.Metadata.Lanes[a.AgentID]
		kind := meta.Type
		if kind == "" {
			kind = "?"
		}
		wake := "unknown"
		if meta.WakeMechanism == "none" || meta.WakeMechanism == "" && kind != "?" {
			wake = "n/a"
		}
		activity := "unknown"
		switch c.State {
		case "WORKING":
			activity = "active now"
		case "ASSIGNED":
			activity = "recently active"
		case "IDLE_WITH_WORK", "BLOCKED", "UNROUTABLE", "COMPLETE":
			activity = "quiet"
		}
		lane := Lane{Agent: a.AgentID, Type: kind, WakeState: wake, Activity: activity, State: c.State, Role: c.Role, TaskOpen: c.Open, Active: c.Active, Stalled: c.Stalled, BlockedTasks: c.Blocked,
			OpenItems: f.Inbox, Items: []Item{}, TasksTotal: f.Total, Counts: f.Counts, UnblockedOpen: f.Unblocked,
			IdleWithWork: c.State == "IDLE_WITH_WORK", Registered: Registered{Router: c.Registered.Router, Thread: c.Registered.Thread, Ledger: c.Registered.Ledger}, LastTouch: f.LastTouch, Tasks: []rawTask{}, Consumer: consumers[a.AgentID]}
		lane.RegisteredKnown = v.Read.Node != nil
		for i, item := range a.Items {
			if item.Stale {
				lane.OldestMsgStale = true
			}
			if i >= 60 {
				continue
			}
			age := item.AgeSeconds
			lane.Items = append(lane.Items, Item{ID: item.ID, Title: item.Title, From: item.From, Type: item.Type, AgeSeconds: &age, Opened: item.Opened, Picked: item.Picked, Blocked: item.Blocked, Stale: item.Stale})
		}
		for _, task := range a.Tasks {
			encoded, e := json.Marshal(task)
			if e != nil {
				return Payload{}, e
			}
			var raw rawTask
			if e = json.Unmarshal(encoded, &raw); e != nil {
				return Payload{}, e
			}
			lane.Tasks = append(lane.Tasks, raw)
		}
		p.Fleet = append(p.Fleet, lane)
		if kind != "service" && kind != "human" && (!c.Registered.Thread || !c.Registered.Ledger) {
			p.RegistrationGaps = append(p.RegistrationGaps, a.AgentID)
		}
		p.Counters.InProgressNow += f.Counts["in-progress"]
		p.Counters.Pending += f.Counts["pending"]
	}
	sort.Slice(p.Fleet, func(i, j int) bool {
		if p.Fleet[i].OpenItems != p.Fleet[j].OpenItems {
			return p.Fleet[i].OpenItems > p.Fleet[j].OpenItems
		}
		return p.Fleet[i].Agent < p.Fleet[j].Agent
	})
	p.Counters.Completed, p.Counters.Total, p.Counters.Done, p.Counters.Blocked = v.Completed, v.Board.Work.Total, v.Board.Work.Done, v.Board.Work.Blocked
	for i, event := range v.Events {
		if i >= 40 {
			break
		}
		p.Activity = append(p.Activity, Event{At: event.At, Agent: event.Agent, TaskID: event.TaskID, Subject: event.Subject, From: event.From, To: event.To})
	}
	p.Board = legacySummary(v, at)
	p.Ledger = p.Board
	p.Tasks = taskDetailsAt(p.Fleet, at)
	return p, nil
}

func legacySummary(v ledger.FabricGenerationView, at time.Time) BoardSummary {
	w := v.Board.Work
	s := BoardSummary{TotalTasks: w.Total, DoneTasks: w.Done, ActiveTasks: w.Open, BlockedTasks: w.Blocked, PctDone: pct(w.Done, w.Total), OpenItems: v.Board.Messages.Open,
		Phases: []Phase{}, Blockers: []Blocker{}, BlockedItems: w.Blocked}
	phases := map[string]*Phase{}
	for _, f := range v.Facts.Phases {
		label := f.Phase
		if label == "" {
			label = f.Agent + " (no phase set)"
		}
		if phases[label] == nil {
			phases[label] = &Phase{Name: label}
		}
		p := phases[label]
		p.Total += f.Total
		p.Done += f.Done
		p.Active += f.Active
		p.Blocked += f.Blocked
	}
	for _, p := range phases {
		p.PctDone = pct(p.Done, p.Total)
		s.Phases = append(s.Phases, *p)
	}
	sort.Slice(s.Phases, func(i, j int) bool {
		if s.Phases[i].Total != s.Phases[j].Total {
			return s.Phases[i].Total > s.Phases[j].Total
		}
		return s.Phases[i].Name < s.Phases[j].Name
	})
	for _, a := range v.Read.Snapshot.OrderedAgents() {
		for _, t := range a.Tasks {
			if t.Status != "blocked" || len(s.Blockers) >= 20 {
				continue
			}
			title := t.BlockedBy
			if title == "" {
				title = t.Subject
			}
			if len(title) > 120 {
				title = title[:120]
			}
			s.Blockers = append(s.Blockers, Blocker{Agent: a.AgentID, ItemID: t.TaskID, Title: title, Age: ageAt(t.Updated, at)})
		}
	}
	return s
}

func taskDetailsAt(fleet []Lane, at time.Time) []TaskDetail {
	out := []TaskDetail{}
	for _, lane := range fleet {
		for _, t := range lane.Tasks {
			var blocked *string
			if value := t.str("blocked_by"); value != "" {
				blocked = &value
			}
			liveness := t.str("liveness")
			switch liveness {
			case "active", "stalled", "blocked", "unknown":
			default:
				liveness = "unknown"
			}
			out = append(out, TaskDetail{TaskID: t.str("task_id"), Agent: lane.Agent, Subject: t.str("subject"), Status: t.str("status"), Phase: t.str("phase"), BlockedBy: blocked, ResponsibleParty: t["responsible_party"], Updated: t.str("updated"), Age: ageAt(t.str("updated"), at), Liveness: liveness,
				Charter: t["charter"], CommissionedAt: t["commissioned_at"], CommissionedBy: t["commissioned_by"], Outline: t["outline"], Timeline: sliceOf(t["timeline"]), Links: sliceOf(t["links"]), TestState: strOr(t["test_state"], "untested"), Stage: strOr(t["stage"], "spec"), TokensConsumed: intOr(t["tokens_consumed"]), DurationSeconds: intOr(t["duration_seconds"])})
		}
	}
	return out
}

func ageAt(value string, at time.Time) string {
	updated, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "?"
	}
	seconds := at.Sub(updated).Seconds()
	if seconds < 0 {
		seconds = 0
	}
	days := int(seconds) / 86400
	hours := (int(seconds) % 86400) / 3600
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	return fmt.Sprintf("%dh%dm", hours, (int(seconds)%3600)/60)
}
