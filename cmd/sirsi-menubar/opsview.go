package main

// opsview.go — ADR-026 step 4b (surface chrome): pure rendering of the Horus
// ops read-model (dashboard.OpsSummary) into menubar menu strings. These are
// pure functions (OpsSummary → display strings) so the presentation is unit-
// tested without the systray; the thin AddMenuItem/SetTitle glue in main.go's
// onReady + refresh loop consumes them. No re-aggregation — the menubar renders
// the canonical OpsSummary (ADR-026 "one read-model, N read-only projections").

import (
	"fmt"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// opsLeadRow renders the menubar's lead ops row: the worst-status glyph plus a
// compact roll-up of the read-model. WorstIcon is the source's own health hint
// (🟢/🟡/🔴); default to 🟢 when absent.
func opsLeadRow(s dashboard.OpsSummary) string {
	return opsLeadRowAt(s, time.Now().UTC())
}

func opsLeadRowAt(s dashboard.OpsSummary, now time.Time) string {
	icon := s.WorstIcon
	if icon == "" {
		icon = "🟢"
	}
	lead := fmt.Sprintf("%s ops: %d live · %d stale · %d queued",
		icon, s.LiveThreadCount, s.StaleThreadCount, s.QueueOpenItems)
	if s.SuspendedThreads > 0 {
		lead += fmt.Sprintf(" · %d suspended", s.SuspendedThreads)
	}
	if s.RecentFailureCount > 0 {
		lead += fmt.Sprintf(" · %d fail", s.RecentFailureCount)
	}
	if s.HasDriftOrAuthIssue {
		lead += " · ⚠ drift/auth"
	}
	generated, err := time.Parse(time.RFC3339, s.GeneratedAt)
	if err != nil || generated.IsZero() {
		lead += " · age unknown"
	} else {
		age := now.Sub(generated)
		if age < 0 {
			age = 0
		}
		lead += fmt.Sprintf(" · age %s", age.Round(time.Second))
	}
	return lead
}

// opsSnapshotRows converts one collection attempt into a complete render. An
// error replaces every prior value with an explicit unavailable state; callers
// must apply the returned rows even on failure so stale success cannot linger.
func opsSnapshotRows(ns *router.NodeStatus, collectErr error, max int, now time.Time) (dashboard.OpsSummary, string, []string) {
	if collectErr != nil || ns == nil {
		return dashboard.OpsSummary{}, "🔴 ops: unavailable · age unknown", []string{"  ⚠ node status unavailable"}
	}
	sum := dashboard.Summarize(ns, max)
	return sum, opsLeadRowAt(sum, now), opsAgentRows(sum)
}

// opsAgentRows renders one indented row per bounded agent, matching the menubar's
// existing "  <icon> <name> — <state>" style, plus a final overflow row when the
// summary collapsed agents (MoreAgents > 0). Icon precedence: needs-login (🔑) >
// healthy (🟢). Stale (old-session) registrations are shown in the state text
// but never alarm — they're cruft, not a current problem.
func opsAgentRows(s dashboard.OpsSummary) []string {
	rows := make([]string, 0, len(s.Agents)+1)
	for _, a := range s.Agents {
		icon := "🟢"
		var state string
		switch {
		case a.NeedsLogin:
			icon, state = "🔑", "needs login"
		default:
			state = fmt.Sprintf("%d live", a.LiveThreads)
			if a.StaleThreads > 0 {
				// Stale = leftover registrations from ended sessions (cruft),
				// not a live problem — surface the count, but don't alarm (the
				// icon stays 🟢). A yellow here was a permanent false alarm.
				state += fmt.Sprintf(", %d old", a.StaleThreads)
			}
			if a.PendingItems > 0 {
				state += fmt.Sprintf(", %d pending", a.PendingItems)
			}
		}
		rows = append(rows, fmt.Sprintf("  %s %s — %s", icon, a.AgentID, state))
	}
	if s.MoreAgents > 0 {
		rows = append(rows, fmt.Sprintf("  +%d more agent(s)", s.MoreAgents))
	}
	return rows
}
