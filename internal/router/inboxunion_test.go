package router

import "testing"

// Router item 20260731-105700: the supervisor called inboxUnion once PER AGENT,
// and each call is a full store OpenRoot → Inbox → Close. With 22 registered
// agents that is 22 SQLite open/close cycles per tick, each dirtying WAL and
// journal pages — ~2.1 GB/day of disk writes, enough that the kernel filed a
// disk-writes report. inboxUnionAll does the same work with ONE open.

func TestInboxUnionAll_PartitionsByRecipient(t *testing.T) {
	a := AgentConfig{ID: "agent-one", Type: "gemma", Wake: WakeConfig{Mechanism: WakeNone}}
	b := AgentConfig{ID: "agent-two", Type: "gemma", Wake: WakeConfig{Mechanism: WakeNone}}
	root := wakeTestRoot(t, a, b)
	sendItem(t, root, a.ID, "for one")
	sendItem(t, root, a.ID, "also for one")
	sendItem(t, root, b.ID, "for two")

	got, err := inboxUnionAll(root, []string{a.ID, b.ID})
	if err != nil {
		t.Fatalf("inboxUnionAll: %v", err)
	}
	if len(got[a.ID]) != 2 {
		t.Fatalf("agent-one got %d items, want 2", len(got[a.ID]))
	}
	if len(got[b.ID]) != 1 {
		t.Fatalf("agent-two got %d items, want 1", len(got[b.ID]))
	}
	for _, it := range got[a.ID] {
		if it.To != a.ID {
			t.Fatalf("item addressed to %q leaked into agent-one's partition", it.To)
		}
	}
}

// Every requested agent must APPEAR, even with no items — callers rely on
// "every registered agent is present" and a missing key would read as a lane
// that does not exist rather than one that is simply idle.
func TestInboxUnionAll_IdleAgentIsPresentNotAbsent(t *testing.T) {
	a := AgentConfig{ID: "busy", Type: "gemma", Wake: WakeConfig{Mechanism: WakeNone}}
	idle := AgentConfig{ID: "idle", Type: "gemma", Wake: WakeConfig{Mechanism: WakeNone}}
	root := wakeTestRoot(t, a, idle)
	sendItem(t, root, a.ID, "only for busy")

	got, err := inboxUnionAll(root, []string{a.ID, idle.ID})
	if err != nil {
		t.Fatalf("inboxUnionAll: %v", err)
	}
	if _, present := got[idle.ID]; !present {
		t.Fatal("an idle agent must be PRESENT with an empty inbox, not absent")
	}
	if len(got[idle.ID]) != 0 {
		t.Fatalf("idle agent got %d items, want 0", len(got[idle.ID]))
	}
}

// Items addressed to an agent NOT in the requested set must not be attributed
// to anyone — partitioning must not invent membership.
func TestInboxUnionAll_IgnoresUnrequestedRecipients(t *testing.T) {
	a := AgentConfig{ID: "wanted", Type: "gemma", Wake: WakeConfig{Mechanism: WakeNone}}
	other := AgentConfig{ID: "unwanted", Type: "gemma", Wake: WakeConfig{Mechanism: WakeNone}}
	root := wakeTestRoot(t, a, other)
	sendItem(t, root, a.ID, "mine")
	sendItem(t, root, other.ID, "not in the requested set")

	got, err := inboxUnionAll(root, []string{a.ID})
	if err != nil {
		t.Fatalf("inboxUnionAll: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("partition has %d agents, want exactly the 1 requested", len(got))
	}
	if len(got[a.ID]) != 1 {
		t.Fatalf("wanted agent got %d items, want 1", len(got[a.ID]))
	}
}
