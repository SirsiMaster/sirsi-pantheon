package main

import (
	"strings"
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

// Every attention item carries a stable id; lane items name their lane; a next
// step is always a copyable command naming that lane (the UI never executes text).
func TestAttentionHasStableIDsLanesAndCopyOnlyNextSteps(t *testing.T) {
	s := dashboard.RouterSnapshot{Registry: dashboard.RouterRegistryPin{Pinned: false}, Consumers: dashboard.RouterConsumers{Running: 2, Max: 2},
		Lanes: dashboard.RouterLanes{List: []dashboard.RouterLaneVerdict{
			{Agent: "q", Verdict: "HELD", Detail: "held: quarantine (needs a human)"},
			{Agent: "w", Verdict: "WATCH_ONLY", Open: 3},
			{Agent: "u", Verdict: "UNSTAFFED", Open: 1}}}}
	first, second := routerAttention(s), routerAttention(s)
	if len(first) != 5 {
		t.Fatalf("want 5 items, got %d: %+v", len(first), first)
	}
	seen := map[string]bool{}
	for i, a := range first {
		if a.ID == "" || a.ID != second[i].ID {
			t.Fatalf("id missing or unstable: %+v vs %+v", a, second[i])
		}
		if seen[a.ID] {
			t.Fatalf("duplicate id %q", a.ID)
		}
		seen[a.ID] = true
		if a.Next != nil && (a.Next.Kind != "command-copy" || a.Next.Command == "") {
			t.Fatalf("next step must be a copyable command: %+v", a.Next)
		}
		if a.Agent != "" && a.Next != nil && !strings.Contains(a.Next.Command, a.Agent) {
			t.Fatalf("lane item's command must name its lane: %+v", a)
		}
	}
	for _, a := range first {
		switch a.ID {
		case "unworked:w":
			if a.Agent != "w" || !strings.Contains(a.Next.Command, "router ping w") {
				t.Fatalf("WATCH_ONLY must diagnose, not install: %+v", a)
			}
		case "unworked:u":
			if !strings.Contains(a.Next.Command, "wake-install u") {
				t.Fatalf("UNSTAFFED should install: %+v", a)
			}
		}
	}
}

// A verdict with no lanes is an explicit zero, never an absent key.
func TestVerdictCountsAreZeroFilled(t *testing.T) {
	c := zeroVerdictCounts()
	for _, v := range []string{"LIVE", "WAKEABLE", "HELD", "AUTH_REQUIRED", "WATCH_ONLY", "UNSTAFFED", "UNREACHABLE"} {
		if n, ok := c[v]; !ok || n != 0 {
			t.Fatalf("verdict %s must be present at zero, got %v ok=%v", v, n, ok)
		}
	}
}

// A section with no entries (the normal state of [Unreleased] right after a cut) is an
// empty list, never JSON null: a UI iterating items must not have to guard for it.
func TestReleaseWithNoEntriesHasAnEmptyItemsListNotNull(t *testing.T) {
	got := readChangelogReleases("## [Unreleased]\n\n## [1.0.0] — 2026-01-01\n\n- shipped\n", 3)
	if len(got) != 2 || got[0].Version != "Unreleased" {
		t.Fatalf("want Unreleased then 1.0.0, got %+v", got)
	}
	if got[0].Items == nil || len(got[0].Items) != 0 {
		t.Fatalf("an empty Unreleased must have Items == []string{}, got %#v", got[0].Items)
	}
}
