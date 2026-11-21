package dashboard

import "github.com/SirsiMaster/sirsi-pantheon/internal/ledger"

// The hosted board's ingress grants access to the board, not to individual
// router messages. Keep the shared wire shape separate from RouterSnapshot so
// adding a field to a local snapshot does not publish it to hosted viewers.
type hostedRouterSummary struct {
	GeneratedAt      string               `json:"generated_at"`
	BuiltMs          int64                `json:"built_ms"`
	Version          string               `json:"version"`
	Lanes            hostedRouterLanes    `json:"lanes"`
	Queue            []hostedRouterQueue  `json:"queue"`
	PrivateWithheld  bool                 `json:"private_details_withheld"`
	Fabric           *hostedFabricSummary `json:"fabric,omitempty"`
	FabricGeneration string               `json:"fabric_generation,omitempty"`
}

type hostedRouterLanes struct {
	Counts map[string]int     `json:"counts"`
	List   []hostedRouterLane `json:"list"`
}

type hostedRouterLane struct {
	Agent   string `json:"agent"`
	Verdict string `json:"verdict"`
	Open    int    `json:"open"`
}

type hostedRouterQueue struct {
	Agent string `json:"agent"`
	Open  int    `json:"open"`
}

func hostedRouterVerdict(value string) bool {
	switch value {
	case "LIVE", "WAKEABLE", "HELD", "AUTH_REQUIRED", "WATCH_ONLY", "UNSTAFFED", "UNREACHABLE", "PROVIDER_QUOTA":
		return true
	default:
		return false
	}
}

func (s *Server) routerSnapshotResponse(snap RouterSnapshot) any {
	if !s.cfg.CloudBoard {
		return snap
	}
	out := hostedRouterSummary{
		GeneratedAt: snap.GeneratedAt, BuiltMs: snap.BuiltMs, Version: snap.Version,
		Lanes: hostedRouterLanes{Counts: map[string]int{}, List: []hostedRouterLane{}},
		Queue: []hostedRouterQueue{}, PrivateWithheld: true,
	}
	for verdict, count := range snap.Lanes.Counts {
		if hostedRouterVerdict(verdict) {
			out.Lanes.Counts[verdict] = count
		}
	}
	for _, lane := range snap.Lanes.List {
		verdict := lane.Verdict
		if !hostedRouterVerdict(verdict) {
			verdict = "UNKNOWN"
		}
		out.Lanes.List = append(out.Lanes.List, hostedRouterLane{Agent: lane.Agent, Verdict: verdict, Open: lane.Open})
	}
	for _, row := range snap.Queue {
		out.Queue = append(out.Queue, hostedRouterQueue(row))
	}
	if snap.Fabric != nil {
		fabric := projectHostedFabric(*snap.Fabric)
		out.Fabric = &fabric
		out.FabricGeneration = snap.FabricGeneration
	}
	return out
}

// Retain aggregate work, inbox and per-lane counts, not task rows or nested
// runtime, health or activity diagnostics. Empty activity/health collections
// are compatibility fields, explicitly labeled as withheld by the UI.
type hostedFabricSummary struct {
	Work             ledger.FabricWork     `json:"work"`
	Messages         ledger.FabricMessages `json:"messages"`
	Lanes            []hostedFabricLane    `json:"lanes"`
	Health           hostedFabricHealth    `json:"health"`
	Activity         []struct{}            `json:"activity"`
	ActivitySeeded   bool                  `json:"activity_seeded"`
	RuntimeAvailable bool                  `json:"runtime_available"`
	Build            string                `json:"build"`
	GeneratedAt      string                `json:"generated_at"`
	PrivateWithheld  bool                  `json:"private_details_withheld"`
}

type hostedFabricHealth struct {
	Engine      string   `json:"engine"`
	EngineState string   `json:"engine_state"`
	Issues      []string `json:"issues"`
}

type hostedFabricLane struct {
	Agent   string `json:"agent"`
	Open    int    `json:"open"`
	Active  int    `json:"active"`
	Stalled int    `json:"stalled"`
	Blocked int    `json:"blocked"`
}

func projectHostedFabric(board ledger.FabricBoard) hostedFabricSummary {
	out := hostedFabricSummary{
		Work: board.Work, Messages: board.Messages, Lanes: []hostedFabricLane{},
		Health:   hostedFabricHealth{Engine: "withheld", EngineState: "withheld", Issues: []string{}},
		Activity: []struct{}{}, Build: board.Build, GeneratedAt: board.GeneratedAt,
		PrivateWithheld: true,
	}
	for _, lane := range board.Lanes {
		out.Lanes = append(out.Lanes, hostedFabricLane{Agent: lane.Agent, Open: lane.Open, Active: lane.Active, Stalled: lane.Stalled, Blocked: lane.Blocked})
	}
	return out
}

func (s *Server) fabricResponse(board ledger.FabricBoard) any {
	if s.cfg.CloudBoard {
		return projectHostedFabric(board)
	}
	return board
}
