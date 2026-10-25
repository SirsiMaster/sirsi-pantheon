package ledger

import (
	"fmt"
	"sort"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// FabricBoard is the single cross-surface contract. Work and messages remain
// separate streams so no renderer can put two different meanings behind one
// headline number.
type FabricBoard struct {
	Work             FabricWork       `json:"work"`
	Messages         FabricMessages   `json:"messages"`
	Lanes            []FabricLane     `json:"lanes"`
	Health           FabricHealth     `json:"health"`
	Activity         []FabricActivity `json:"activity"`
	ActivitySeeded   bool             `json:"activity_seeded"`
	RuntimeAvailable bool             `json:"runtime_available"`
	Build            string           `json:"build"`
	GeneratedAt      string           `json:"generated_at"`
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
	Agent      string                            `json:"agent"`
	State      string                            `json:"state"`
	StateLabel string                            `json:"state_label"`
	Runtime    *routerstore.LaneOperationalState `json:"runtime,omitempty"`
	Open       int                               `json:"open"`
	Active     int                               `json:"active"`
	Stalled    int                               `json:"stalled"`
	Blocked    int                               `json:"blocked"`
	LastTouch  string                            `json:"last_touch,omitempty"`
	Registered FabricRegistration                `json:"registered"`
}

type FabricHealth struct {
	RAMPct      *float64 `json:"ram_pct"`
	Swap        *int64   `json:"swap"`
	GitDirty    *bool    `json:"git_dirty"`
	Engine      string   `json:"engine"`
	Issues      []string `json:"issues"`
	TotalRAM    *int64   `json:"total_ram"`
	CPUCores    *int     `json:"cpu_cores"`
	EngineState string   `json:"engine_state"`
	ObservedAt  string   `json:"observed_at,omitempty"`
	RAMSource   string   `json:"ram_source,omitempty"`
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
// FabricEvidence is already-read authority, not a second store/policy. Missing
// operational evidence is UNKNOWN, never status/heartbeat-derived WORKING.
type FabricEvidence struct {
	Operational    map[string]routerstore.LaneOperationalState
	Health         *FabricHealth
	Activity       []FabricActivity
	ActivitySeeded bool
}
type FabricRead struct {
	Snapshot Snapshot
	Node     *router.NodeStatus
	Evidence FabricEvidence
}

func FabricFrom(s Snapshot, ns *router.NodeStatus, build string, inputs ...FabricEvidence) FabricBoard {
	var evidence FabricEvidence
	if len(inputs) == 1 {
		evidence = inputs[0]
	}
	b := FabricBoard{
		Build:            build,
		GeneratedAt:      s.GeneratedAt,
		Health:           FabricHealth{Engine: "unknown", EngineState: "unavailable", Issues: []string{}},
		RuntimeAvailable: evidence.Operational != nil,
		ActivitySeeded:   evidence.ActivitySeeded,
		Activity:         []FabricActivity{},
		Lanes:            []FabricLane{},
	}
	if evidence.Health != nil {
		b.Health = *evidence.Health
		b.Health.Issues = append([]string{}, evidence.Health.Issues...)
	}
	b.Activity = append(b.Activity, evidence.Activity...)
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
		lane.State = "UNKNOWN"
		validRuntime := false
		if state, ok := evidence.Operational[a.AgentID]; ok && state.Agent == a.AgentID {
			switch state.Classification {
			case routerstore.LaneWorking, routerstore.LaneAssigned, routerstore.LaneIdleWithWork, routerstore.LaneBlocked, routerstore.LaneUnroutable, routerstore.LaneComplete:
				validRuntime = true
				lane.State = state.Classification
				copied := state
				lane.Runtime = &copied
			}
		}
		if !validRuntime {
			b.RuntimeAvailable = false
		}

		lane.StateLabel = lane.State
		if lane.State == routerstore.LaneIdleWithWork {
			lane.StateLabel = "IDLE with work"
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
// ReadFabric reads existing canonical ledger/node/runtime authorities only.
// In particular it never invokes SuperviseOnce or its reconcile/wake writes.
func ReadFabric(repoRoot string, now time.Time) (FabricRead, error) {
	snap, err := Build(repoRoot, "", now, DefaultStaleAfter)
	if err != nil {
		return FabricRead{}, err
	}
	ns, err := router.CollectNodeStatus(repoRoot, nil)
	if err != nil {
		return FabricRead{}, err
	}
	if ns == nil {
		return FabricRead{}, fmt.Errorf("fabric node status unavailable")
	}
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		return FabricRead{}, err
	}
	defer f.Close()
	unavailable, err := router.ReadLaneRoutability(repoRoot)
	if err != nil {
		return FabricRead{}, err
	}
	ids, err := f.Store().OperationalAgents()
	if err != nil {
		return FabricRead{}, err
	}
	seen := map[string]bool{}
	for _, agent := range snap.Agents {
		seen[agent.AgentID] = true
	}
	for _, id := range append(ids, ns.RegisteredAgents...) {
		if !seen[id] {
			snap.Agents = append(snap.Agents, Agent{AgentID: id})
			seen[id] = true
		}
	}
	operational := map[string]routerstore.LaneOperationalState{}
	for _, agent := range snap.Agents {
		blocked, declared := unavailable[agent.AgentID]
		value, err := f.Store().ClassifyLane(agent.AgentID, declared && !blocked, 5*time.Minute)
		if err != nil {
			return FabricRead{}, fmt.Errorf("fabric runtime unavailable: %w", err)
		}
		operational[agent.AgentID] = value
	}
	return FabricRead{Snapshot: snap, Node: ns, Evidence: FabricEvidence{Operational: operational}}, nil
}
func BuildFabric(repoRoot, build string, now time.Time) (FabricBoard, error) {
	read, err := ReadFabric(repoRoot, now)
	if err != nil {
		return FabricBoard{}, err
	}
	return FabricFrom(read.Snapshot, read.Node, build, read.Evidence), nil
}
