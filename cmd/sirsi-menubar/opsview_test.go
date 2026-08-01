package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestOpsLeadRow(t *testing.T) {
	// Healthy: default glyph, compact roll-up.
	if got := opsLeadRowAt(dashboard.OpsSummary{LiveThreadCount: 5, QueueOpenItems: 2}, time.Now()); got != "🟢 ops: 5 live · 0 stale · 2 queued · age unknown" {
		t.Errorf("healthy lead = %q", got)
	}
	// Drift/auth + failures + suspended + explicit worst icon.
	got := opsLeadRow(dashboard.OpsSummary{
		WorstIcon: "🔴", LiveThreadCount: 3, StaleThreadCount: 1, QueueOpenItems: 4,
		SuspendedThreads: 2, RecentFailureCount: 1, HasDriftOrAuthIssue: true,
	})
	for _, want := range []string{"🔴", "3 live", "1 stale", "4 queued", "2 suspended", "1 fail", "⚠ drift/auth"} {
		if !strings.Contains(got, want) {
			t.Errorf("lead %q missing %q", got, want)
		}
	}
}

func TestOpsSnapshotRows_ErrorReplacesPriorSuccess(t *testing.T) {
	now := time.Date(2026, 8, 1, 3, 5, 0, 0, time.UTC)
	ns := &router.NodeStatus{GeneratedAt: now.Add(-5 * time.Second).Format(time.RFC3339), LiveThreadCount: 2}
	_, lead, _ := opsSnapshotRows(ns, nil, 12, now)
	if !strings.Contains(lead, "2 live") || !strings.Contains(lead, "age 5s") {
		t.Fatalf("success render = %q", lead)
	}

	sum, lead, rows := opsSnapshotRows(ns, errors.New("store unavailable"), 12, now)
	if lead != "🔴 ops: unavailable · age unknown" || len(rows) != 1 || !strings.Contains(rows[0], "unavailable") {
		t.Fatalf("error transition retained stale success: sum=%+v lead=%q rows=%v", sum, lead, rows)
	}
}

func TestOpsAgentRows(t *testing.T) {
	s := dashboard.OpsSummary{
		Agents: []dashboard.AgentSummary{
			{AgentID: "claude-pantheon", LiveThreads: 1},
			{AgentID: "claude-home", LiveThreads: 1, StaleThreads: 2, PendingItems: 3},
			{AgentID: "codex-pantheon", NeedsLogin: true},
		},
		MoreAgents: 4,
	}
	rows := opsAgentRows(s)
	if len(rows) != 4 { // 3 agents + 1 overflow
		t.Fatalf("got %d rows, want 4: %v", len(rows), rows)
	}
	// Healthy agent: 🟢, "1 live".
	if !strings.Contains(rows[0], "🟢") || !strings.Contains(rows[0], "claude-pantheon") || !strings.Contains(rows[0], "1 live") {
		t.Errorf("row0 = %q", rows[0])
	}
	// Agent with old (stale) registrations: stays 🟢 — old session records are
	// cruft, not an alarm — but the count is still surfaced as "N old".
	if !strings.Contains(rows[1], "🟢") || strings.Contains(rows[1], "🟡") ||
		!strings.Contains(rows[1], "2 old") || !strings.Contains(rows[1], "3 pending") {
		t.Errorf("row1 = %q", rows[1])
	}
	// Needs-login agent: 🔑 + "needs login" (precedence over live count).
	if !strings.Contains(rows[2], "🔑") || !strings.Contains(rows[2], "needs login") {
		t.Errorf("row2 = %q", rows[2])
	}
	// Overflow row.
	if !strings.Contains(rows[3], "+4 more agent") {
		t.Errorf("overflow row = %q", rows[3])
	}
	// Every agent row uses the menubar's indented "  <icon> <name> — <state>" style.
	for i, r := range rows[:3] {
		if !strings.HasPrefix(r, "  ") || !strings.Contains(r, " — ") {
			t.Errorf("row%d %q not in '  <icon> <name> — <state>' style", i, r)
		}
	}
}

// No agents + no overflow → no rows (clean empty render, no spurious lines).
func TestOpsAgentRows_Empty(t *testing.T) {
	if rows := opsAgentRows(dashboard.OpsSummary{}); len(rows) != 0 {
		t.Errorf("empty summary produced rows: %v", rows)
	}
}
