package ledger

import (
	"sort"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// FabricBoard is the single cross-surface contract. Work and messages remain
// separate streams so no renderer can put two different meanings behind one
// headline number.
type FabricBoard struct {
	Work        FabricWork       `json:"work"`
	Messages    FabricMessages   `json:"messages"`
	Lanes       []FabricLane     `json:"lanes"`
	Health      FabricHealth     `json:"health"`
	Activity    []FabricActivity `json:"activity"`
	Build       string           `json:"build"`
	GeneratedAt string           `json:"generated_at"`
}

type FabricWork struct {
	Done               int `json:"done"`
	Open               int `json:"open"`
	InProgress         int `json:"in_progress"`
	AssignedNotStarted int `json:"assigned_not_started"`
	Stalled            int `json:"stalled"`
	Blocked            int `json:"blocked"`
	Total              int `json:"total"`
}

type FabricMessages struct {
	Open             int     `json:"open"`
	OldestAgeSeconds float64 `json:"oldest_age_seconds"`
	Stranded         int     `json:"stranded"`
}

type FabricRegistration struct {
	Router bool `json:"router"`
	Thread bool `json:"thread"`
	Ledger bool `json:"ledger"`
}

type FabricLane struct {
	Agent      string             `json:"agent"`
	State      string             `json:"state"`
	Open       int                `json:"open"`
	Active     int                `json:"active"`
	Stalled    int                `json:"stalled"`
	Blocked    int                `json:"blocked"`
	LastTouch  string             `json:"last_touch,omitempty"`
	Registered FabricRegistration `json:"registered"`
}

type FabricHealth struct {
	RAMPct   *float64 `json:"ram_pct"`
	Swap     *int64   `json:"swap"`
	GitDirty *bool    `json:"git_dirty"`
	Engine   string   `json:"engine"`
	Issues   []string `json:"issues"`
}

type FabricActivity struct {
	At     string `json:"at"`
	Agent  string `json:"agent"`
	TaskID string `json:"task_id"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// FabricFrom derives all task/message/lane values once for every renderer.
// Unknown host-health values are nil, never fabricated zeroes.
func FabricFrom(s Snapshot, ns *router.NodeStatus, build string) FabricBoard {
	b := FabricBoard{
		Build:       build,
		GeneratedAt: s.GeneratedAt,
		Health:      FabricHealth{Engine: "unknown", Issues: []string{}},
		Activity:    []FabricActivity{},
	}
	registered := map[string]bool{}
	threaded := map[string]bool{}
	if ns != nil {
		for _, id := range ns.RegisteredAgents {
			registered[id] = true
		}
		for _, t := range ns.LiveThreads {
			threaded[t.AgentID] = true
		}
		for _, t := range ns.StaleThreads {
			threaded[t.AgentID] = true
			b.Health.Issues = append(b.Health.Issues, "stale thread: "+t.ThreadID)
		}
		for _, lane := range ns.StrandedInbox {
			b.Messages.Stranded += lane.OpenItems
		}
	}
	for _, a := range s.Agents {
		lane := FabricLane{Agent: a.AgentID, Registered: FabricRegistration{Router: registered[a.AgentID], Thread: threaded[a.AgentID], Ledger: len(a.Tasks) > 0}}
		for _, item := range a.Items {
			b.Messages.Open++
			if item.AgeSeconds > b.Messages.OldestAgeSeconds {
				b.Messages.OldestAgeSeconds = item.AgeSeconds
			}
		}
		for _, task := range a.Tasks {
			b.Work.Total++
			if task.Updated > lane.LastTouch {
				lane.LastTouch = task.Updated
			}
			switch task.Status {
			case "done":
				b.Work.Done++
			case "blocked":
				b.Work.Open++
				b.Work.Blocked++
				lane.Open++
				lane.Blocked++
			case "in-progress":
				b.Work.Open++
				lane.Open++
				if task.Liveness == "stalled" {
					b.Work.Stalled++
					lane.Stalled++
				} else {
					b.Work.InProgress++
					lane.Active++
				}
			default:
				b.Work.Open++
				b.Work.AssignedNotStarted++
				lane.Open++
				if task.Liveness == "stalled" {
					b.Work.Stalled++
					lane.Stalled++
				}
			}
		}
		switch {
		case lane.Active > 0:
			lane.State = "WORKING"
		case lane.Stalled > 0:
			lane.State = "IDLE with work"
		case lane.Blocked > 0:
			lane.State = "BLOCKED"
		case lane.Open > 0:
			lane.State = "ASSIGNED"
		default:
			lane.State = "IDLE"
		}
		b.Lanes = append(b.Lanes, lane)
	}
	sort.Slice(b.Lanes, func(i, j int) bool { return b.Lanes[i].Agent < b.Lanes[j].Agent })
	if b.GeneratedAt == "" {
		b.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return b
}

// BuildFabric joins the canonical ledger and node-status producers. Callers
// receive an error instead of a zero-valued board when either producer is
// unavailable, allowing surfaces to retain and label their last good payload.
func BuildFabric(repoRoot, build string, now time.Time) (FabricBoard, error) {
	snap, err := Build(repoRoot, "", now, DefaultStaleAfter)
	if err != nil {
		return FabricBoard{}, err
	}
	ns, err := router.CollectNodeStatus(repoRoot, nil)
	if err != nil {
		return FabricBoard{}, err
	}
	return FabricFrom(snap, ns, build), nil
}
