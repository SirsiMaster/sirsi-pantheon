package dashboard

import (
	"encoding/json"
	"net/http"
)

// RouterSnapshot is the router panel: everything an operator needs to see about
// the router in one read, produced by the caller (the dashboard does not import
// the registry or store itself).
type RouterSnapshot struct {
	GeneratedAt   string            `json:"generated_at"`
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
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Action   string `json:"action,omitempty"`
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
