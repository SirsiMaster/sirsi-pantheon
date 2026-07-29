package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
)

func TestCommandDeckRows_GemmaOnline(t *testing.T) {
	rows := commandDeckRows(&StatsSnapshot{
		PrimaryAccelerator: "M4 Max",
		RAMPercent:         42,
		RAMPressure:        "low",
		GitBranch:          "codex/menubar-command-deck",
		UncommittedFiles:   2,
		OsirisRisk:         "low",
	}, dashboard.OpsSummary{
		LiveThreadCount: 8,
		QueueOpenItems:  3,
		Agents: []dashboard.AgentSummary{
			{AgentID: "gemma-pantheon", LiveThreads: 1, PendingItems: 2},
		},
	})
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5: %v", len(rows), rows)
	}
	for _, want := range []string{"Local AI", "Gemma online", "2 admin task"} {
		if !strings.Contains(rows[0], want) {
			t.Errorf("local ai row %q missing %q", rows[0], want)
		}
	}
	if !strings.Contains(rows[1], "M4 Max") || !strings.Contains(rows[1], "42% RAM") {
		t.Errorf("compute row = %q", rows[1])
	}
	if !strings.Contains(rows[2], "8 live") || !strings.Contains(rows[2], "3 queued") || !strings.Contains(rows[2], "armed") {
		t.Errorf("router row = %q", rows[2])
	}
	if !strings.Contains(rows[3], "full wake digest") || !strings.Contains(rows[3], "3 item") {
		t.Errorf("context row = %q", rows[3])
	}
	if !strings.Contains(rows[4], "codex/menubar-command-deck") || !strings.Contains(rows[4], "2 changed") {
		t.Errorf("risk row = %q", rows[4])
	}
}

func TestCommandDeckRows_GemmaMisregistered(t *testing.T) {
	rows := commandDeckRows(&StatsSnapshot{}, dashboard.OpsSummary{
		Agents: []dashboard.AgentSummary{
			{AgentID: "gemma", StaleThreads: 2},
		},
	})
	if !strings.Contains(rows[0], "Gemma misregistered") || !strings.Contains(rows[0], "admin held") {
		t.Fatalf("local ai row = %q", rows[0])
	}
}

func TestCommandDeckRows_RouterAttention(t *testing.T) {
	rows := commandDeckRows(&StatsSnapshot{RAMPercent: 91, RAMPressure: "high"}, dashboard.OpsSummary{
		LiveThreadCount:     4,
		QueueOpenItems:      1,
		HasDriftOrAuthIssue: true,
		RecentFailureCount:  1,
		StaleThreadCount:    2,
	})
	if !strings.Contains(rows[1], "memory constrained") {
		t.Errorf("compute row = %q", rows[1])
	}
	if !strings.Contains(rows[2], "drift/auth attention") {
		t.Errorf("router row = %q", rows[2])
	}
}

func TestCommandDeckRows_OfflineDefaults(t *testing.T) {
	rows := commandDeckRows(nil, dashboard.OpsSummary{})
	if !strings.Contains(rows[0], "Gemma offline") {
		t.Errorf("local ai row = %q", rows[0])
	}
	if !strings.Contains(rows[1], "detecting hardware") {
		t.Errorf("compute row = %q", rows[1])
	}
	if !strings.Contains(rows[3], "quiet") {
		t.Errorf("context row = %q", rows[3])
	}
}
