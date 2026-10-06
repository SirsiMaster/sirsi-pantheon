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
	// ID is a stable selection identity (deterministic from severity+title),
	// so a UI can select/compare attention rows without parsing Title text.
	ID string `json:"id"`
	// Agent names the lane this attention item is about, when it is about one
	// specific lane rather than a fabric-wide condition (e.g. swap pressure).
	Agent    string `json:"agent,omitempty"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Action   string `json:"action,omitempty"`
	// Next is an additive, typed projection of Action for UIs that want to
	// offer a concrete follow-up instead of parsing the prose Action string.
	// Nil when Action is prose guidance rather than a copyable command or a
	// known in-app destination — the UI must not infer one from free text.
	Next *RouterNextStep `json:"next,omitempty"`
}

// RouterNextStep is a typed, additive next action for an attention row.
// Kind is "command-copy" (Command is a literal CLI invocation to copy, never
// execute) or "navigate" (Command is a known in-app route). The dashboard
// never invents or executes a command from unstructured text.
type RouterNextStep struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Command string `json:"command"`
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
	// WorkerThreadID is the thread registry ID the verdict was computed
	// against, when the verdict came from a live or recent thread record.
	// Empty when the verdict has no backing thread (e.g. UNSTAFFED). This is
	// a direct registry projection, not an inferred liveness claim — readiness
	// is not proof that a worker is executing.
	WorkerThreadID string `json:"worker_thread_id,omitempty"`
	// Next mirrors this lane's own attention row's typed next step (if any),
	// so the lane inspector can offer the same command-copy without the UI
	// having to cross-reference the attention list itself.
	Next *RouterNextStep `json:"next,omitempty"`
}

type RouterQueueRow struct {
	Agent string `json:"agent"`
	Open  int    `json:"open"`
	// Items is an optional per-item projection for queue drill-in. Omitted
	// (nil) means the caller did not supply item-level detail, which the UI
	// must render as "not available", never as "zero items".
	Items []RouterQueueItem `json:"items,omitempty"`
}

// RouterQueueItem is one inbox item's identity and ack state. There is no
// "lease owner/thread" on an inbox item — leases live on the task registry,
// a separate concept — so that field is intentionally not projected here.
type RouterQueueItem struct {
	ID             string `json:"id"`
	Recipient      string `json:"recipient"`
	Subject        string `json:"subject"`
	OpenedAt       string `json:"opened_at"`
	AcknowledgedAt string `json:"acknowledged_at,omitempty"`
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
