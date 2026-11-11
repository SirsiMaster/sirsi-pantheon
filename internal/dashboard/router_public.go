package dashboard

import "github.com/SirsiMaster/sirsi-pantheon/internal/ledger"

// The hosted board's audience is not the local operator. This is an explicit
// wire allowlist, not a RouterSnapshot with selected secrets zeroed: new local
// fields must never become hosted fields automatically. Fabric is already the
// canonical public projection; its redaction and task-reference rules stay put.
type publicRouterSnapshot struct {
	Fabric           *ledger.FabricBoard `json:"fabric,omitempty"`
	FabricGeneration string              `json:"fabric_generation,omitempty"`
	GeneratedAt      string              `json:"generated_at"`
	BuiltMs          int64               `json:"built_ms"`
	Version          string              `json:"version"`
	Lanes            publicRouterLanes   `json:"lanes"`
	Queue            []publicRouterQueue `json:"queue"`
	Items            []publicRouterItem  `json:"items"`
	Consumers        publicConsumers     `json:"consumers"`
	Registry         publicRegistry      `json:"registry"`
	KnownFailures    []publicKnownFail   `json:"known_failures"`
	Swap             *publicRouterSwap   `json:"swap,omitempty"`
	Releases         []publicRelease     `json:"releases"`
	Attention        []publicAttention   `json:"attention"`
}

type publicRouterLanes struct {
	Counts map[string]int     `json:"counts"`
	List   []publicRouterLane `json:"list"`
}

type publicRouterLane struct {
	Agent        string `json:"agent"`
	Verdict      string `json:"verdict"`
	Open         int    `json:"open"`
	ObservedAt   string `json:"observed_at,omitempty"`
	LastReportAt string `json:"last_report_at,omitempty"`
}

type publicRouterQueue struct {
	Agent string `json:"agent"`
	Open  int    `json:"open"`
}

type publicRouterItem struct {
	ID                  string `json:"id"`
	From                string `json:"from"`
	To                  string `json:"to"`
	Title               string `json:"title"`
	Type                string `json:"type"`
	Opened              string `json:"opened"`
	AckedAt             string `json:"acked_at,omitempty"`
	RequestBodyWithheld bool   `json:"request_body_withheld"`
	BlockerRecorded     bool   `json:"blocker_recorded"`
}

type publicConsumers struct {
	Running int `json:"running"`
	Max     int `json:"max"`
}

type publicRegistry struct {
	Pinned    bool   `json:"pinned"`
	Commit    string `json:"commit,omitempty"`
	FetchedAt string `json:"fetched_at,omitempty"`
}

type publicKnownFail struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	FixedIn string `json:"fixed_in"`
}

type publicRouterSwap struct {
	At          string  `json:"at"`
	Verdict     string  `json:"verdict"`
	UsedMiB     float64 `json:"used_mib"`
	TotalMiB    float64 `json:"total_mib"`
	FreePct     int     `json:"free_pct"`
	DeltaPages  int64   `json:"delta_pages"`
	Correctness bool    `json:"correctness_only_ok"`
	Timing      bool    `json:"release_timing_ok"`
	Restart     bool    `json:"restart_proposed"`
}

type publicRelease struct {
	Version                string `json:"version"`
	Date                   string `json:"date,omitempty"`
	ReleaseBulletsWithheld bool   `json:"release_bullets_withheld"`
}

type publicAttention struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Agent    string `json:"agent,omitempty"`
	Title    string `json:"title"`
}

func (s *Server) routerResponse(snapshot RouterSnapshot) any {
	if !s.cfg.CloudBoard {
		return snapshot
	}
	return projectPublicRouter(snapshot)
}

func projectPublicRouter(s RouterSnapshot) publicRouterSnapshot {
	out := publicRouterSnapshot{
		Fabric: s.Fabric, FabricGeneration: s.FabricGeneration,
		GeneratedAt: s.GeneratedAt, BuiltMs: s.BuiltMs, Version: s.Version,
		Lanes: publicRouterLanes{Counts: map[string]int{}, List: []publicRouterLane{}},
		Queue: []publicRouterQueue{}, Items: []publicRouterItem{},
		Consumers:     publicConsumers{Running: s.Consumers.Running, Max: s.Consumers.Max},
		Registry:      publicRegistry{Pinned: s.Registry.Pinned, Commit: s.Registry.Commit, FetchedAt: s.Registry.FetchedAt},
		KnownFailures: []publicKnownFail{}, Releases: []publicRelease{}, Attention: []publicAttention{},
	}
	// Keys are also data. Do not forward arbitrary producer-supplied map keys.
	for _, verdict := range []string{"LIVE", "WAKEABLE", "HELD", "AUTH_REQUIRED", "WATCH_ONLY", "UNSTAFFED", "UNREACHABLE", "PROVIDER_QUOTA"} {
		if n, ok := s.Lanes.Counts[verdict]; ok {
			out.Lanes.Counts[verdict] = n
		}
	}
	for _, lane := range s.Lanes.List {
		out.Lanes.List = append(out.Lanes.List, publicRouterLane{Agent: lane.Agent, Verdict: lane.Verdict, Open: lane.Open, ObservedAt: lane.ObservedAt, LastReportAt: lane.LastReportAt})
	}
	for _, row := range s.Queue {
		out.Queue = append(out.Queue, publicRouterQueue(row))
	}
	for _, item := range s.Items {
		// Presence only: arbitrary blocker text does not prove a dependency is
		// unresolved (or even that it names a task).
		out.Items = append(out.Items, publicRouterItem{ID: item.ID, From: item.From, To: item.To, Title: item.Title, Type: item.Type, Opened: item.Opened, AckedAt: item.AckedAt, RequestBodyWithheld: true, BlockerRecorded: item.BlockedBy != ""})
	}
	for _, failure := range s.KnownFailures {
		out.KnownFailures = append(out.KnownFailures, publicKnownFail{ID: failure.ID, Title: failure.Title, Status: failure.Status, FixedIn: failure.FixedIn})
	}
	if swap := s.Swap; swap != nil {
		out.Swap = &publicRouterSwap{At: swap.At, Verdict: swap.Verdict, UsedMiB: swap.UsedMiB, TotalMiB: swap.TotalMiB, FreePct: swap.FreePct, DeltaPages: swap.DeltaPages, Correctness: swap.Correctness, Timing: swap.Timing, Restart: swap.Restart}
	}
	for _, release := range s.Releases {
		out.Releases = append(out.Releases, publicRelease{Version: release.Version, Date: release.Date, ReleaseBulletsWithheld: true})
	}
	for _, attention := range s.Attention {
		out.Attention = append(out.Attention, publicAttention{ID: attention.ID, Severity: attention.Severity, Agent: attention.Agent, Title: attention.Title})
	}
	return out
}
