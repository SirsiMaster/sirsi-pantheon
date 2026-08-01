// Package dashboard — nodestatus.go
//
// ADR-026 Horus ops-dashboard read endpoint. Serves the typed router.NodeStatus
// read-model at GET /api/node-status, plus a bounded OpsSummary projection at
// ?view=summary for the menubar (NSMenu budget — top-N + "N more" overflow).
//
// Boundary: claude-home defines this read contract; claude-pantheon owns
// surface chrome (menubar rows, TUI pane) that decodes into these types.
// Surfaces never re-aggregate — they consume one read-model, N read-only
// projections.
package dashboard

import (
	"net/http"
	"sort"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// nexusOrigins is the allowlist of web origins permitted to read local
// dashboard data cross-origin. Deliberately explicit, not a wildcard: this
// data includes live agent PIDs and session identifiers, so only the known
// Nexus web-panel origins (production + local dev server) may read it, and
// only over a same-machine loopback request in the first place.
var nexusOrigins = map[string]bool{
	"https://sirsi.ai":         true,
	"https://sirsi-ai.web.app": true,
	"http://localhost:5183":    true,
	"http://127.0.0.1:5183":    true,
}

// allowNexusOrigin sets Access-Control-Allow-Origin when the request's Origin
// header matches nexusOrigins. No-op (and no header set) for any other origin,
// which browsers treat as a same-origin-only response.
func allowNexusOrigin(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && nexusOrigins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
}

// DefaultOpsSummaryMax bounds the OpsSummary agent list to fit a typical
// NSMenu (~12-20 rows comfortably; we pick 12 to leave headroom for fixed rows
// like counts and the "Open full dashboard" link). The remainder collapse into
// `more_agents` so the menubar can render an "N more…" overflow row.
const DefaultOpsSummaryMax = 12

// OpsSummary is the bounded menubar projection of router.NodeStatus.
// It is a pure reduction — every field is derived from the source NodeStatus
// in Summarize(); no field is sourced independently. This keeps
// "one read-model" honest even when the menubar's NSMenu can't show the full
// thread list (claude-pantheon constraint #2 from boundary ack 235652).
type OpsSummary struct {
	SchemaVersion string `json:"schema_version"` // mirrors router.NodeStatus
	GeneratedAt   string `json:"generated_at"`   // source snapshot time; never restamped by a surface

	// Roll-ups (reduce loops, not separate sources)
	LiveThreadCount    int `json:"live_thread_count"`
	StaleThreadCount   int `json:"stale_thread_count"`
	SuspendedThreads   int `json:"suspended_threads,omitempty"` // populated when ADR-025 records visible
	QueueOpenItems     int `json:"queue_open_items"`            // = TotalPending
	RecentFailureCount int `json:"recent_failure_count"`

	// Drift / health flags — true if any agent CLI has auth issues, daemon misconfig,
	// or the binary drift detector (ADR-023) found a stale sibling.
	HasDriftOrAuthIssue bool   `json:"has_drift_or_auth_issue"`
	WorstIcon           string `json:"worst_icon,omitempty"` // glyph hint for the menubar's lead row

	// Bounded agent list — top-N agents by pending+live signal, others collapsed.
	Agents     []AgentSummary `json:"agents"`
	MoreAgents int            `json:"more_agents,omitempty"` // count of agents NOT shown

	// Echo source identifiers so the menubar can deep-link.
	RouterHome string `json:"router_home"`
}

// AgentSummary is a single menubar row.
type AgentSummary struct {
	AgentID      string `json:"agent_id"`
	LiveThreads  int    `json:"live_threads"`
	StaleThreads int    `json:"stale_threads"`
	PendingItems int    `json:"pending_items"`
	NeedsLogin   bool   `json:"needs_login,omitempty"`
}

// Summarize is the **exported** pure reduction NodeStatus → OpsSummary.
//
// Surfaces (menubar, future macapp) call this directly in-process to avoid the
// silly serialize→HTTP-loopback→deserialize path when they already have a
// *router.NodeStatus in hand. It's the same reduction served at
// GET /api/node-status?view=summary — one read-model, one reducer, no surface
// re-aggregates (ADR-026 invariant).
//
// Bounded by `max` (the agent list is truncated; remainder counted in
// MoreAgents). Stable ordering: pending desc, then live desc, then alpha — for
// diff-rendering by the menubar without flicker on equal-signal agents. Pass
// DefaultOpsSummaryMax to use the NSMenu-safe default (12 + overflow row).
//
// Pure function — no host side effects, safe to call from any goroutine.
func Summarize(ns *router.NodeStatus, max int) OpsSummary {
	if max <= 0 {
		max = DefaultOpsSummaryMax
	}
	sum := OpsSummary{
		SchemaVersion:      ns.SchemaVersion,
		GeneratedAt:        ns.GeneratedAt,
		LiveThreadCount:    ns.LiveThreadCount,
		StaleThreadCount:   len(ns.StaleThreads),
		QueueOpenItems:     ns.TotalPending,
		RecentFailureCount: len(ns.RecentFailures),
		RouterHome:         ns.RouterHome,
	}

	// SuspendedThreads is the count of LiveThreads + StaleThreads whose Status
	// is the ADR-025 "suspended" lifecycle state. CollectNodeStatus drops
	// terminal statuses (closed/reaped) before populating these slices, so a
	// suspended record can land in either bucket depending on its idleness.
	for _, t := range ns.LiveThreads {
		if t.Status == router.ThreadStatusSuspended {
			sum.SuspendedThreads++
		}
	}
	for _, t := range ns.StaleThreads {
		if t.Status == router.ThreadStatusSuspended {
			sum.SuspendedThreads++
		}
	}

	// Drift / auth — true if any agent CLI needs a real re-login, or any wake
	// mechanism is not ready, or daemon is installed but its configured binary
	// does not exist (ADR-023 drift class). Only a confirmed logout (NeedsLogin)
	// counts — a DEGRADED/inconclusive probe (cold-start timeout) is NOT an
	// actionable auth issue and must never paint Horus red (the 8s-timeout false
	// alarm: nothing the user could click would clear it). See feedback
	// "surfaces_current_actionable_only".
	for _, h := range ns.AgentHealth {
		if h.CLIFound && h.NeedsLogin {
			sum.HasDriftOrAuthIssue = true
			break
		}
	}
	if !sum.HasDriftOrAuthIssue {
		for _, w := range ns.WakeHealth {
			if !w.Ready {
				sum.HasDriftOrAuthIssue = true
				break
			}
		}
	}
	if !sum.HasDriftOrAuthIssue && ns.DaemonInstalled && ns.ConfiguredBinary != "" && !ns.BinaryExists {
		sum.HasDriftOrAuthIssue = true
	}
	if sum.HasDriftOrAuthIssue {
		sum.WorstIcon = "🔴"
	} else if sum.RecentFailureCount > 0 {
		// Only a CURRENT, actionable signal turns Horus yellow — a recent
		// failure. Stale thread registrations (ended sessions whose records
		// weren't reaped) are registry cruft, not a problem the user can act
		// on, so they must NOT paint Horus yellow — that was a permanent
		// false alarm no click could clear. The count is still surfaced in the
		// per-agent rows as plain information.
		sum.WorstIcon = "🟡"
	} else {
		sum.WorstIcon = "🟢"
	}

	// Per-agent roll-up. Sources: ns.RegisteredAgents (membership),
	// ns.PendingByAgent (queue), ns.LiveThreads/StaleThreads (presence),
	// ns.AgentHealth (auth).
	type agg struct {
		live, stale, pending int
		needsLogin           bool
	}
	by := map[string]*agg{}
	get := func(id string) *agg {
		a, ok := by[id]
		if !ok {
			a = &agg{}
			by[id] = a
		}
		return a
	}
	for _, id := range ns.RegisteredAgents {
		_ = get(id) // ensure registered agents appear even with zero signal
	}
	for _, t := range ns.LiveThreads {
		get(t.AgentID).live++
	}
	for _, t := range ns.StaleThreads {
		get(t.AgentID).stale++
	}
	for agentID, ids := range ns.PendingByAgent {
		get(agentID).pending += len(ids)
	}
	for _, h := range ns.AgentHealth {
		// AgentHealth keys on AgentType ("claude"/"codex"), not agent_id; mark every
		// agent_id whose name contains the failing type as needs_login. This mirrors
		// CollectNodeStatus's own BlockedItems computation.
		if h.CLIFound && h.NeedsLogin {
			for id := range by {
				if containsType(id, h.AgentType) {
					by[id].needsLogin = true
				}
			}
		}
	}

	// Rank: pending desc, then live desc, then alpha for determinism.
	ids := make([]string, 0, len(by))
	for id := range by {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		ai, aj := by[ids[i]], by[ids[j]]
		if ai.pending != aj.pending {
			return ai.pending > aj.pending
		}
		if ai.live != aj.live {
			return ai.live > aj.live
		}
		return ids[i] < ids[j]
	})

	if len(ids) > max {
		sum.MoreAgents = len(ids) - max
		ids = ids[:max]
	}
	sum.Agents = make([]AgentSummary, 0, len(ids))
	for _, id := range ids {
		a := by[id]
		sum.Agents = append(sum.Agents, AgentSummary{
			AgentID:      id,
			LiveThreads:  a.live,
			StaleThreads: a.stale,
			PendingItems: a.pending,
			NeedsLogin:   a.needsLogin,
		})
	}
	return sum
}

// containsType reports whether an agent id contains a CLI type token
// ("claude" or "codex"). Exists so Summarize doesn't pull in strings.
//
// v1 CONSTRAINT (codex arch-verify 2026-06-02): substring match works because
// current agent IDs are all "claude-*" or "codex-*" prefixed (claude-pantheon,
// codex-finalwishes, etc). If agent naming expands beyond those prefixes,
// replace this with a prefix match or an explicit agent_type / agent-id
// mapping before relying on it for auth badges. The risk is a false-positive
// auth alarm on an agent whose id incidentally contains "claude"/"codex" as a
// substring (e.g. a future "myclaudething" agent).
func containsType(agentID, cliType string) bool {
	// agent ids are "claude-pantheon", "codex-finalwishes", etc.
	if len(cliType) == 0 || len(agentID) < len(cliType) {
		return false
	}
	for i := 0; i+len(cliType) <= len(agentID); i++ {
		if agentID[i:i+len(cliType)] == cliType {
			return true
		}
	}
	return false
}

// NodeStatusCollector is the producer hook for the GET /api/node-status
// endpoint. Tests inject a deterministic *router.NodeStatus without touching
// the host. Production wiring (cmd/sirsi-menubar) passes
// router.CollectNodeStatus's caller via Config.NodeStatusFn.
type NodeStatusCollector func() (*router.NodeStatus, error)

// apiNodeStatus serves GET /api/node-status (ADR-026).
//   - default: full router.NodeStatus
//   - ?view=summary: bounded OpsSummary (top-N agents + "more_agents")
//
// Read-only: no method-gating, no ConfirmGuard path, no side effects.
//
// CORS is scoped to the known Nexus web-panel origins (ADR-047 shared-services
// consumer) rather than a wildcard, since this endpoint exposes live agent PIDs
// and session identifiers — local-only data that should only ever be readable
// by a page the operator is themselves looking at, never a wildcard origin.
func (s *Server) apiNodeStatus(w http.ResponseWriter, r *http.Request) {
	allowNexusOrigin(w, r)
	if s.cfg.NodeStatusFn == nil {
		writeError(w, "node-status not available (collector not wired)", http.StatusServiceUnavailable)
		return
	}
	ns, err := s.cfg.NodeStatusFn()
	if err != nil {
		writeError(w, "node-status collection failed", http.StatusInternalServerError)
		return
	}
	if ns == nil {
		writeError(w, "node-status returned nil", http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("view") == "summary" {
		writeJSON(w, Summarize(ns, DefaultOpsSummaryMax))
		return
	}
	writeJSON(w, ns)
}
