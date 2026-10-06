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
	if quarantine.Next != nil {
		t.Fatalf("prose guidance must not become a typed next step: %+v", quarantine.Next)
	}
	if unpinned.Next == nil || unpinned.Next.Kind != "command-copy" || unpinned.Next.Command != "sirsi router registry sync --install" {
		t.Fatalf("a literal CLI action must become a command-copy next step: %+v", unpinned.Next)
	}
}

// applyLaneNextSteps mirrors an agent-scoped attention row's typed Next onto
// that same lane (first match wins), and never touches a lane with no
// matching attention row — the lane inspector reads lane.next directly, so a
// mismatch here would silently drop the button the UI already renders for it.
func TestApplyLaneNextStepsMirrorsAgentScopedNext(t *testing.T) {
	step := &dashboard.RouterNextStep{Kind: "command-copy", Label: "Copy command", Command: "sirsi router wake q"}
	snap := dashboard.RouterSnapshot{
		Lanes: dashboard.RouterLanes{List: []dashboard.RouterLaneVerdict{
			{Agent: "q", Verdict: "UNSTAFFED"},
			{Agent: "r", Verdict: "WAKEABLE"},
		}},
		Attention: []dashboard.RouterAttention{
			{Agent: "q", Severity: "warn", Title: "q needs staffing", Next: step},
		},
	}
	applyLaneNextSteps(&snap)
	if snap.Lanes.List[0].Next != step {
		t.Fatalf("lane q must receive its attention row's next step: %+v", snap.Lanes.List[0])
	}
	if snap.Lanes.List[1].Next != nil {
		t.Fatalf("lane r has no matching attention row and must stay nil: %+v", snap.Lanes.List[1])
	}
}
