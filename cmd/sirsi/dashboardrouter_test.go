package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
)

// The panel shows each release's headline items and marks the unreleased section;
// a bullet with a bold lead keeps the lead (both shapes).
func TestReadChangelogReleases(t *testing.T) {
	body := "# Changelog\n\n## [Unreleased] — wip\n\n- **Alpha thing.** It does a\n  long explanation.\n- plain bullet\n\n## [0.24.68] — 2026-10-05\n\n- **Beta thing:** shipped.\n\n## [0.24.67] — 2026-10-04\n\n- old\n"
	got := readChangelogReleases(body, 1)
	if len(got) != 2 || got[0].Version != "Unreleased" || got[0].Date != "" || got[1].Version != "0.24.68" || got[1].Date != "2026-10-05" {
		t.Fatalf("sections: %+v", got)
	}
	if len(got[0].Items) != 2 || got[0].Items[1] != "plain bullet" || got[0].Items[0][:11] != "Alpha thing" {
		t.Fatalf("unreleased items: %+v", got[0].Items)
	}
	if readChangelogReleases("", 3) != nil {
		t.Fatal("a missing changelog must yield nothing, not an invented list")
	}
}

// Several [Unreleased] headings merge into one section, and attention rules claim
// only what the data shows (both directions).
func TestChangelogMergesUnreleasedAndAttentionIsDerived(t *testing.T) {
	got := readChangelogReleases("## [Unreleased] — a\n\n- one\n\n## [Unreleased] — b\n\n- two\n\n## [1.0.0] — 2026-01-01\n\n- old\n", 3)
	if len(got) != 2 || len(got[0].Items) != 2 {
		t.Fatalf("unreleased headings must merge: %+v", got)
	}
	clean := dashboard.RouterSnapshot{Registry: dashboard.RouterRegistryPin{Pinned: true}, Consumers: dashboard.RouterConsumers{Running: 1, Max: 2},
		Lanes: dashboard.RouterLanes{List: []dashboard.RouterLaneVerdict{{Agent: "a", Verdict: "WAKEABLE"}, {Agent: "b", Verdict: "WATCH_ONLY", Open: 0}}}}
	if a := routerAttention(clean); len(a) != 0 {
		t.Fatalf("a healthy snapshot must report nothing: %+v", a)
	}
	sick := clean
	sick.Registry.Pinned = false
	sick.Lanes.List = []dashboard.RouterLaneVerdict{{Agent: "q", Verdict: "HELD", Detail: "held: quarantine (needs a human)"}, {Agent: "w", Verdict: "WATCH_ONLY", Open: 3}}
	a := routerAttention(sick)
	if len(a) != 3 || a[0].Severity != "critical" || a[0].Title != "q: quarantined" {
		t.Fatalf("attention wrong or unordered: %+v", a)
	}
}

// Attention IDs are stable selection identities (same severity+title -> same ID,
// different title -> different ID) and the agent field tracks the lane a row is
// about. Only a literal CLI invocation is promoted into a typed NextStep —
// prose guidance never is, so the UI can never be tricked into "running" advice.
func TestAttentionIDAgentAndTypedNextStep(t *testing.T) {
	sick := dashboard.RouterSnapshot{
		Registry:  dashboard.RouterRegistryPin{Pinned: false},
		Consumers: dashboard.RouterConsumers{Running: 1, Max: 2},
		Lanes: dashboard.RouterLanes{List: []dashboard.RouterLaneVerdict{
			{Agent: "q", Verdict: "HELD", Detail: "held: quarantine (needs a human)"},
		}},
	}
	a := routerAttention(sick)
	if len(a) != 2 {
		t.Fatalf("expected 2 attention rows, got %+v", a)
	}
	var quarantine, unpinned *dashboard.RouterAttention
	for i := range a {
		switch a[i].Title {
		case "q: quarantined":
			quarantine = &a[i]
		case "Registry is not pinned to origin/main":
			unpinned = &a[i]
		}
	}
	if quarantine == nil || unpinned == nil {
		t.Fatalf("missing expected rows: %+v", a)
	}
	if quarantine.ID == "" || unpinned.ID == "" || quarantine.ID == unpinned.ID {
		t.Fatalf("attention IDs must be stable and distinct: quarantine=%q unpinned=%q", quarantine.ID, unpinned.ID)
	}
	if attentionID(quarantine.Severity, quarantine.Title) != quarantine.ID {
		t.Fatalf("attention ID must be deterministic from severity+title")
	}
	if quarantine.Agent != "q" {
		t.Fatalf("quarantine row must name its lane: %+v", quarantine)
	}
	if quarantine.NextStep != nil {
		t.Fatalf("prose guidance must not become a typed next step: %+v", quarantine.NextStep)
	}
	if unpinned.NextStep == nil || unpinned.NextStep.Kind != "command-copy" || unpinned.NextStep.Command != "sirsi router registry sync --install" {
		t.Fatalf("a literal CLI action must become a command-copy next step: %+v", unpinned.NextStep)
	}
}
