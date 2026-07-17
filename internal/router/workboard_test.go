package router

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeItem(t *testing.T, root, id, from, to, title, status string, opened, closed time.Time) {
	t.Helper()
	dir := filepath.Join(root, "items")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nfrom: \"" + from + "\"\nto: \"" + to + "\"\ntitle: \"" + title + "\"\nstatus: " + status + "\nopened: " + opened.Format(time.RFC3339) + "\n"
	if status == "closed" {
		body += "closed: " + closed.Format(time.RFC3339) + "\n"
	}
	body += "---\n\n## Instructions\n\nx\n"
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestComputeWorkBoard pins the board contract: packages with titles + age,
// per-agent and fabric pace, oldest-first ordering, busiest-agent-first board.
func TestComputeWorkBoard(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()

	writeItem(t, root, "20260716-000001-a", "claude-home", "claude-pantheon", "old open thing", "open", now.Add(-30*time.Hour), time.Time{})
	writeItem(t, root, "20260717-000002-b", "gemma", "claude-pantheon", "fresh open thing", "open", now.Add(-1*time.Hour), time.Time{})
	// Closed 2h ago after 4h open → counts today + 7d, close time 4h.
	writeItem(t, root, "20260717-000003-c", "x", "claude-pantheon", "done today", "closed", now.Add(-6*time.Hour), now.Add(-2*time.Hour))
	// Closed 3 days ago after 12h → 7d only.
	writeItem(t, root, "20260714-000004-d", "x", "claude-home", "done this week", "closed", now.Add(-76*time.Hour), now.Add(-72*time.Hour))
	// Closed 30 days ago → outside every window.
	writeItem(t, root, "20260617-000005-e", "x", "claude-home", "ancient", "closed", now.Add(-31*24*time.Hour), now.Add(-30*24*time.Hour))

	b, err := ComputeWorkBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	if b.TotalOpen != 2 || b.ClosedToday != 1 || b.Closed7d != 2 {
		t.Fatalf("fabric totals open=%d today=%d 7d=%d, want 2/1/2", b.TotalOpen, b.ClosedToday, b.Closed7d)
	}
	if len(b.Agents) == 0 || b.Agents[0].AgentID != "claude-pantheon" {
		t.Fatalf("busiest agent first: got %+v", b.Agents)
	}
	p := b.Agents[0]
	if len(p.Open) != 2 || p.Open[0].Title != "old open thing" || p.Open[0].From != "claude-home" {
		t.Fatalf("packages oldest-first with titles: %+v", p.Open)
	}
	if p.Open[0].AgeHours < 29 || p.Open[0].AgeHours > 31 {
		t.Fatalf("age = %.1fh, want ~30", p.Open[0].AgeHours)
	}
	if p.ClosedToday != 1 || p.Closed7d != 1 {
		t.Fatalf("pantheon pace today=%d 7d=%d, want 1/1", p.ClosedToday, p.Closed7d)
	}
	if p.AvgCloseHours < 3.9 || p.AvgCloseHours > 4.1 {
		t.Fatalf("avg close = %.2fh, want ~4", p.AvgCloseHours)
	}
	// Fabric mean over the two in-window closes: (4h + 4h) / 2 = 4h.
	if b.AvgCloseHours < 3.9 || b.AvgCloseHours > 4.1 {
		t.Fatalf("fabric avg close = %.2fh, want ~4", b.AvgCloseHours)
	}
}

// TestComputeWorkBoard_LivePeersFromRegistry: a live agent with zero items is
// still on the board — peers must be visible even when idle.
func TestComputeWorkBoard_LivePeersFromRegistry(t *testing.T) {
	root := t.TempDir()
	old := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDAlive })
	defer setPIDStateFn(old)

	reg, _ := LoadThreadRegistry(root)
	id := NewThreadID()
	reg.Threads[id] = &Thread{
		ThreadID: id, AgentID: "claude-nexus", Surface: "worker", PID: 80001,
		Status: ThreadStatusActive, MachineID: MachineID(), LastSeenAt: time.Now().UTC(),
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}

	b, err := ComputeWorkBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	var nexus *AgentBoard
	for i := range b.Agents {
		if b.Agents[i].AgentID == "claude-nexus" {
			nexus = &b.Agents[i]
		}
	}
	if nexus == nil || !nexus.Live || len(nexus.Surfaces) != 1 || nexus.Surfaces[0] != "worker" {
		t.Fatalf("idle live peer missing or wrong: %+v", nexus)
	}
}
