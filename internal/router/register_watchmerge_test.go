package router

import "testing"

// TestRegisterThread_ReuseRefreshesWatches locks the fix for the codex-home
// 2026-06-19 bug: the idempotent reuse fast-path updated only LastSeenAt +
// CurrentItem, so an agent re-registering with NEW --watch values silently kept
// its old set — under-declaring its watches and missing inbox items. The
// re-register must restate the live declaration (replace-when-non-empty) while
// keeping the SAME thread_id (no duplicate thread/loop).
func TestRegisterThread_ReuseRefreshesWatches(t *testing.T) {
	root := t.TempDir()
	const agent, pid, start = "codex-home", 4242, "Mon Jun 1 09:00"

	mk := func(watches ...string) *Thread {
		return &Thread{AgentID: agent, Surface: "codex", PID: pid, StartTime: start, Watches: watches}
	}
	contains := func(ws []string, w string) bool {
		for _, x := range ws {
			if x == w {
				return true
			}
		}
		return false
	}

	first, err := RegisterThread(root, mk("codex-home"))
	if err != nil {
		t.Fatal(err)
	}

	// Re-register the SAME (agent, pid, start) adding two new watches.
	grown, err := RegisterThread(root, mk("codex-home", "codex-pantheon", "claude-home"))
	if err != nil {
		t.Fatal(err)
	}
	if grown.ThreadID != first.ThreadID {
		t.Fatalf("re-register must reuse the same thread, got %s vs %s", grown.ThreadID, first.ThreadID)
	}
	for _, w := range []string{"codex-home", "codex-pantheon", "claude-home"} {
		if !contains(grown.Watches, w) {
			t.Errorf("merged watches must include %q; got %v", w, grown.Watches)
		}
	}

	// Narrowing: re-register with only self → the declaration replaces (the agent
	// can tighten its watch set), still the same thread.
	narrowed, err := RegisterThread(root, mk("codex-home"))
	if err != nil {
		t.Fatal(err)
	}
	if narrowed.ThreadID != first.ThreadID {
		t.Fatalf("narrowing must reuse the same thread")
	}
	if len(narrowed.Watches) != 1 || narrowed.Watches[0] != "codex-home" {
		t.Errorf("narrowing must replace the set, got %v", narrowed.Watches)
	}

	// Empty watches on re-register must NOT wipe the existing declaration (a bare
	// heartbeat-style register is not a de-declaration).
	bare, err := RegisterThread(root, &Thread{AgentID: agent, Surface: "codex", PID: pid, StartTime: start})
	if err != nil {
		t.Fatal(err)
	}
	if len(bare.Watches) != 1 || bare.Watches[0] != "codex-home" {
		t.Errorf("empty re-register must leave watches untouched, got %v", bare.Watches)
	}
}

// TestNormalizeWatches_SelfAlwaysFirstAndDeduped: every thread watches its own
// inbox (self prepended) and duplicates collapse with stable order.
func TestNormalizeWatches_SelfAlwaysFirstAndDeduped(t *testing.T) {
	got := normalizeWatches("codex-home", []string{"claude-home", "codex-home", "claude-home", "", "codex-pantheon"})
	want := []string{"codex-home", "claude-home", "codex-pantheon"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order/dedupe wrong: got %v, want %v", got, want)
		}
	}

	// A registration that omits self still ends up watching self (first).
	if g := normalizeWatches("codex-home", []string{"claude-home"}); g[0] != "codex-home" {
		t.Errorf("self must be watched even when omitted; got %v", g)
	}
}
