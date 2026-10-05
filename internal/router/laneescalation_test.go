package router

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/supervision"
)

func ownerOpenTitles(t *testing.T, root string) map[string]bool {
	t.Helper()
	f, err := dispatch.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	items, err := f.Inbox("owner")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for _, it := range items {
		m[it.Title] = true
	}
	return m
}

// A Horus alert is closed once the lane is no longer escalated, left open while it
// still is, and an owner card from anyone else is never touched (both directions).
func TestRouteLaneEscalationsResolvesClearedAlerts(t *testing.T) {
	root := t.TempDir()
	t.Setenv(routercfg.StoreWakeEnv, "1")
	f, err := dispatch.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{
		"Lane needs you: lane-a cannot be reached automatically",
		"Lane needs you: lane-b cannot be reached automatically",
	} {
		if _, err := f.Send("horus", "owner", title, "decision", "why"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Send("claude-pantheon", "owner", "Lane needs you: lane-c cannot be reached automatically", "decision", "a person wrote this"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	still := []supervision.Escalation{{Agent: "lane-b", OpenItems: 2}}
	if _, err := RouteLaneEscalations(root, still); err != nil {
		t.Fatal(err)
	}
	got := ownerOpenTitles(t, root)
	if got["Lane needs you: lane-a cannot be reached automatically"] {
		t.Fatal("lane-a is no longer escalated: its Horus alert must be closed")
	}
	if !got["Lane needs you: lane-b cannot be reached automatically"] {
		t.Fatal("lane-b is still escalated: its alert must stay open")
	}
	if !got["Lane needs you: lane-c cannot be reached automatically"] {
		t.Fatal("an owner card not sent by Horus must never be auto-closed")
	}

	// With nothing escalated at all, the remaining Horus alert resolves too (the
	// old early return skipped this case entirely).
	if _, err := RouteLaneEscalations(root, nil); err != nil {
		t.Fatal(err)
	}
	if ownerOpenTitles(t, root)["Lane needs you: lane-b cannot be reached automatically"] {
		t.Fatal("no escalations left: lane-b's alert must resolve")
	}
}
