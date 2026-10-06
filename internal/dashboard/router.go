package dashboard

import (
	"encoding/json"
	"net/http"
)

// RouterSnapshot is the router panel: everything an operator needs to see about
// the router in one read, produced by the caller (the dashboard does not import
// the registry or store itself).
type RouterSnapshot struct {
	GeneratedAt string `json:"generated_at"`
	// BuiltMs is how long producing this snapshot took, and Timings the same by
	// stage, so a slow panel can be traced to its source rather than guessed at.
	BuiltMs       int64             `json:"built_ms"`
	Timings       map[string]int64  `json:"timings_ms,omitempty"`
	Version       string            `json:"version"`
	Lanes         RouterLanes       `json:"lanes"`
	Queue         []RouterQueueRow  `json:"queue"`
	Consumers     RouterConsumers   `json:"consumers"`
	Registry      RouterRegistryPin `json:"registry"`
	KnownFailures []RouterKnownFail `json:"known_failures"`
	Swap          *RouterSwap       `json:"swap,omitempty"`
	Releases      []RouterRelease   `json:"releases"`
	// Attention is computed deterministically from the data above: what needs a
	// person or a fix now, most severe first. Empty means nothing does.
	Attention []RouterAttention `json:"attention"`
}

// RouterAttention is one thing that needs attention. Severity is critical, warn or info.
type RouterAttention struct {
	// ID is stable across refreshes for the same condition (kind plus lane), so a
	// UI can keep a selection without parsing the title.
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Agent    string `json:"agent,omitempty"` // the related lane, when there is one
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Action   string `json:"action,omitempty"`
	// Next is a typed next step. The UI may copy a command or navigate to a known
	// route; it never executes text.
	Next *RouterNext `json:"next,omitempty"`
}

// RouterNext is one safe next step. Kind is "command-copy" (Command is shown and
// copied, never run) or "navigate" (Target is a dashboard route such as "lanes").
type RouterNext struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Command string `json:"command,omitempty"`
	Target  string `json:"target,omitempty"`
}

type RouterLanes struct {
	Counts map[string]int      `json:"counts"`
	List   []RouterLaneVerdict `json:"list"`
}

// RouterLaneVerdict is one lane's honest availability (the same verdict as
// `sirsi router ping`), never a second classification.
type RouterLaneVerdict struct {
	Agent   string `json:"agent"`
	Verdict string `json:"verdict"`
	Detail  string `json:"detail"`
	Open    int    `json:"open"` // open router items addressed to this lane
	// Evidence, each omitted when unknown (omitted never means healthy).
	ObservedAt        string `json:"observed_at,omitempty"`         // when this verdict was computed
	WorkerThreadID    string `json:"worker_thread_id,omitempty"`    // the thread the verdict rests on
	LastReportAt      string `json:"last_report_at,omitempty"`      // the worker's own last publication
	LastReportSummary string `json:"last_report_summary,omitempty"` // its last consumer outcome
}

type RouterQueueRow struct {
	Agent string `json:"agent"`
	Open  int    `json:"open"`
}

type RouterConsumers struct {
	Running int `json:"running"`
	Max     int `json:"max"`
}

type RouterRegistryPin struct {
	Pinned    bool   `json:"pinned"`
	Source    string `json:"source"`
	Commit    string `json:"commit,omitempty"`
	FetchedAt string `json:"fetched_at,omitempty"`
}

type RouterKnownFail struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	FixedIn string `json:"fixed_in"`
	Guard   string `json:"guard"`
}

type RouterSwap struct {
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

type RouterRelease struct {
	Version string   `json:"version"`
	Date    string   `json:"date,omitempty"`
	Items   []string `json:"items"`
}

// RouterProducer builds the router panel.
type RouterProducer func() (RouterSnapshot, error)

// apiRouter serves GET /api/router. An unwired or failing producer is a 503/502,
// never an empty panel that would read as "the router has nothing to report".
func (s *Server) apiRouter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.cfg.RouterFn == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "router producer not configured"})
		return
	}
	snap, err := s.cfg.RouterFn()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(snap)
}
